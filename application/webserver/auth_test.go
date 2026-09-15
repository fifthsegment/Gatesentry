package gatesentryWebserver

import (
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"bytes"
	"context"
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
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testBootstrapAuthorization = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
const weakBootstrapAuthorization = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

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
		{name: "empty username", username: "", password: "legacy-password"},
		{name: "empty username and password", username: "", password: ""},
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
			token, err := auth.CreateToken(tc.username)
			if err != nil {
				t.Fatalf("create migrated session: %v", err)
			}
			if got, err := auth.VerifyToken(token); err != nil || got != tc.username {
				t.Fatalf("verify migrated session = %q, %v", got, err)
			}
		})
	}
}

func TestLegacyMigrationPreservesUnrelatedGeneralSettings(t *testing.T) {
	store := authStore(t)
	legacy := `{"log_location":"./log.db","admin_username":"admin","admin_password":"admin","future_setting":{"enabled":true}}`
	if err := store.Update("general_settings", legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuthManager(store, ""); err != nil {
		t.Fatal(err)
	}
	var migrated map[string]json.RawMessage
	if err := json.Unmarshal([]byte(store.GetOrDefault("general_settings", "")), &migrated); err != nil {
		t.Fatal(err)
	}
	if _, ok := migrated["future_setting"]; !ok {
		t.Fatal("migration removed an unrelated general setting")
	}
	if _, ok := migrated["admin_username"]; ok {
		t.Fatal("migration retained legacy username")
	}
	if _, ok := migrated["admin_password"]; ok {
		t.Fatal("migration retained legacy password")
	}
}

func TestLegacyMigrationConsumesPersistedAuthorization(t *testing.T) {
	store := authStore(t)
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte(`{"authorization":"`+testBootstrapAuthorization+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuthManager(store, path); err != nil {
		t.Fatal(err)
	}
	if err := store.Update("general_settings", `{"log_location":"./log.db","admin_username":"admin","admin_password":"admin"}`); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	complete, requiresAuthorization, err := auth.Status()
	if err != nil || !complete || requiresAuthorization {
		t.Fatalf("migrated status: complete=%v authorization=%v err=%v", complete, requiresAuthorization, err)
	}
	if ok, err := auth.Verify("admin", "admin"); err != nil || !ok {
		t.Fatalf("migrated login = %v, %v", ok, err)
	}
}

func TestCompletedStateRemovesStaleLegacyCredentialsWithoutReadingBootstrap(t *testing.T) {
	store := authStore(t)
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Update("general_settings", `{"log_location":"./log.db","admin_username":"stale","admin_password":"stale-secret","future_setting":true}`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuthManager(store, filepath.Join(t.TempDir(), "missing-bootstrap.json")); err != nil {
		t.Fatal(err)
	}
	raw := store.GetOrDefault("general_settings", "")
	if strings.Contains(raw, "admin_username") || strings.Contains(raw, "admin_password") || !strings.Contains(raw, "future_setting") {
		t.Fatalf("stale credential cleanup = %s", raw)
	}
	if ok, err := auth.Verify("owner", "long-secure-password"); err != nil || !ok {
		t.Fatalf("established credentials changed: %v, %v", ok, err)
	}
}

func TestBootstrapAuthorizationAndSecretFile(t *testing.T) {
	store := authStore(t)
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte(`{"authorization":"`+testBootstrapAuthorization+`"}`), 0600); err != nil {
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
	if err := auth.Bootstrap("owner", "long-secure-password", testBootstrapAuthorization, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(store.GetOrDefault(authStateKey, ""), testBootstrapAuthorization) {
		t.Fatal("bootstrap secret persisted")
	}
	if err := auth.Bootstrap("owner", "long-secure-password", testBootstrapAuthorization, false); !errors.Is(err, errSetupComplete) {
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
		if !requestIsLoopback(r) {
			t.Fatalf("loopback rejected: %s", addr)
		}
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Real-IP"} {
		r := httptest.NewRequest("POST", "/api/setup", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set(header, "203.0.113.2")
		if requestIsLoopback(r) {
			t.Fatalf("forwarded loopback request trusted: %s", header)
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
		if err := os.WriteFile(tokenPath, []byte(`{"authorization":"`+testBootstrapAuthorization+`"}`), 0600); err != nil {
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
	const secret = testBootstrapAuthorization
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

func TestAuthenticationResponsesIncludeVerifiedUsername(t *testing.T) {
	auth, err := NewAuthManager(authStore(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/auth/token", nil)
	request = request.WithContext(context.WithValue(request.Context(), "username", "owner"))
	response := httptest.NewRecorder()
	tokenCreationHandlerFor(auth)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("token response status = %d", response.Code)
	}
	var tokenResponse struct {
		Jwtoken   string
		Validated bool
		Username  string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &tokenResponse); err != nil {
		t.Fatal(err)
	}
	if !tokenResponse.Validated || tokenResponse.Jwtoken == "" || tokenResponse.Username != "owner" {
		t.Fatalf("token response = %+v", tokenResponse)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/auth/verify", nil)
	request = request.WithContext(context.WithValue(request.Context(), "username", "owner"))
	response = httptest.NewRecorder()
	verifyAuthHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("verify response status = %d", response.Code)
	}
	var verifyResponse struct {
		Validated bool
		Username  string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &verifyResponse); err != nil {
		t.Fatal(err)
	}
	if !verifyResponse.Validated || verifyResponse.Username != "owner" {
		t.Fatalf("verify response = %+v", verifyResponse)
	}
}

func TestSessionRequiresBoundedRegisteredClaims(t *testing.T) {
	store := authStore(t)
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err != nil {
		t.Fatal(err)
	}
	state, err := auth.read()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		claims sessionClaims
	}{
		{name: "missing registered claims", claims: sessionClaims{Username: state.Username, Generation: state.SessionGeneration}},
		{name: "missing expiration", claims: sessionClaims{
			Username: state.Username, Generation: state.SessionGeneration,
			RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(time.Now()), NotBefore: jwt.NewNumericDate(time.Now())},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tc.claims).SignedString([]byte(state.JWTSecret))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := auth.VerifyToken(token); err == nil {
				t.Fatal("session without all bounded registered claims was accepted")
			}
		})
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

func TestBootstrapPersistenceFailureDoesNotGrantAccess(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("general_settings", `{"log_location":"./log.db"}`); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	// Replacing the storage directory with a regular file forces the atomic
	// write to fail before its rename commit point.
	path := filepath.Join(dir, "settings")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(path), []byte("unwritable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err == nil {
		t.Fatal("expected bootstrap persistence error")
	}
	if ok, err := auth.Verify("owner", "long-secure-password"); err == nil || ok {
		t.Fatalf("credentials became usable after failed persistence: ok=%v err=%v", ok, err)
	}
}

type postRenameFailureStore struct {
	delegate *gatesentry2storage.MapStore
	blocked  error
}

func (s *postRenameFailureStore) GetE(key string) (string, error) {
	value, err := s.delegate.GetE(key)
	if s.blocked != nil {
		return value, s.blocked
	}
	return value, err
}

func (s *postRenameFailureStore) UpdateMap(update func(map[string]string) error) error {
	return s.delegate.UpdateMap(update)
}

func (s *postRenameFailureStore) UpdateValue(key string, update func(string) (string, error)) error {
	if err := s.delegate.UpdateValue(key, update); err != nil {
		return err
	}
	s.blocked = errors.New("injected post-rename directory sync failure")
	return s.blocked
}

func TestBootstrapPostRenameDurabilityFailureDoesNotGrantAccess(t *testing.T) {
	store := authStore(t)
	auth, err := NewAuthManager(store, "")
	if err != nil {
		t.Fatal(err)
	}
	auth.store = &postRenameFailureStore{delegate: store}
	if err := auth.Bootstrap("owner", "long-secure-password", "", true); err == nil {
		t.Fatal("expected post-rename durability error")
	}
	if ok, err := auth.Verify("owner", "long-secure-password"); err == nil || ok {
		t.Fatalf("credentials became usable after durability failure: ok=%v err=%v", ok, err)
	}
	if _, err := auth.CreateToken("owner"); err == nil {
		t.Fatal("session creation succeeded after durability failure")
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
		{"unknown field", `{"authorization":"` + testBootstrapAuthorization + `","extra":true}`, 0600},
		{"short authorization", "{\"authorization\":\"short\"}", 0600},
		{"weak authorization", `{"authorization":"` + weakBootstrapAuthorization + `"}`, 0600},
		{"ambiguous", `{"authorization":"` + testBootstrapAuthorization + `","username":"owner","password":"long-secure-password"}`, 0600},
		{"permissions", `{"authorization":"` + testBootstrapAuthorization + `"}`, 0644},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := authStore(t)
			path := filepath.Join(t.TempDir(), "bootstrap.json")
			if err := os.WriteFile(path, []byte(tc.body), tc.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := NewAuthManager(store, path); err == nil || strings.Contains(err.Error(), testBootstrapAuthorization) || strings.Contains(err.Error(), "long-secure-password") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.json")
		path := filepath.Join(dir, "bootstrap.json")
		if err := os.WriteFile(target, []byte(`{"authorization":"`+testBootstrapAuthorization+`"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, err := NewAuthManager(authStore(t), path); err == nil {
			t.Fatal("expected symlink bootstrap file error")
		}
	})
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
