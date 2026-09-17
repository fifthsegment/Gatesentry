package gatesentry2storage

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	installationKeyFilename = "installation.key"
	managedKeyFileEnv       = "GATESENTRY_INSTALLATION_KEY_FILE"
	keyringFormat           = "gatesentry-keyring-v1"
	keySize                 = 32
	envelopeInspectionBytes = 32 * 1024
)

type keyringFile struct {
	Format   string   `json:"format"`
	Current  string   `json:"current"`
	Previous []string `json:"previous,omitempty"`
}

type installationKeys struct {
	current  []byte
	previous [][]byte
}

var installationKeyMu sync.Mutex

func installationKeyPath() (path string, managed bool) {
	if configured := os.Getenv(managedKeyFileEnv); configured != "" {
		return configured, true
	}
	return filepath.Join(GSBASEDIR, installationKeyFilename), false
}

func readInstallationKeys(path string) (installationKeys, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return installationKeys{}, err
	}
	if !info.Mode().IsRegular() {
		return installationKeys{}, errors.New("installation key file must be a regular file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return installationKeys{}, fmt.Errorf("installation key file permissions are %04o; group and other access must be disabled", info.Mode().Perm())
	}
	file, err := os.Open(path)
	if err != nil {
		return installationKeys{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil {
		_ = file.Close()
		return installationKeys{}, err
	}
	if err := file.Close(); err != nil {
		return installationKeys{}, fmt.Errorf("close installation key file: %w", err)
	}
	if len(data) > 64*1024 {
		return installationKeys{}, errors.New("installation key file is too large")
	}
	if len(data) == keySize {
		return installationKeys{current: append([]byte(nil), data...)}, nil
	}
	var disk keyringFile
	if err := json.Unmarshal(data, &disk); err != nil {
		return installationKeys{}, errors.New("installation key file must contain exactly 32 raw bytes or a valid GateSentry keyring")
	}
	if disk.Format != keyringFormat {
		return installationKeys{}, fmt.Errorf("unsupported installation key format %q", disk.Format)
	}
	current, err := decodeKey(disk.Current)
	if err != nil {
		return installationKeys{}, fmt.Errorf("invalid current installation key: %w", err)
	}
	keys := installationKeys{current: current}
	for i, encoded := range disk.Previous {
		key, err := decodeKey(encoded)
		if err != nil {
			return installationKeys{}, fmt.Errorf("invalid previous installation key %d: %w", i+1, err)
		}
		keys.previous = append(keys.previous, key)
	}
	return keys, nil
}

func decodeKey(encoded string) ([]byte, error) {
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid base64 key material")
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("key has %d bytes, want %d", len(key), keySize)
	}
	return key, nil
}

func randomKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate installation key: %w", err)
	}
	return key, nil
}

func loadInstallationKeys(allowGenerate bool) (installationKeys, error) {
	installationKeyMu.Lock()
	defer installationKeyMu.Unlock()
	return loadInstallationKeysLocked(allowGenerate)
}

func loadInstallationKeysLocked(allowGenerate bool) (installationKeys, error) {
	path, managed := installationKeyPath()
	keys, err := readInstallationKeys(path)
	if err == nil {
		return keys, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return installationKeys{}, fmt.Errorf("read installation key file %q: %w", path, err)
	}
	if managed {
		return installationKeys{}, fmt.Errorf("read managed installation key file %q: %w", path, err)
	}
	if !allowGenerate {
		return installationKeys{}, fmt.Errorf("installation key file %q is missing; restore it from backup", path)
	}
	protectedStores, err := authenticatedStoresWithoutKey(path)
	if err != nil {
		return installationKeys{}, err
	}
	if len(protectedStores) != 0 {
		// Another process may have published the installation key and then
		// written an authenticated store after our first key lookup. Re-read
		// before diagnosing recovery so concurrent first writes converge on
		// the winner instead of spuriously failing.
		keys, rereadErr := readInstallationKeys(path)
		if rereadErr == nil {
			return keys, nil
		}
		if !errors.Is(rereadErr, os.ErrNotExist) {
			return installationKeys{}, fmt.Errorf("read installation key file %q: %w", path, rereadErr)
		}
		return installationKeys{}, fmt.Errorf("installation key file %q is missing while authenticated storage %q exists; restore the key from backup", path, protectedStores[0])
	}
	key, err := randomKey()
	if err != nil {
		return installationKeys{}, err
	}
	created, committed, err := atomicCreate(path, key)
	if err != nil {
		return installationKeys{}, fmt.Errorf("create installation key file %q (committed=%t): %w", path, committed, err)
	}
	if !created {
		keys, err := readInstallationKeys(path)
		if err != nil {
			return installationKeys{}, fmt.Errorf("read concurrently created installation key file %q: %w", path, err)
		}
		return keys, nil
	}
	return installationKeys{current: key}, nil
}

