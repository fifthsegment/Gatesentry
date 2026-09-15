package gatesentryWebserver

import (
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func authStore(t *testing.T) *gatesentry2storage.MapStore {
	t.Helper()
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("general_settings", "{\"log_location\":\"./log.db\"}"); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestFreshSetupIsOneTimeAndPersistent(t *testing.T) {
	store := authStore(t)
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := auth.Verify("admin", "admin"); err != nil || ok {
		t.Fatalf("default login = %v, %v", ok, err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap("other", "another-long-password", "", true); !errors.Is(err, errSetupComplete) {
		t.Fatalf("reuse = %v", err)
	}
	if ok, err := auth.Verify("owner", "long-secure-password"); err != nil || !ok {
		t.Fatalf("login = %v, %v", ok, err)
	}
	raw := store.GetOrDefault(authStateKey, "")
	if strings.Contains(raw, "long-secure-password") || !strings.Contains(raw, "\"password_hash\":\"$2") {
		t.Fatalf("unsafe auth state")
	}
	restarted, err := NewAuthManager(store, filepath.Join(t.TempDir(), "removed"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := restarted.Verify("owner", "long-secure-password"); !ok {
		t.Fatal("credential did not survive restart")
	}
}

func TestExactlyOneConcurrentBootstrapWins(t *testing.T) {
	auth, err := NewAuthManager(authStore(t), "")
	if err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if auth.Bootstrap("owner", "long-secure-password", "", true) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("winners = %d", wins.Load())
	}
}

func TestLegacyMigrationAndSessionInvalidation(t *testing.T) {
	store := authStore(t)
	if err := store.Update("general_settings", "{\"log_location\":\"./log.db\",\"admin_username\":\"admin\",\"admin_password\":\"admin\"}"); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.Verify("admin", "admin"); !ok {
		t.Fatal("legacy owner locked out")
	}
	general := store.GetOrDefault("general_settings", "")
	if strings.Contains(general, "admin_password") {
		t.Fatalf("plaintext retained")
	}
	token, err := auth.CreateToken("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.VerifyToken(token); err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangeCredentials("owner", "replacement-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.VerifyToken(token); err == nil {
		t.Fatal("old token remained valid")
	}
	newToken, err := auth.CreateToken("owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.VerifyToken(newToken); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyMigrationPreservesPreviouslyAcceptedCredentialShapes(t *testing.T) {
	for _, tc := range []struct {
		name, username, password string
	}{
		{name: "empty password", username: "owner", password: ""},
		{name: "oversized password", username: "owner", password: strings.Repeat("p", 100)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := authStore(t)
			legacyJSON, err := json.Marshal(map[string]string{
				"log_location": "./log.db", "admin_username": tc.username, "admin_password": tc.password,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Update("general_settings", string(legacyJSON)); err != nil {
				t.Fatal(err)
			}
			auth, err := NewAuthManager(store, "")
			if err != nil {
				t.Fatal(err)
			}
			if ok, err := auth.Verify(tc.username, tc.password); err != nil || !ok {
				t.Fatalf("legacy login = %v, %v", ok, err)
			}
		})
	}
}

func TestBootstrapAuthorizationAndSecretFile(t *testing.T) {
	store := authStore(t)
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte("{\"authorization\":\"one-time-bootstrap-authorization-1234\"}"), 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "wrong", false); !errors.Is(err, errSetupAuth) {
		t.Fatalf("wrong auth = %v", err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", false); !errors.Is(err, errSetupAuth) {
		t.Fatalf("missing auth = %v", err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "one-time-bootstrap-authorization-1234", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(store.GetOrDefault(authStateKey, ""), "one-time-bootstrap-authorization-1234") {
		t.Fatal("bootstrap secret persisted")
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "one-time-bootstrap-authorization-1234", false); !errors.Is(err, errSetupComplete) {
		t.Fatalf("reused auth = %v", err)
	}
}

func TestUnattendedBootstrapAndTrustBoundary(t *testing.T) {
	remoteAuth, err := NewAuthManager(authStore(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := remoteAuth.Bootstrap("owner", "long-secure-password", "", false); !errors.Is(err, errSetupAuth) {
		t.Fatalf("remote setup without authorization = %v", err)
	}

	store := authStore(t)
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte("{\"username\":\"owner\",\"password\":\"long-secure-password\"}"), 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, path)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.Verify("owner", "long-secure-password"); !ok {
		t.Fatal("unattended setup failed")
	}
	for _, addr := range []string{"127.0.0.1:1234", "[::1]:1234"} {
		r := httptest.NewRequest("POST", "/api/setup", nil)
		r.RemoteAddr = addr
		r.Header.Set("X-Forwarded-For", "203.0.113.2")
		if !requestIsLoopback(r) {
			t.Fatalf("loopback rejected: %s", addr)
		}
	}
	r := httptest.NewRequest("POST", "/api/setup", nil)
	r.RemoteAddr = "192.0.2.5:1234"
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	if requestIsLoopback(r) {
		t.Fatal("proxy header trusted")
	}
}

func TestUnattendedBootstrapResumesIncompleteStateAtomically(t *testing.T) {
	store := authStore(t)
	if _, err := NewAuthManager(store, ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte(`{"username":"owner","password":"long-secure-password"}`), 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, path)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := auth.Verify("owner", "long-secure-password"); err != nil || !ok {
		t.Fatalf("unattended restart login = %v, %v", ok, err)
	}
}

func TestUnattendedBootstrapRejectsMissingAndConflictingInput(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := NewAuthManager(authStore(t), filepath.Join(t.TempDir(), "missing.json")); err == nil {
			t.Fatal("expected missing bootstrap file error")
		}
	})
	t.Run("persisted authorization conflicts with credentials", func(t *testing.T) {
		store := authStore(t)
		tokenPath := filepath.Join(t.TempDir(), "token.json")
		if err := os.WriteFile(tokenPath, []byte(`{"authorization":"one-time-bootstrap-authorization-1234"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewAuthManager(store, tokenPath); err != nil {
			t.Fatal(err)
		}
		credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
		if err := os.WriteFile(credentialsPath, []byte(`{"username":"owner","password":"long-secure-password"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewAuthManager(store, credentialsPath); err == nil {
			t.Fatal("expected conflicting bootstrap mode error")
		}
		complete, requiresAuthorization, err := (&AuthManager{store: store}).Status()
		if err != nil || complete || !requiresAuthorization {
			t.Fatalf("state changed after conflict: complete=%v auth=%v err=%v", complete, requiresAuthorization, err)
		}
	})
}

func TestSetupHTTPTrustAuthorizationAndSecretRedaction(t *testing.T) {
	store := authStore(t)
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	const secret = "one-time-bootstrap-authorization-1234"
	if err := os.WriteFile(path, []byte(`{"authorization":"`+secret+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, path)
	if err != nil {
		t.Fatal(err)
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	statusRequest.RemoteAddr = "192.0.2.8:1234"
	statusResponse := httptest.NewRecorder()
	setupStatusHandlerFor(auth)(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || strings.Contains(statusResponse.Body.String(), secret) {
		t.Fatalf("unsafe status response: %d %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status struct {
		Complete              bool `json:"complete"`
		RequiresAuthorization bool `json:"requires_authorization"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil || status.Complete || !status.RequiresAuthorization {
		t.Fatalf("unexpected setup status: %+v, %v", status, err)
	}

	requestBody := func(authz string) *bytes.Reader {
		body, _ := json.Marshal(map[string]string{"username": "owner", "password": "long-secure-password", "authorization": authz})
		return bytes.NewReader(body)
	}
	wrongRequest := httptest.NewRequest(http.MethodPost, "/api/setup", requestBody("wrong-bootstrap-value"))
	wrongRequest.RemoteAddr = "192.0.2.8:1234"
	wrongResponse := httptest.NewRecorder()
	setupHandlerFor(auth)(wrongResponse, wrongRequest)
	if wrongResponse.Code != http.StatusForbidden || strings.Contains(wrongResponse.Body.String(), "wrong-bootstrap-value") {
		t.Fatalf("unsafe rejection: %d %s", wrongResponse.Code, wrongResponse.Body.String())
	}

	goodRequest := httptest.NewRequest(http.MethodPost, "/api/setup", requestBody(secret))
	goodRequest.RemoteAddr = "192.0.2.8:1234"
	goodResponse := httptest.NewRecorder()
	setupHandlerFor(auth)(goodResponse, goodRequest)
	if goodResponse.Code != http.StatusOK || strings.Contains(goodResponse.Body.String(), secret) || strings.Contains(goodRequest.URL.String(), secret) {
		t.Fatalf("unsafe setup result: %d %s", goodResponse.Code, goodResponse.Body.String())
	}
}

func TestAuthenticationMiddlewareRejectsPreSetupAndInvalidatedSessions(t *testing.T) {
	store := authStore(t)
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	protected := authenticationMiddlewareFor(auth)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/settings/general_settings", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("pre-setup status = %d", response.Code)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err != nil {
		t.Fatal(err)
	}
	token, err := auth.CreateToken("owner")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/settings/general_settings", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d", response.Code)
	}
	if err := auth.ChangeCredentialsAndGeneral("owner", "replacement-password", gatesentryWebserverTypes.GSGeneral_Settings{LogLocation: "updated.db"}); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/settings/general_settings", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalidated token status = %d", response.Code)
	}
	if raw := store.GetOrDefault("general_settings", ""); strings.Contains(raw, "replacement-password") || strings.Contains(raw, "admin_password") {
		t.Fatalf("plaintext credential persisted: %s", raw)
	}
	usernameToken, err := auth.CreateToken("owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangeCredentialsAndGeneral("renamed-owner", "", gatesentryWebserverTypes.GSGeneral_Settings{LogLocation: "updated-again.db"}); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.VerifyToken(usernameToken); err == nil {
		t.Fatal("username change did not invalidate prior token")
	}
	if ok, err := auth.Verify("renamed-owner", "replacement-password"); err != nil || !ok {
		t.Fatalf("renamed login = %v, %v", ok, err)
	}
}

func TestHashFailureDoesNotCompleteSetup(t *testing.T) {
	store := authStore(t)
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	original := passwordHash
	passwordHash = func([]byte) ([]byte, error) { return nil, errors.New("injected") }
	t.Cleanup(func() { passwordHash = original })
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err == nil {
		t.Fatal("expected hash error")
	}
	complete, _, err := auth.Status()
	if err != nil || complete {
		t.Fatalf("complete=%v err=%v", complete, err)
	}
}

func TestLegacyHashFailurePreservesPlaintextForRetry(t *testing.T) {
	store := authStore(t)
	legacy := `{"log_location":"./log.db","admin_username":"admin","admin_password":"admin"}`
	if err := store.Update("general_settings", legacy); err != nil {
		t.Fatal(err)
	}
	original := passwordHash
	passwordHash = func([]byte) ([]byte, error) { return nil, errors.New("injected") }
	_, err := NewAuthManager(store, "")
	passwordHash = original
	if err == nil {
		t.Fatal("expected legacy hash error")
	}
	if got := store.GetOrDefault("general_settings", ""); got != legacy {
		t.Fatalf("legacy credentials changed after failed migration: %s", got)
	}
	if got := store.GetOrDefault(authStateKey, ""); got != "" {
		t.Fatalf("authentication state persisted after failed migration: %s", got)
	}
}

func TestMalformedAndUnsafeBootstrapFiles(t *testing.T) {
	cases := []struct {
		name, body string
		mode       os.FileMode
	}{
		{"malformed", "{", 0600},
		{"empty", "{}", 0600},
		{"incomplete credentials", "{\"username\":\"owner\"}", 0600},
		{"unknown field", "{\"authorization\":\"one-time-bootstrap-authorization-1234\",\"extra\":true}", 0600},
		{"short authorization", "{\"authorization\":\"short\"}", 0600},
		{"ambiguous", "{\"authorization\":\"one-time-bootstrap-authorization-1234\",\"username\":\"owner\",\"password\":\"long-secure-password\"}", 0600},
		{"permissions", "{\"authorization\":\"one-time-bootstrap-authorization-1234\"}", 0644},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := authStore(t)
			path := filepath.Join(t.TempDir(), "bootstrap.json")
			if err := os.WriteFile(path, []byte(tc.body), tc.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := NewAuthManager(store, path); err == nil || strings.Contains(err.Error(), "one-time-bootstrap-authorization-1234") || strings.Contains(err.Error(), "long-secure-password") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestInconsistentAuthenticationStateIsRejected(t *testing.T) {
	store := authStore(t)
	secret, err := randomSecret()
	if err != nil {
		t.Fatal(err)
	}
	bad, err := json.Marshal(authState{Version: 1, BootstrapComplete: true, Username: "owner", SessionGeneration: 1, JWTSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(authStateKey, string(bad)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuthManager(store, ""); err == nil {
		t.Fatal("expected inconsistent authentication state to fail startup")
	}
}
