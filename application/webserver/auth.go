package gatesentryWebserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const authStateKey = "admin_auth"

var (
	errSetupComplete = errors.New("setup is already complete")
	errSetupAuth     = errors.New("setup is not authorized")
	errInvalidLogin  = errors.New("invalid credentials")
	passwordHash     = func(password []byte) ([]byte, error) { return bcrypt.GenerateFromPassword(password, 12) }
)

// bcrypt accepts at most 72 bytes. New passwords are deliberately limited to
// that size, but legacy GateSentry accepted arbitrary strings. Pre-hash only
// oversized legacy values so every previously usable credential can migrate
// without weakening the normal bcrypt work factor.
func passwordHashInput(password string) []byte {
	if len(password) <= 72 {
		return []byte(password)
	}
	prehash := hmac.New(sha256.New, []byte("GateSentry legacy password prehash v1"))
	_, _ = prehash.Write([]byte(password))
	return prehash.Sum(nil)
}

type authState struct {
	Version              int    `json:"version"`
	BootstrapComplete    bool   `json:"bootstrap_complete"`
	Username             string `json:"username,omitempty"`
	PasswordHash         string `json:"password_hash,omitempty"`
	BootstrapTokenDigest string `json:"bootstrap_token_digest,omitempty"`
	SessionGeneration    uint64 `json:"session_generation"`
	JWTSecret            string `json:"jwt_secret"`
}

