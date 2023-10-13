package gatesentryproxy

import (
	"net/http"
	"strings"
)

func ProxyCredentials(r *http.Request) (user, pass string, ok bool) {
	auth := r.Header.Get("Proxy-Authorization")

	if val, okP := IProxy.UsersCache[auth]; okP {
		return val.User, val.Pass, true
	}

	if auth == "" || !strings.HasPrefix(auth, "Basic ") {
		return "", "", false
	}

	nuser, npass, nok := decodeBase64Credentials(strings.TrimPrefix(auth, "Basic "))
	gsu := GSUserCached{User: nuser, Pass: npass}
	IProxy.UsersCache[auth] = gsu
	return nuser, npass, nok
}

func HandleAuthAndAssignUser(r *http.Request, passthru *GSProxyPassthru, h ProxyHandler, authEnabled bool, defaultUser string) (user string, pass string, authUser string) {
	authUser = defaultUser
	user = ""
	pass = ""
	if authEnabled {
		ok := false
		user, pass, ok = ProxyCredentials(r)
		if ok {
			// Verify Credentials here
			authUser = user
			temp := []byte(r.Header.Get("Proxy-Authorization"))
			isauth, _ := IProxy.RunHandler("isauthuser", "", &temp, passthru)
			if !isauth {
				user = ""
				return user, pass, authUser
			}
			_ = pass
		}

		if h.user != "" {
			authUser = h.user
			user = h.user
		}
	}
	if authUser != "" {
		user = authUser
	}

	passthru.User = user
	return user, pass, authUser
}
