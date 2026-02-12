package gatesentryWebserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverEndpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"
	gatesentryWebserverFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gorilla/mux"
)

var hmacSampleSecret = []byte("I7JE72S9XJ48ANXMI78ASDNMQ839")

// corsMiddleware adds CORS headers to all API responses.
// Echoes back the Origin header to allow cross-origin requests from any hostname,
// which is necessary when accessing GateSentry from different device hostnames
// (e.g., monster-jj, monster-jj.local, monster-jj.jvj28.com, localhost, IP addresses).
var corsMiddleware mux.MiddlewareFunc = func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Pass     string `json:"pass"`
}

// blockedDomainMiddleware returns a middleware that intercepts requests from
// DNS-blocked domains. When the DNS server resolves a blocked domain to
// GateSentry's own IP, the browser sends the request here with the blocked
// domain as the Host header. This middleware detects that the Host doesn't
// belong to GateSentry and serves a block page instead of the admin UI.
func blockedDomainMiddleware(settings *gatesentry2storage.MapStore, port string) mux.MiddlewareFunc {
	// Build a set of hostnames that belong to GateSentry itself.
	// Any request with a Host header NOT in this set is assumed to be
	// from a DNS-blocked domain and gets the block page.
	knownHosts := map[string]bool{
		"localhost": true,
		"127.0.0.1": true,
		"::1":       true,
	}

	// Add the machine's hostname
	if hostname, err := os.Hostname(); err == nil {
		knownHosts[strings.ToLower(hostname)] = true
		// Also add hostname.local for mDNS
		knownHosts[strings.ToLower(hostname)+".local"] = true
	}

	// Add any local network IPs
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				knownHosts[ipnet.IP.String()] = true
			}
		}
	}

	blockedHandler := gatesentryWebserverEndpoints.GSBlockedPageHandler()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			// Strip port from Host header
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = strings.ToLower(host)

			// Also check the dynamic wpad_proxy_host setting
			if proxyHost := settings.Get("wpad_proxy_host"); proxyHost != "" {
				if strings.ToLower(proxyHost) == host {
					next.ServeHTTP(w, r)
					return
				}
			}

			if knownHosts[host] {
				next.ServeHTTP(w, r)
				return
			}

			// Host doesn't match any known GateSentry hostname —
			// this is a DNS-blocked domain, serve the block page
			log.Printf("[WEB] Serving block page for DNS-blocked domain: %s", r.Host)
			blockedHandler.ServeHTTP(w, r)
		})
	}
}

type ErrorResponse struct {
	StatusCode   int    `json:"status"`
	ErrorMessage string `json:"message"`
}

type OkResponse struct {
	Response string `json:"Response"`
}

func CreateToken(username string) (string, error) {

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"nbf":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour * 1).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString(hmacSampleSecret)

	return tokenString, err
}

func VerifyAdminUser(username string, password string, settingsStore *gatesentry2storage.MapStore) bool {
	if gatesentryWebserverTypes.GetAdminUser(settingsStore) == username &&
		gatesentryWebserverTypes.GetAdminPassword(settingsStore) == password {
		return true
	}
	return false
}

var tokenCreationHandler HttpHandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	// get username from context
	username, ok := r.Context().Value("username").(string)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error getting username"))
		return
	}
	token, err := CreateToken(username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error creating token"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"Jwtoken": "` + token + `", "Validated": "true"}`))

}

var authenticationMiddleware mux.MiddlewareFunc = func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.Header.Get("Authorization")
		// Check if tokenString starts with "Bearer ", and if so, remove it
		if strings.HasPrefix(tokenString, "Bearer ") {
			tokenString = strings.TrimPrefix(tokenString, "Bearer ")
		}
		// Fallback: accept token as a query parameter for SSE / EventSource
		// connections, which cannot set custom headers.
		if tokenString == "" {
			tokenString = r.URL.Query().Get("token")
		}
		if tokenString == "" {
			SendError(w, errors.New("Missing token auth"), http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Don't forget to validate the alg is what you expect:

			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}

			return hmacSampleSecret, nil
		})

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			ctx := context.WithValue(r.Context(), "username", claims["username"].(string))
			log.Println("Logged in with username = ", claims["username"])
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			SendError(w, err, http.StatusUnauthorized)
			return
		}
	})
}

