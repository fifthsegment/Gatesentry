package gatesentryWebserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"

	gatesentryDiagnostics "bitbucket.org/abdullah_irfan/gatesentryf/diagnostics"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverEndpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"
	gatesentryWebserverFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/gorilla/mux"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Pass     string `json:"pass"`
}

type ErrorResponse struct {
	StatusCode   int    `json:"status"`
	ErrorMessage string `json:"message"`
}

type OkResponse struct {
	Response string `json:"Response"`
}

func authenticationMiddlewareFor(auth *AuthManager) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tokenString == "" {
				SendError(w, errors.New("missing authentication"), http.StatusUnauthorized)
				return
			}
			username, err := auth.VerifyToken(tokenString)
			if err != nil {
				SendError(w, errors.New("invalid authentication"), http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "username", username)))
		})
	}
}

func tokenCreationHandlerFor(auth *AuthManager) HttpHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		username, ok := r.Context().Value("username").(string)
		if !ok {
			SendError(w, errors.New("missing username"), http.StatusInternalServerError)
			return
		}
		token, err := auth.CreateToken(username)
		if err != nil {
			SendError(w, errors.New("unable to create session"), http.StatusInternalServerError)
			return
		}
		SendJSON(w, struct {
			Jwtoken   string
			Validated bool
			Username  string
		}{Jwtoken: token, Validated: true, Username: username})
	}
}

func setupStatusHandlerFor(auth *AuthManager) HttpHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		complete, err := auth.Status()
		if err != nil {
			SendError(w, errors.New("unable to read setup status"), http.StatusInternalServerError)
			return
		}
		SendJSON(w, struct {
			Complete bool `json:"complete"`
		}{complete})
	}
}

func setupHandlerFor(auth *AuthManager) HttpHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var data struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := ParseJSONRequest(r, &data); err != nil {
			SendError(w, errors.New("invalid setup request"), http.StatusBadRequest)
			return
		}
		if err := auth.Bootstrap(data.Username, data.Password); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errSetupComplete) {
				status = http.StatusConflict
			}
			SendError(w, errors.New("setup could not be completed"), status)
			return
		}
		SendJSON(w, struct {
			Complete bool `json:"complete"`
		}{true})
	}
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
		Username  string
	}{Validated: true, Jwtoken: "", Message: `Username : ` + username, Username: username})
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

func registerDashboardAssetRoutes(router *mux.Router) {
	assetHandler := gatesentryWebserverFrontend.GetAssetHandler()
	router.PathPrefix("/fs/").Handler(assetHandler)
	router.Path("/vite.svg").Handler(assetHandler)
}

