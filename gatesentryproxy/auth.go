package gatesentryproxy

import (
	"net/http"
	"strings"
	"time"
)

const userCacheTTL = 5 * time.Minute

func ProxyCredentials(r *http.Request) (user, pass string, ok bool) {
	auth := r.Header.Get("Proxy-Authorization")

	if val, okP := IProxy.UsersCache.Load(auth); okP {
		cached := val.(GSUserCached)
		if time.Now().Unix()-cached.CachedAt < int64(userCacheTTL.Seconds()) {
			return cached.User, cached.Pass, true
		}
		// Expired, remove and re-decode
		IProxy.UsersCache.Delete(auth)
	}

	if auth == "" || !strings.HasPrefix(auth, "Basic ") {
		return "", "", false
	}

	nuser, npass, nok := decodeBase64Credentials(strings.TrimPrefix(auth, "Basic "))
	gsu := GSUserCached{User: nuser, Pass: npass, CachedAt: time.Now().Unix()}
	IProxy.UsersCache.Store(auth, gsu)
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
			isauth := IProxy.AuthHandler(r.Header.Get("Proxy-Authorization"))
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