// authenticatedStoresWithoutKey prevents a write to one store from creating
// a replacement key that would permanently orphan another authenticated
// store. Inspection is deliberately bounded: data directories also contain
// databases and logs that may be very large. GCM envelopes put their marker at
// the start of data, while the encrypted and version fields are in the tail.
func authenticatedStoresWithoutKey(keyPath string) ([]string, error) {
	dir := filepath.Clean(GSBASEDIR)
	cleanKeyPath, err := filepath.Abs(keyPath)
	if err != nil {
		cleanKeyPath = filepath.Clean(keyPath)
	}
	var stores []string
	err = filepath.WalkDir(dir, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if candidate == dir {
			return nil
		}
		// WalkDir never follows directory symlinks. Ignore only names reserved
		// for files created by atomicWrite and atomicCreate; dot-prefixed real
		// stores still participate in the missing-key check.
		if isAtomicTemporaryName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		absoluteCandidate, absErr := filepath.Abs(candidate)
		if absErr != nil {
			absoluteCandidate = filepath.Clean(candidate)
		}
		if absoluteCandidate == cleanKeyPath {
			return nil
		}
		keyDependent, readErr := isKeyDependentEnvelope(candidate)
		if readErr != nil {
			return fmt.Errorf("inspect data file %q before generating installation key: %w", candidate, readErr)
		}
		if keyDependent {
			relative, relErr := filepath.Rel(dir, candidate)
			if relErr != nil {
				return relErr
			}
			stores = append(stores, relative)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inspect data directory before generating installation key: %w", err)
	}
	return stores, nil
}

// isAtomicTemporaryName recognizes the exact numeric suffix produced by
// os.CreateTemp for this package's two temporary-file patterns. OpenStore and
// key rotation reject this namespace so a real store can never be hidden from
// missing-key inspection by looking like a temporary file.
func isAtomicTemporaryName(name string) bool {
	if !strings.HasPrefix(name, ".") {
		return false
	}
	for _, marker := range []string{".tmp-", ".create-"} {
		markerAt := strings.LastIndex(name, marker)
		if markerAt < 2 || markerAt+len(marker) == len(name) {
			continue
		}
		numeric := true
		for _, char := range name[markerAt+len(marker):] {
			if char < '0' || char > '9' {
				numeric = false
				break
			}
		}
		if numeric {
			return true
		}
	}
	return false
}

func isKeyDependentEnvelope(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}

	prefixSize := min(info.Size(), envelopeInspectionBytes)
	prefix := make([]byte, prefixSize)
	if _, err := io.ReadFull(file, prefix); err != nil {
		return false, err
	}
	trimmed := bytes.TrimSpace(prefix)
	// Non-JSON files such as log.db are rejected from consideration after one
	// small read. Storage envelopes are JSON objects containing a data field.
	if len(trimmed) == 0 || trimmed[0] != '{' || !containsJSONField(prefix, "data") {
		return false, nil
	}

	inspection := append([]byte(nil), prefix...)
	if info.Size() > prefixSize {
		tailSize := min(info.Size()-prefixSize, envelopeInspectionBytes)
		tail := make([]byte, tailSize)
		if _, err := file.ReadAt(tail, info.Size()-tailSize); err != nil {
			return false, err
		}
		inspection = append(inspection, tail...)
	}
	compact := removeJSONWhitespace(inspection)
	gcmMarked := bytes.Contains(compact, []byte(`"data":"`+gcmPayloadPrefix))
	versioned := bytes.Contains(compact, []byte(`"version":`))
	encrypted := bytes.Contains(compact, []byte(`"encrypted":"true"`))
	// A GCM marker is key-dependent even if corruption removed or changed the
	// version/encrypted fields. Any explicitly versioned encrypted envelope is
	// also key-dependent, including unknown future or corrupted versions.
	return gcmMarked || (encrypted && versioned), nil
}

func containsJSONField(data []byte, field string) bool {
	compact := removeJSONWhitespace(data)
	return bytes.Contains(compact, []byte(`"`+field+`":`))
}

func removeJSONWhitespace(data []byte) []byte {
	compact := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
		default:
			compact = append(compact, b)
		}
	}
	return compact
}

// atomicCreate publishes a fully written file only when destination does not
// already exist. The hard-link operation is the cross-process commit point:
// competing creators cannot replace the winner's installation key.
func atomicCreate(path string, data []byte) (created, committed bool, err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".create-*")
	if err != nil {
		return false, false, fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := tmp.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close temporary file: %w", closeErr))
			}
		}
		if removeErr := os.Remove(tmpName); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("clean temporary file: %w", removeErr))
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return false, false, fmt.Errorf("set temporary file permissions: %w", err)
	}
	written, err := tmp.Write(data)
	if err != nil {
		return false, false, fmt.Errorf("write temporary file: %w", err)
	}
	if written != len(data) {
		return false, false, io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		return false, false, fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return false, false, fmt.Errorf("close temporary file: %w", err)
	}
	closed = true
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("publish file: %w", err)
	}
	committed = true
	if err := os.Remove(tmpName); err != nil {
		return true, true, fmt.Errorf("remove published temporary name: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return true, true, fmt.Errorf("open directory for sync: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return true, true, fmt.Errorf("sync directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return true, true, fmt.Errorf("close directory: %w", err)
	}
	return true, true, nil
}

func encodeKeyring(keys installationKeys) ([]byte, error) {
	disk := keyringFile{
		Format:  keyringFormat,
		Current: base64.RawStdEncoding.EncodeToString(keys.current),
	}
	for _, key := range keys.previous {
		disk.Previous = append(disk.Previous, base64.RawStdEncoding.EncodeToString(key))
	}
	data, err := json.Marshal(disk)
	if err != nil {
		return nil, fmt.Errorf("serialize installation keyring: %w", err)
	}
	return data, nil
}

func writeInstallationKeys(keys installationKeys) error {
	installationKeyMu.Lock()
	defer installationKeyMu.Unlock()
	path, _ := installationKeyPath()
	data, err := encodeKeyring(keys)
	if err != nil {
		return err
	}
	committed, err := atomicWrite(path, data)
	if err != nil {
		return fmt.Errorf("write installation key file %q (committed=%t): %w", path, committed, err)
	}
	return nil
}

func allKeys(keys installationKeys) [][]byte {
	result := make([][]byte, 0, 1+len(keys.previous))
	result = append(result, keys.current)
	result = append(result, keys.previous...)
	return result
}