var verifyAuthHandler HttpHandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	username, ok := r.Context().Value("username").(string)
	if !ok {
		SendError(w, errors.New("Error getting username"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	SendJSON(w, struct {
		Validated bool
		Jwtoken   string
		Message   string
	}{Validated: true, Jwtoken: "", Message: `Username : ` + username})
}

var indexHandler = makeIndexHandler("/")

func makeIndexHandler(basePath string) HttpHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := gatesentryWebserverFrontend.GetIndexHtmlWithBasePath(basePath)
		if data == nil {
			SendError(w, errors.New("Error getting index.html"), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	}
}

func RegisterEndpointsStartServer(
	Filters *[]gatesentryFilters.GSFilter,
	runtime *gatesentryWebserverTypes.TemporaryRuntime,
	logger *gatesentry2logger.Log,
	dnsServerInfo *gatesentryTypes.DnsServerInfo,
	boundAddress *string,
	port string,
	internalSettings *gatesentry2storage.MapStore,
	ruleManager gatesentryWebserverEndpoints.RuleManagerInterface,
	basePath string,
) {

	// newRouter := mux.NewRouter()

	internalServer := NewGsWeb(basePath)

	// Apply blocked-domain middleware to the root router.
	// This intercepts requests from DNS-blocked domains (where the DNS server
	// resolved the blocked domain to GateSentry's IP) and serves a block page
	// instead of the admin UI. Must be on the root router so it runs before
	// any subrouter matching.
	internalServer.router.Use(blockedDomainMiddleware(internalSettings, port))

	internalServer.Post("/api/auth/token", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data User
		if err := ParseJSONRequest(r, &data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Error parsing json"))
			return
		}
		if !VerifyAdminUser(data.Username, data.Pass, internalSettings) {

			SendJSON(w, struct {
				Validated bool
			}{Validated: false})

			return
		}

		// On successful login, capture the host the admin used to reach us.
		// If wpad_proxy_host hasn't been configured yet, seed it — the admin
		// just proved this address works by logging in with it.
		if internalSettings.Get("wpad_proxy_host") == "" {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			// Only seed if it's not localhost — that wouldn't help other devices
			if host != "" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
				internalSettings.Update("wpad_proxy_host", host)
				log.Printf("[WPAD] Auto-detected proxy host from admin login: %s", host)
			}
		}

		ctx := context.WithValue(r.Context(), "username", data.Username)
		tokenCreationHandler(w, r.WithContext(ctx))
	}))

	internalServer.Get("/api/about", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseJson := gatesentryWebserverEndpoints.GSApiAboutGET(runtime)
		SendJSON(w, responseJson)
	}))

	internalServer.Get("/api/auth/verify", authenticationMiddleware, verifyAuthHandler)

	internalServer.Get("/api/filters", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		responseJson := gatesentryWebserverEndpoints.GetAllFilters(Filters)
		SendJSON(w, responseJson)
	})
	internalServer.Get("/api/filters/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestedId := vars["id"]
		responseJson := gatesentryWebserverEndpoints.GetSingleFilter(requestedId, Filters)
		SendJSON(w, responseJson)
	})
	internalServer.Post("/api/filters/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestedId := vars["id"]
		var dataReceived []gatesentryFilters.GsFilterLine
		ParseJSONRequest(r, &dataReceived)
		responseJson := gatesentryWebserverEndpoints.PostSingleFilter(requestedId, dataReceived, Filters)
		SendJSON(w, responseJson)
		runtime.Reload()
	})

	internalServer.Get("/api/settings/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestedId := vars["id"]
		jsonResponse := gatesentryWebserverEndpoints.GSApiSettingsGET(requestedId, internalSettings)
		SendJSON(w, jsonResponse)
	})

	internalServer.Post("/api/settings/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestedId := vars["id"]
		var temp gatesentryWebserverTypes.Datareceiver
		err := ParseJSONRequest(r, &temp)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		output := gatesentryWebserverEndpoints.GSApiSettingsPOST(requestedId, internalSettings, temp)
		runtime.Reload()
		SendJSON(w, output)
	})

	internalServer.Get("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		jsonResponse := gatesentryWebserverEndpoints.GSApiUsersGET(runtime, internalSettings.Get("authusers"))
		SendJSON(w, jsonResponse)
	})

	internalServer.Put("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var userJson gatesentryWebserverEndpoints.UserInputJsonSingle
		err := ParseJSONRequest(r, &userJson)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse := gatesentryWebserverEndpoints.GSApiUserPUT(internalSettings, userJson)
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Delete("/api/users/{username}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		username := vars["username"]
		jsonResponse := gatesentryWebserverEndpoints.GSApiUserDELETE(username, internalSettings)
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Post("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var userJson gatesentryWebserverEndpoints.UserInputJsonSingle
		err := ParseJSONRequest(r, &userJson)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse := gatesentryWebserverEndpoints.GSApiUserCreate(userJson, internalSettings)
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Get("/api/consumption", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		data := string(runtime.GetUserGetJSON())
		output := gatesentryWebserverEndpoints.GSApiConsumptionGET(data, internalSettings, runtime)
		SendJSON(w, output)
	})

	internalServer.Post("/api/consumption", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var temp gatesentryWebserverEndpoints.Datareceiver
		err := ParseJSONRequest(r, &temp)
		if err != nil {
			return
		}
		output := gatesentryWebserverEndpoints.GSApiConsumptionPOST(temp, internalSettings, runtime)
		SendJSON(w, output)
	})

	internalServer.Get("/api/logs/{id}", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		searchValue := queryParams.Get("search")

		if searchValue != "" {
			output := gatesentryWebserverEndpoints.ApiLogsSearchGET(logger, searchValue)
			SendJSON(w, output)
			return
		}

		output := gatesentryWebserverEndpoints.ApiLogsGET(logger)
		SendJSON(w, output)
	}))

	internalServer.Get("/api/dns/info", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.GSApiDNSInfo(dnsServerInfo)
		SendJSON(w, output)
	})

	internalServer.Get("/api/dns/custom_entries", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		data := internalSettings.Get("DNS_custom_entries")
		output := gatesentryWebserverEndpoints.GSApiDNSEntriesCustom(data, internalSettings, runtime)
		SendJSON(w, output)
	})

	internalServer.Post("/api/dns/custom_entries", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var customEntries []gatesentryTypes.DNSCustomEntry
		err := ParseJSONRequest(r, &customEntries)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		output := gatesentryWebserverEndpoints.GSApiDNSSaveEntriesCustom(customEntries, internalSettings, runtime)
		SendJSON(w, output)
		runtime.Reload()
	})

	// DNS cache stats and SSE event stream
	internalServer.Get("/api/dns/cache/stats", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDNSCacheStats(w, r)
	})
	internalServer.Get("/api/dns/cache/stats/history", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDNSCacheHistory(w, r)
	})
	internalServer.Post("/api/dns/cache/flush", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDNSCacheFlush(w, r)
	})
	internalServer.Get("/api/dns/events", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDNSEvents(w, r)
	})

	internalServer.Post("/api/stats", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		fromTimeParam := params["fromTime"]
		output := gatesentryWebserverEndpoints.ApiGetStats(fromTimeParam, logger)
		SendJSON(w, output)
	})

	internalServer.Get("/api/status", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.ApiGetStatus(logger, boundAddress, internalSettings)
		SendJSON(w, output)
	})

	internalServer.Get("/api/stats/byUrl", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		seconds, group := gatesentryWebserverEndpoints.ParseStatsQuery(r)
		output := gatesentryWebserverEndpoints.ApiGetStatsByURL(logger, seconds, group)
		SendJSON(w, output)
	})

	internalServer.Get("/api/toggleServer/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id := params["id"]
		output := gatesentryWebserverEndpoints.ApiToggleServer(id, logger)
		SendJSON(w, output)
	})

	internalServer.Get("/api/certificate/info", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.GetCertificateInfo(internalSettings)
		SendJSON(w, output)
	})

	internalServer.Post("/api/certificate/generate", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.GSApiCertificateGenerate(internalSettings)
		SendJSON(w, output)
	})

	internalServer.Get("/api/files/certificate", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.GetCertificateBytes(internalSettings)
		w.Header().Set("Content-Disposition", "attachment; filename=gatesentry-ca.crt")
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Write(output)
	}))

	// Register rule endpoints with authentication
	log.Println("Initializing rule manager...")
	gatesentryWebserverEndpoints.InitRuleManager(ruleManager)
	log.Println("Rule manager initialized")

	log.Println("Registering GET /api/rules...")
	internalServer.Get("/api/rules", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRulesGetAll(w, r)
	})

	log.Println("Registering POST /api/rules...")
	internalServer.Post("/api/rules", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRuleCreate(w, r)
	})

	log.Println("Registering GET /api/rules/{id}...")
	internalServer.Get("/api/rules/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRuleGet(w, r)
	})

	log.Println("Registering PUT /api/rules/{id}...")
	internalServer.Put("/api/rules/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRuleUpdate(w, r)
	})

	log.Println("Registering DELETE /api/rules/{id}...")
	internalServer.Delete("/api/rules/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRuleDelete(w, r)
	})

	log.Println("Registering POST /api/rules/test...")
	internalServer.Post("/api/rules/test", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRuleTest(w, r)
	})
	log.Println("All rule endpoints registered successfully")

	// Device inventory endpoints
	log.Println("Registering device API endpoints...")
	internalServer.Get("/api/devices", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDevicesGetAll(w, r)
	})
	internalServer.Get("/api/devices/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceGet(w, r)
	})
	internalServer.Post("/api/devices/{id}/name", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceSetName(w, r)
	})
	internalServer.Delete("/api/devices/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceDelete(w, r)
	})
	log.Println("Device API endpoints registered")

	// --- WPAD / PAC file endpoint ---
	// Registered on the ROOT router (not the basePath subrouter) because:
	// 1. WPAD auto-discovery always fetches http://wpad.<domain>/wpad.dat (root path)
	// 2. No authentication — all network clients must be able to fetch the PAC file
	// 3. Must work regardless of GS_BASE_PATH configuration
	log.Println("Registering WPAD/PAC endpoints...")
	wpadHandler := gatesentryWebserverEndpoints.GSApiWPADHandler(internalSettings)
	internalServer.router.HandleFunc("/wpad.dat", wpadHandler).Methods("GET")
	internalServer.router.HandleFunc("/proxy.pac", wpadHandler).Methods("GET")
	log.Println("WPAD/PAC endpoints registered: /wpad.dat, /proxy.pac")

	// Authenticated WPAD info endpoint (for admin UI)
	internalServer.Get("/api/wpad/info", authenticationMiddleware,
		HttpHandlerFunc(gatesentryWebserverEndpoints.GSApiWPADInfoHandler(internalSettings, port)))

	// Serve static assets from the embedded files/fs/ directory.
	// GetFSHandler() returns fs.Sub(build, "files"), so files live at fs/bundle.js etc.
	// We only strip the basePath prefix (not /fs), so the remaining path /fs/bundle.js
	// correctly maps to fs/bundle.js in the embedded filesystem.
	fsHandler := http.FileServer(gatesentryWebserverFrontend.GetFSHandler())
	if basePath != "/" {
		internalServer.sub.PathPrefix("/fs/").Handler(
			http.StripPrefix(basePath, fsHandler),
		)
	} else {
		internalServer.sub.PathPrefix("/fs/").Handler(fsHandler)
	}

	// --- Root-level static files (favicon, logo, etc.) ---
	// These must be registered before the SPA catch-all routes so they
	// are matched first. No authentication required.
	internalServer.sub.HandleFunc("/gatesentry.svg",
		gatesentryWebserverFrontend.RootFileHandler("gatesentry.svg")).Methods("GET")
	internalServer.sub.HandleFunc("/favicon.ico",
		gatesentryWebserverFrontend.RootFileHandler("favicon.ico")).Methods("GET")

	baseIndexHandler := makeIndexHandler(basePath)
	internalServer.Get("/", baseIndexHandler)
	internalServer.Get("/login", baseIndexHandler)
	internalServer.Get("/stats", baseIndexHandler)
	internalServer.Get("/users", baseIndexHandler)
	internalServer.Get("/dns", baseIndexHandler)
	internalServer.Get("/settings", baseIndexHandler)
	internalServer.Get("/rules", baseIndexHandler)
	internalServer.Get("/logs", baseIndexHandler)
	internalServer.Get("/blockedkeywords", baseIndexHandler)
	internalServer.Get("/blockedfiletypes", baseIndexHandler)
	internalServer.Get("/excludeurls", baseIndexHandler)
	internalServer.Get("/blockedurls", baseIndexHandler)
	internalServer.Get("/excludehosts", baseIndexHandler)
	internalServer.Get("/services", baseIndexHandler)
	internalServer.Get("/devices", baseIndexHandler)
	internalServer.Get("/ai", baseIndexHandler)

	internalServer.ListenAndServe(":" + port)

}