type bootstrapFile struct {
	Authorization string `json:"authorization,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
}

type authStateStore interface {
	GetE(string) (string, error)
	UpdateMap(func(map[string]string) error) error
	UpdateValue(string, func(string) (string, error)) error
}

type AuthManager struct{ store authStateStore }

type sessionClaims struct {
	Username   string `json:"username"`
	Generation uint64 `json:"generation"`
	jwt.RegisteredClaims
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate authentication state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func validateCredentials(username, password string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	return validatePassword(password)
}

func validateUsername(username string) error {
	if strings.TrimSpace(username) == "" || username != strings.TrimSpace(username) {
		return errors.New("username must be non-empty and have no surrounding whitespace")
	}
	if len(username) > 128 {
		return errors.New("username must contain at most 128 bytes")
	}
	return nil
}

func validatePassword(password string) error {
	// bcrypt rejects inputs over 72 bytes. Reject them explicitly instead of
	// silently truncating or turning a valid-looking form into a hash error.
	if len(password) < 12 || len(password) > 72 {
		return errors.New("password must contain between 12 and 72 bytes")
	}
	return nil
}

func validateBootstrapAuthorization(value string) error {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return errors.New("bootstrap authorization must be 32 random bytes encoded as unpadded base64url")
	}
	// Length and encoding alone accept obvious low-entropy inputs such as 32
	// repeated bytes. Require enough byte diversity to reject accidental or
	// hand-authored weak values while retaining a negligible false-rejection
	// probability for a token generated from crypto/rand.
	distinct := make(map[byte]struct{}, len(decoded))
	for _, b := range decoded {
		distinct[b] = struct{}{}
	}
	if len(distinct) < 16 {
		return errors.New("bootstrap authorization must be generated from a cryptographically secure random source")
	}
	return nil
}

func validateAuthState(state authState) error {
	if state.Version != 1 || state.JWTSecret == "" || state.SessionGeneration == 0 {
		return errors.New("unsupported authentication state")
	}
	if state.BootstrapComplete {
		// Legacy GateSentry accepted an empty administrator username. Preserve it
		// during migration so upgrading cannot silently lock out that owner. New
		// setup and credential changes still require a non-empty username.
		if state.PasswordHash == "" || state.BootstrapTokenDigest != "" {
			return errors.New("authentication state is inconsistent")
		}
	} else if state.Username != "" || state.PasswordHash != "" {
		return errors.New("authentication state is inconsistent")
	}
	return nil
}

func loadBootstrapFile(path string) (bootstrapFile, error) {
	var cfg bootstrapFile
	file, err := openBootstrapFile(path)
	if err != nil {
		return cfg, errors.New("cannot read bootstrap secret file")
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0077 != 0 {
		if closeErr := file.Close(); closeErr != nil {
			return cfg, errors.New("cannot close bootstrap secret file")
		}
		return cfg, errors.New("bootstrap secret file must be a regular file with mode 0600 or stricter")
	}
	b, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return cfg, errors.New("cannot read bootstrap secret file")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, errors.New("bootstrap secret file is malformed")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return cfg, errors.New("bootstrap secret file is malformed")
	}
	hasToken := cfg.Authorization != ""
	hasCredentials := cfg.Username != "" || cfg.Password != ""
	if hasToken == hasCredentials {
		return cfg, errors.New("bootstrap secret file must contain either authorization or username and password")
	}
	if hasCredentials {
		if err := validateCredentials(cfg.Username, cfg.Password); err != nil {
			return cfg, errors.New("bootstrap secret file contains invalid credentials")
		}
	} else if err := validateBootstrapAuthorization(cfg.Authorization); err != nil {
		return cfg, errors.New("bootstrap secret file contains invalid authorization")
	}
	return cfg, nil
}

func NewAuthManager(store *gatesentry2storage.MapStore, bootstrapPath string) (*AuthManager, error) {
	a := &AuthManager{store: store}
	rawState, err := store.GetE(authStateKey)
	if err != nil {
		return nil, err
	}
	if rawState != "" {
		var state authState
		if err := json.Unmarshal([]byte(rawState), &state); err != nil {
			return nil, fmt.Errorf("parse authentication state: %w", err)
		}
		if err := validateAuthState(state); err != nil {
			return nil, err
		}
	}
	var cfg bootstrapFile
	// A completed installation never reads bootstrap input again. This lets
	// operators remove the first-run secret and prevents stale mounted input
	// from affecting established credentials.
	stateComplete := rawState != ""
	if stateComplete {
		var state authState
		if err := json.Unmarshal([]byte(rawState), &state); err != nil {
			return nil, fmt.Errorf("parse authentication state: %w", err)
		}
		stateComplete = state.BootstrapComplete
	}
	if bootstrapPath != "" && !stateComplete {
		cfg, err = loadBootstrapFile(bootstrapPath)
		if err != nil {
			return nil, err
		}
	}
	var unattendedHash []byte
	if cfg.Username != "" {
		unattendedHash, err = passwordHash([]byte(cfg.Password))
		if err != nil {
			return nil, fmt.Errorf("hash unattended administrator password: %w", err)
		}
	}
	var newJWTSecret string
	if rawState == "" {
		newJWTSecret, err = randomSecret()
		if err != nil {
			return nil, err
		}
	}
	err = store.UpdateMap(func(values map[string]string) error {
		var state authState
		if raw := values[authStateKey]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &state); err != nil {
				return fmt.Errorf("parse authentication state: %w", err)
			}
			if err := validateAuthState(state); err != nil {
				return err
			}
		} else {
			state = authState{Version: 1, SessionGeneration: 1, JWTSecret: newJWTSecret}
		}
		var general gatesentryWebserverTypes.GSGeneral_Settings
		rawGeneral := values["general_settings"]
		legacyFields := make(map[string]json.RawMessage)
		if rawGeneral != "" {
			if err := json.Unmarshal([]byte(rawGeneral), &general); err != nil {
				return fmt.Errorf("parse general settings for authentication migration: %w", err)
			}
			if err := json.Unmarshal([]byte(rawGeneral), &legacyFields); err != nil {
				return fmt.Errorf("parse general settings for authentication migration: %w", err)
			}
		}
		_, hasLegacyUser := legacyFields["admin_username"]
		_, hasLegacyPassword := legacyFields["admin_password"]
		if state.BootstrapComplete {
			if hasLegacyUser || hasLegacyPassword {
				delete(legacyFields, "admin_username")
				delete(legacyFields, "admin_password")
				clean, err := json.Marshal(legacyFields)
				if err != nil {
					return err
				}
				values["general_settings"] = string(clean)
			}
			return nil
		}
		if hasLegacyUser || hasLegacyPassword {
			if !hasLegacyUser || !hasLegacyPassword {
				return errors.New("legacy administrator credentials are incomplete")
			}
			hash, err := passwordHash(passwordHashInput(general.AdminPassword))
			if err != nil {
				return fmt.Errorf("hash legacy administrator password: %w", err)
			}
			state.BootstrapComplete, state.Username, state.PasswordHash = true, general.AdminUser, string(hash)
			state.BootstrapTokenDigest = ""
			state.SessionGeneration++
			delete(legacyFields, "admin_username")
			delete(legacyFields, "admin_password")
			clean, err := json.Marshal(legacyFields)
			if err != nil {
				return err
			}
			values["general_settings"] = string(clean)
		}
		if !state.BootstrapComplete && cfg.Authorization != "" {
			wanted := digest(cfg.Authorization)
			if state.BootstrapTokenDigest != "" && subtle.ConstantTimeCompare([]byte(wanted), []byte(state.BootstrapTokenDigest)) != 1 {
				return errors.New("bootstrap authorization conflicts with persisted state")
			}
			state.BootstrapTokenDigest = wanted
		}
		if !state.BootstrapComplete && cfg.Username != "" {
			if state.BootstrapTokenDigest != "" {
				return errors.New("unattended bootstrap conflicts with persisted authorization state")
			}
			state.BootstrapComplete = true
			state.Username, state.PasswordHash = cfg.Username, string(unattendedHash)
			state.BootstrapTokenDigest = ""
			state.SessionGeneration++
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			return err
		}
		values[authStateKey] = string(encoded)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (a *AuthManager) read() (authState, error) {
	raw, err := a.store.GetE(authStateKey)
	if err != nil {
		return authState{}, err
	}
	var state authState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, fmt.Errorf("parse authentication state: %w", err)
	}
	if err := validateAuthState(state); err != nil {
		return state, err
	}
	return state, nil
}

func (a *AuthManager) Status() (bool, bool, error) {
	s, err := a.read()
	return s.BootstrapComplete, s.BootstrapTokenDigest != "", err
}

func (a *AuthManager) Bootstrap(username, password, authorization string, trustedLocal bool) error {
	if err := validateCredentials(username, password); err != nil {
		return err
	}
	// Reject unauthorized remote work before paying bcrypt's cost. The state
	// and authorization are checked again in the atomic update below.
	state, err := a.read()
	if err != nil {
		return err
	}
	if state.BootstrapComplete {
		return errSetupComplete
	}
	if state.BootstrapTokenDigest != "" {
		actual := digest(authorization)
		if subtle.ConstantTimeCompare([]byte(actual), []byte(state.BootstrapTokenDigest)) != 1 {
			return errSetupAuth
		}
	} else if !trustedLocal {
		return errSetupAuth
	}
	hash, err := passwordHash([]byte(password))
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
	}
	return a.store.UpdateValue(authStateKey, func(raw string) (string, error) {
		var state authState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			return "", fmt.Errorf("parse authentication state: %w", err)
		}
		if err := validateAuthState(state); err != nil {
			return "", err
		}
		if state.BootstrapComplete {
			return "", errSetupComplete
		}
		authorized := trustedLocal
		if state.BootstrapTokenDigest != "" {
			actual := digest(authorization)
			authorized = subtle.ConstantTimeCompare([]byte(actual), []byte(state.BootstrapTokenDigest)) == 1
		}
		if !authorized {
			return "", errSetupAuth
		}
		state.BootstrapComplete = true
		state.Username, state.PasswordHash = username, string(hash)
		state.BootstrapTokenDigest = ""
		state.SessionGeneration++
		return marshalAuthState(state)
	})
}

func marshalAuthState(state authState) (string, error) {
	b, err := json.Marshal(state)
	return string(b), err
}

func (a *AuthManager) Verify(username, password string) (bool, error) {
	state, err := a.read()
	if err != nil {
		return false, err
	}
	if !state.BootstrapComplete {
		return false, nil
	}
	// Always perform the expensive hash check for completed installations so
	// an unknown username does not create a cheap timing oracle.
	err = bcrypt.CompareHashAndPassword([]byte(state.PasswordHash), passwordHashInput(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verify administrator password: %w", err)
	}
	wantedUsername := sha256.Sum256([]byte(state.Username))
	actualUsername := sha256.Sum256([]byte(username))
	return subtle.ConstantTimeCompare(wantedUsername[:], actualUsername[:]) == 1, nil
}

func (a *AuthManager) CreateToken(username string) (string, error) {
	state, err := a.read()
	if err != nil {
		return "", err
	}
	if !state.BootstrapComplete || username != state.Username {
		return "", errInvalidLogin
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, sessionClaims{
		Username: username, Generation: state.SessionGeneration,
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))},
	})
	return token.SignedString([]byte(state.JWTSecret))
}

func (a *AuthManager) VerifyToken(tokenString string) (string, error) {
	state, err := a.read()
	if err != nil {
		return "", err
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(state.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return "", errInvalidLogin
	}
	if claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil ||
		claims.Username != state.Username || claims.Generation != state.SessionGeneration || !state.BootstrapComplete {
		return "", errInvalidLogin
	}
	return claims.Username, nil
}

func (a *AuthManager) ChangeCredentials(username, password string) error {
	if password == "" {
		return errors.New("a new password is required")
	}
	return a.store.UpdateValue(authStateKey, func(raw string) (string, error) {
		var state authState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			return "", err
		}
		if err := validateAuthState(state); err != nil {
			return "", err
		}
		if !state.BootstrapComplete {
			return "", errors.New("setup is incomplete")
		}
		if username == "" {
			username = state.Username
		}
		if err := validateCredentials(username, password); err != nil {
			return "", err
		}
		hash, err := passwordHash([]byte(password))
		if err != nil {
			return "", fmt.Errorf("hash administrator password: %w", err)
		}
		state.Username, state.PasswordHash = username, string(hash)
		state.SessionGeneration++
		return marshalAuthState(state)
	})
}

func (a *AuthManager) ChangeCredentialsAndGeneral(username, password string, general gatesentryWebserverTypes.GSGeneral_Settings) error {
	if username == "" && password == "" {
		return errors.New("a new username or password is required")
	}
	return a.store.UpdateMap(func(values map[string]string) error {
		var state authState
		if err := json.Unmarshal([]byte(values[authStateKey]), &state); err != nil {
			return err
		}
		if err := validateAuthState(state); err != nil {
			return err
		}
		if !state.BootstrapComplete {
			return errors.New("setup is incomplete")
		}
		if username == "" {
			username = state.Username
		}
		if err := validateUsername(username); err != nil {
			return err
		}
		if password != "" {
			if err := validatePassword(password); err != nil {
				return err
			}
			hash, err := passwordHash([]byte(password))
			if err != nil {
				return fmt.Errorf("hash administrator password: %w", err)
			}
			state.PasswordHash = string(hash)
		}
		state.Username = username
		state.SessionGeneration++
		authJSON, err := json.Marshal(state)
		if err != nil {
			return err
		}
		general.AdminUser, general.AdminPassword = "", ""
		generalJSON, err := json.Marshal(general)
		if err != nil {
			return err
		}
		values[authStateKey], values["general_settings"] = string(authJSON), string(generalJSON)
		return nil
	})
}

func requestIsLoopback(r *http.Request) bool {
	// The admin listener has no configured trusted-proxy list. A connection
	// arriving through any forwarding-aware proxy must therefore use the
	// one-time authorization, even when the immediate TCP peer is loopback.
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Real-IP"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return false
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
