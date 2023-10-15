package gatesentryWebserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverEndpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"
	gatesentryWebserverFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gorilla/mux"
)

var hmacSampleSecret = []byte("I7JE72S9XJ48ANXMI78ASDNMQ839")

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

func VerifyAdminUser(username string, password string, settingsStore *gatesentryWebserverTypes.SettingsStore) bool {
	if settingsStore.GetAdminUser() == username && settingsStore.GetAdminPassword() == password {
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

var indexHandler HttpHandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	data := gatesentryWebserverFrontend.GetIndexHtml()
	if data == nil {
		SendError(w, errors.New("Error getting index.html"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func RegisterEndpointsStartServer(Filters *[]gatesentryFilters.GSFilter,
	runtime *gatesentryWebserverTypes.TemporaryRuntime,
	settings *gatesentryWebserverTypes.SettingsStore,
	logger *gatesentry2logger.Log,
	boundAddress *string, port string) {

	// newRouter := mux.NewRouter()

	internalServer := NewGsWeb()

	internalServer.Post("/api/auth/token", HttpHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data User
		if err := ParseJSONRequest(r, &data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Error parsing json"))
			return
		}
		if !VerifyAdminUser(data.Username, data.Pass, settings) {

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
		var dataReceived []gatesentryFilters.GSFILTERLINE
		ParseJSONRequest(r, &dataReceived)
		responseJson := gatesentryWebserverEndpoints.PostSingleFilter(requestedId, dataReceived, Filters)
		SendJSON(w, responseJson)
		runtime.Reload()
	})

	internalServer.Get("/api/settings/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		requestedId := vars["id"]
		jsonResponse := gatesentryWebserverEndpoints.GSApiSettingsGET(requestedId, settings)
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
		output := gatesentryWebserverEndpoints.GSApiSettingsPOST(requestedId, settings, temp)
		SendJSON(w, output)
	})

	internalServer.Get("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		jsonResponse := gatesentryWebserverEndpoints.GSApiUsersGET(runtime, settings.GetSettings("authusers"))
		SendJSON(w, jsonResponse)
	})

	internalServer.Put("/api/users", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var userJson gatesentryWebserverEndpoints.UserInputJsonSingle
		err := ParseJSONRequest(r, &userJson)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		jsonResponse := gatesentryWebserverEndpoints.GSApiUserPUT(settings, userJson)
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Delete("/api/users/{username}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		username := vars["username"]
		jsonResponse := gatesentryWebserverEndpoints.GSApiUserDELETE(username, settings)
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
		jsonResponse := gatesentryWebserverEndpoints.GSApiUserCreate(userJson, settings)
		SendJSON(w, jsonResponse)
		runtime.Reload()
	})

	internalServer.Get("/api/consumption", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		data := string(runtime.GetUserGetJSON())
		output := gatesentryWebserverEndpoints.GSApiConsumptionGET(data, settings, runtime)
		SendJSON(w, output)
	})

	internalServer.Post("/api/consumption", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var temp gatesentryWebserverEndpoints.Datareceiver
		err := ParseJSONRequest(r, &temp)
		if err != nil {
			return
		}
		output := gatesentryWebserverEndpoints.GSApiConsumptionPOST(temp, settings, runtime)
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

	internalServer.Get("/api/dns/custom_entries", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		data := settings.Get("DNS_custom_entries")
		output := gatesentryWebserverEndpoints.GSApiDNSEntriesCustom(data, settings, runtime)
		SendJSON(w, output)
	})

	internalServer.Post("/api/dns/custom_entries", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		var customEntries []gatesentryTypes.DNSCustomEntry
		err := ParseJSONRequest(r, &customEntries)
		if err != nil {
			SendError(w, err, http.StatusInternalServerError)
			return
		}
		output := gatesentryWebserverEndpoints.GSApiDNSSaveEntriesCustom(customEntries, settings, runtime)
		SendJSON(w, output)
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

	internalServer.Get("/api/toggleServer/{id}", authenticationMiddleware, func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		id := params["id"]
		output := gatesentryWebserverEndpoints.ApiToggleServer(id, logger)
		SendJSON(w, output)
	})

	internalServer.router.PathPrefix("/fs/").Handler(
		http.StripPrefix("/fs",
			http.FileServer(
				gatesentryWebserverFrontend.GetFSHandler(),
			),
		),
	)

	internalServer.Get("/", indexHandler)
	internalServer.Get("/login", indexHandler)
	internalServer.Get("/stats", indexHandler)
	internalServer.Get("/users", indexHandler)
	internalServer.Get("/dns", indexHandler)
	internalServer.Get("/settings", indexHandler)

	internalServer.ListenAndServe(":" + port)

}
