package gatesentry2storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
)

var rotationMu sync.Mutex

// RotateInstallationKey installs a new current key and re-encrypts each named
// encrypted store. Previous keys remain in installation.key, so interruption
// at any point leaves both already-rotated and not-yet-rotated stores readable.
// Callers must stop other GateSentry processes before invoking this operation.
func RotateInstallationKey(storeNames ...string) error {
	rotationMu.Lock()
	defer rotationMu.Unlock()
	if len(storeNames) == 0 {
		return errors.New("rotate installation key: at least one encrypted store name is required")
	}
	names, err := validateStoreNames(storeNames)
	if err != nil {
		return fmt.Errorf("rotate installation key: %w", err)
	}
	keys, err := loadInstallationKeys(false)
	if err != nil {
		return fmt.Errorf("rotate installation key: %w", err)
	}
	newKey, err := randomKey()
	if err != nil {
		return fmt.Errorf("rotate installation key: %w", err)
	}
	rotated := installationKeys{current: newKey}
	rotated.previous = appendUniqueKeys(rotated.previous, keys.current)
	for _, key := range keys.previous {
		rotated.previous = appendUniqueKeys(rotated.previous, key)
	}
	if err := writeInstallationKeys(rotated); err != nil {
		return fmt.Errorf("rotate installation key: %w", err)
	}
	for _, name := range names {
		if err := reencryptStore(name, rotated); err != nil {
			return fmt.Errorf("rotate installation key: %w; previous keys remain available for recovery", err)
		}
	}
	return nil
}

// PrunePreviousInstallationKeys removes retained keys only after every named
// store has been verified with the current key. It is deliberately separate
// from rotation so an operator can take and test a backup first.
func PrunePreviousInstallationKeys(storeNames ...string) error {
	rotationMu.Lock()
	defer rotationMu.Unlock()
	if len(storeNames) == 0 {
		return errors.New("prune installation keys: at least one encrypted store name is required")
	}
	keys, err := loadInstallationKeys(false)
	if err != nil {
		return fmt.Errorf("prune installation keys: %w", err)
	}
	currentOnly := installationKeys{current: keys.current}
	names, err := validateStoreNames(storeNames)
	if err != nil {
		return fmt.Errorf("prune installation keys: %w", err)
	}
	for _, name := range names {
		path, pathErr := storagePath(name)
		if pathErr != nil {
			return fmt.Errorf("prune installation keys: %w", pathErr)
		}
		pathMu := lockForPath(path)
		pathMu.Lock()
		data, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			pathMu.Unlock()
			continue
		}
		if readErr == nil {
			var parsed envelope
			parsed, readErr = parseEnvelope(data)
			if readErr == nil && (parsed.Encrypted != "true" || parsed.Version != gcmEnvelopeVersion) {
				readErr = errors.New("store is not in the authenticated encryption format")
			}
			if readErr == nil {
				_, _, readErr = decryptEnvelope(parsed, currentOnly, name)
			}
		}
		pathMu.Unlock()
		if readErr != nil {
			return fmt.Errorf("prune installation keys: verify storage %q with current key: %w", name, readErr)
		}
	}
	if err := writeInstallationKeys(currentOnly); err != nil {
		return fmt.Errorf("prune installation keys: %w", err)
	}
	return nil
}

func reencryptStore(name string, keys installationKeys) error {
	path, err := storagePath(name)
	if err != nil {
		return err
	}
	pathMu := lockForPath(path)
	pathMu.Lock()
	defer pathMu.Unlock()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read storage %q: %w", name, err)
	}
	parsed, err := parseEnvelope(data)
	if err != nil {
		return fmt.Errorf("load storage %q: %w", name, err)
	}
	if parsed.Encrypted != "true" {
		return fmt.Errorf("storage %q is not encrypted", name)
	}
	plaintext, _, err := decryptEnvelope(parsed, keys, name)
	if err != nil {
		return fmt.Errorf("load storage %q: %w", name, err)
	}
	encoded, err := encodeEnvelopeWithKey(plaintext, true, keys.current, name)
	if err != nil {
		return fmt.Errorf("encrypt storage %q: %w", name, err)
	}
	committed, err := atomicWrite(path, encoded)
	if err != nil {
		return fmt.Errorf("write storage %q (committed=%t): %w", name, committed, err)
	}
	return nil
}

func appendUniqueKeys(keys [][]byte, candidate []byte) [][]byte {
	for _, key := range keys {
		if bytes.Equal(key, candidate) {
			return keys
		}
	}
	return append(keys, append([]byte(nil), candidate...))
}

func validateStoreNames(names []string) ([]string, error) {
	seen := make(map[string]bool, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		if _, err := storagePath(name); err != nil {
			return nil, err
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result, nil
}