// healthHandler reports web-server readiness. It is intentionally
// unauthenticated and returns no configuration, credential, or browsing
// material so container health checks and load balancers can probe it safely.
var healthHandler HttpHandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	SendJSON(w, map[string]string{"status": "ok"})
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
	devices *gatesentry2storage.MapStore,
	dataDir string,
) error {
	auth, err := NewAuthManager(internalSettings, os.Getenv("GATESENTRY_BOOTSTRAP_FILE"))
	if err != nil {
		return fmt.Errorf("initialize administrator authentication: %w", err)
	}
	// First-run setup accepts the first client that reaches the dashboard. Keep
	// that window visible in the startup log so operators finish setup promptly
	// or restrict access to the admin port.
	if complete, statusErr := auth.Status(); statusErr != nil {
		log.Printf("WARNING: unable to read first-run setup status: %v", statusErr)
	} else if !complete {
		log.Printf("WARNING: first-run setup is not complete. Anyone who can reach the dashboard on port %s can create the administrator account. Complete setup now, or restrict access to this port.", port)
	}
	authenticationMiddleware := authenticationMiddlewareFor(auth)
	tokenCreationHandler := tokenCreationHandlerFor(auth)

	internalServer := NewGsWeb(basePath)
	internalServer.Get("/health", healthHandler)
	internalServer.Get("/api/setup/status", setupStatusHandlerFor(auth))
	internalServer.Post("/api/setup", setupHandlerFor(auth))

	internalServer.Post("/api/auth/token", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var data User
		if err := ParseJSONRequest(r, &data); err != nil {
			SendError(w, errors.New("invalid login request"), http.StatusBadRequest)
			return
		}
		verified, err := auth.Verify(data.Username, data.Pass)
		if err != nil {
			SendError(w, errors.New("unable to verify login"), http.StatusInternalServerError)
			return
		}
		if !verified {

			SendJSON(w, struct {
				Validated bool
			}{Validated: false})

			return
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
		jsonResponse, err := gatesentryWebserverEndpoints.GSApiSettingsGET(requestedId, internalSettings)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, jsonResponse)
	})

	internalServer.Get("/api/ai/status", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiAIStatusGET(w, r, internalSettings)
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
		if requestedId == "general_settings" {
			var submitted gatesentryWebserverTypes.GSGeneral_Settings
			if err := json.Unmarshal([]byte(temp.Value), &submitted); err != nil {
				SendError(w, errors.New("invalid general settings"), http.StatusBadRequest)
				return
			}
			if submitted.AdminUser != "" || submitted.AdminPassword != "" {
				if err := auth.ChangeCredentialsAndGeneral(submitted.AdminUser, submitted.AdminPassword, submitted); err != nil {
					SendError(w, errors.New("unable to update administrator credentials"), http.StatusBadRequest)
					return
				}
				submitted.AdminUser, submitted.AdminPassword = "", ""
				clean, err := json.Marshal(submitted)
				if err != nil {
					SendError(w, errors.New("unable to encode updated settings"), http.StatusInternalServerError)
					return
				}
				temp.Value = string(clean)
				runtime.Reload()
				SendJSON(w, temp)
				return
			}
		}
		output, err := gatesentryWebserverEndpoints.GSApiSettingsPOST(requestedId, internalSettings, temp)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		runtime.Reload()
		SendJSON(w, output)
	})

	internalServer.Get("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		authUsers, err := internalSettings.GetE("authusers")
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse := gatesentryWebserverEndpoints.GSApiUsersGET(runtime, authUsers)
		SendJSON(w, jsonResponse)
	})

	internalServer.Put("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var userJson gatesentryWebserverEndpoints.UserInputJsonSingle
		err := ParseJSONRequest(r, &userJson)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse, err := gatesentryWebserverEndpoints.GSApiUserPUT(internalSettings, userJson)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Delete("/api/users/{username}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		username := vars["username"]
		jsonResponse, err := gatesentryWebserverEndpoints.GSApiUserDELETE(username, internalSettings)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
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
		jsonResponse, err := gatesentryWebserverEndpoints.GSApiUserCreate(userJson, internalSettings)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Get("/api/consumption", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		data := string(runtime.GetUserGetJSON())
		output, err := gatesentryWebserverEndpoints.GSApiConsumptionGET(data, internalSettings, runtime)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, output)
	})

	internalServer.Post("/api/consumption", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var temp gatesentryWebserverEndpoints.Datareceiver
		err := ParseJSONRequest(r, &temp)
		if err != nil {
			return
		}
		output, err := gatesentryWebserverEndpoints.GSApiConsumptionPOST(temp, internalSettings, runtime)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, output)
	})

	internalServer.Get("/api/logs/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		searchValue := queryParams.Get("search")

		if searchValue != "" {
			output := gatesentryWebserverEndpoints.ApiLogsSearchGET(logger, searchValue)
			SendJSON(w, output)
			return
		}

		output := gatesentryWebserverEndpoints.ApiLogsGET(logger)
		SendJSON(w, output)
	})

	internalServer.Get("/api/dns/info", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.GSApiDNSInfo(dnsServerInfo)
		SendJSON(w, output)
	})

	internalServer.Get("/api/onboarding/status", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiOnboardingStatusGET(w, r, gatesentryWebserverEndpoints.OnboardingDeps{
			Settings:      internalSettings,
			DnsServerInfo: dnsServerInfo,
		})
	})

	internalServer.Post("/api/onboarding/protection-check", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiOnboardingProtectionCheckPOST(w, r, gatesentryWebserverEndpoints.OnboardingDeps{
			Settings:      internalSettings,
			DnsServerInfo: dnsServerInfo,
		})
	})

	internalServer.Get("/api/dns/custom_entries", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		data, err := internalSettings.GetE("DNS_custom_entries")
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
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
		output, err := gatesentryWebserverEndpoints.GSApiDNSSaveEntriesCustom(customEntries, internalSettings, runtime)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, output)
		runtime.Reload()
	})

	internalServer.Post("/api/stats", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		fromTimeParam := params["fromTime"]
		output := gatesentryWebserverEndpoints.ApiGetStats(fromTimeParam, logger)
		SendJSON(w, output)
	})

	internalServer.Get("/api/status", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.ApiGetStatus(logger, boundAddress)
		SendJSON(w, output)
	})

	internalServer.Get("/api/stats/byUrl", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output := gatesentryWebserverEndpoints.ApiGetStatsByURL(logger)
		SendJSON(w, output)
	})

	internalServer.Get("/api/decisions", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDecisionsGET(w, r, logger)
	})

	internalServer.Get("/api/decisions/summary", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDecisionSummaryGET(w, r, logger)
	})

	internalServer.Get("/api/toggleServer/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id := params["id"]
		output := gatesentryWebserverEndpoints.ApiToggleServer(id, logger)
		SendJSON(w, output)
	})

	internalServer.Get("/api/certificate/info", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		output, err := gatesentryWebserverEndpoints.GetCertificateInfo(internalSettings)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		SendJSON(w, output)
	})

	internalServer.Get("/api/files/certificate", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		output, err := gatesentryWebserverEndpoints.GetCertificateBytes(internalSettings)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=certificate.pem")
		w.Header().Set("Content-Type", "application/octet-stream")
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
	internalServer.Get("/api/devices/{id}/policy", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDevicePolicyGet(w, r)
	})
	internalServer.Put("/api/devices/{id}/assignment", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceAssignmentSet(w, r)
	})
	internalServer.Get("/api/devices/{id}/activity", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceActivityGet(w, r, logger)
	})
	internalServer.Post("/api/devices/{id}/name", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceSetName(w, r)
	})
	internalServer.Delete("/api/devices/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDeviceDelete(w, r)
	})
	log.Println("Device API endpoints registered")

	// Policy group endpoints
	log.Println("Registering policy API endpoints...")
	internalServer.Get("/api/policy/templates", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyTemplatesGet(w, r)
	})
	internalServer.Post("/api/policy/templates/{id}/apply", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyTemplateApply(w, r)
	})
	internalServer.Get("/api/policy/groups", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyGroupsGet(w, r)
	})
	internalServer.Post("/api/policy/groups", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyGroupCreate(w, r)
	})
	internalServer.Put("/api/policy/groups", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyGroupsReplace(w, r)
	})
	internalServer.Put("/api/policy/groups/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyGroupUpdate(w, r)
	})
	internalServer.Delete("/api/policy/groups/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyGroupDelete(w, r)
	})
	internalServer.Get("/api/policy/assignments", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyAssignmentsGet(w, r)
	})
	internalServer.Put("/api/policy/assignments", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyAssignmentsReplace(w, r)
	})
	internalServer.Post("/api/policy/preview", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPolicyPreview(w, r)
	})

	// Exception and access-request endpoints (PER-38).
	log.Println("Registering exception and access-request API endpoints...")
	internalServer.Get("/api/exceptions", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiExceptionsGET(w, r)
	})
	internalServer.Post("/api/exceptions", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiExceptionCreate(w, r)
	})
	internalServer.Delete("/api/exceptions/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiExceptionRevoke(w, r)
	})
	internalServer.Post("/api/exceptions/preview", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiExceptionPreview(w, r)
	})
	internalServer.Get("/api/access-requests", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiAccessRequestsGET(w, r)
	})
	// Public, rate-limited access-request submission. Deliberately NOT behind
	// authenticationMiddleware so a blocked user can request review without
	// admin credentials. The handler rate-limits per client IP and never grants
	// access; only an admin approval creates an exception.
	internalServer.Post("/api/access-requests", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiAccessRequestSubmit(w, r)
	}))
	internalServer.Post("/api/access-requests/{id}/approve", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiAccessRequestApprove(w, r)
	})
	internalServer.Post("/api/access-requests/{id}/reject", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiAccessRequestReject(w, r)
	})
	log.Println("Exception and access-request API endpoints registered")

	// Pause and schedule endpoints (PER-39).
	log.Println("Registering pause and schedule API endpoints...")
	internalServer.Get("/api/pauses", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPausesGET(w, r)
	})
	internalServer.Post("/api/pauses", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPauseCreate(w, r)
	})
	internalServer.Delete("/api/pauses/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiPauseRevoke(w, r)
	})
	internalServer.Get("/api/policy/schedule-presets", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiSchedulePresetsGET(w, r)
	})
	internalServer.Post("/api/policy/schedule-presets/apply", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiSchedulePresetApply(w, r)
	})
	log.Println("Pause and schedule API endpoints registered")
	log.Println("Policy API endpoints registered")

	// Backup and restore endpoints (PER-41).
	log.Println("Registering backup and restore API endpoints...")
	// reloadAfterRestore refreshes runtime state and the DNS policy service so
	// that groups restored directly to storage are reflected in live enforcement.
	reloadAfterRestore := func() error {
		runtime.Reload()
		if svc := gatesentryDnsServer.GetPolicyService(); svc != nil {
			return svc.Reload()
		}
		return nil
	}
	internalServer.Get("/api/backup", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiBackupGET(w, r, gatesentryWebserverEndpoints.BackupDeps{
			Settings: internalSettings,
			Devices:  devices,
			BaseDir:  dataDir,
			Reload:   reloadAfterRestore,
			Version:  runtime.GetApplicationVersion,
		})
	})
	internalServer.Post("/api/restore", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiRestorePOST(w, r, gatesentryWebserverEndpoints.BackupDeps{
			Settings: internalSettings,
			Devices:  devices,
			BaseDir:  dataDir,
			Reload:   reloadAfterRestore,
			Version:  runtime.GetApplicationVersion,
		})
	})
	log.Println("Backup and restore API endpoints registered")

	// Gateway diagnostics and the redacted, previewable support bundle.
	diagnosticsDeps := gatesentryWebserverEndpoints.DiagnosticsDeps{
		Diagnostics: gatesentryDiagnostics.Deps{
			Settings:   internalSettings,
			DnsInfo:    dnsServerInfo,
			Filters:    Filters,
			GetDevices: gatesentryDnsServer.GetDeviceStore,
			GetPolicy:  gatesentryDnsServer.GetPolicyService,
		},
		ApplicationVersion: runtime.GetApplicationVersion,
	}
	internalServer.Get("/api/diagnostics", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDiagnosticsGET(w, r, diagnosticsDeps)
	})
	internalServer.Get("/api/diagnostics/bundle", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		gatesentryWebserverEndpoints.GSApiDiagnosticsBundleGET(w, r, diagnosticsDeps)
	})

	// Register MIME types for static file serving
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".svg", "image/svg+xml")

	// Serve dashboard assets at the paths emitted by the shared frontend build.
	registerDashboardAssetRoutes(internalServer.router)

	baseIndexHandler := makeIndexHandler(basePath)
	internalServer.Get("/", baseIndexHandler)
	internalServer.Get("/login", baseIndexHandler)
	internalServer.Get("/setup", baseIndexHandler)
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

	return internalServer.ListenAndServe(":" + port)

}
