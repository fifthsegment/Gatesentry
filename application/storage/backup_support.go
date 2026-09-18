package gatesentry2storage

import (
	"encoding/json"
	"fmt"
)

// Snapshot returns a copy of every key under the shared path lock. It re-reads
// durable state so long-running callers see the latest committed transaction
// rather than a stale in-memory snapshot. It is the read path backup uses to
// export complete configuration in one consistent view.
func (m *MapStore) Snapshot() (map[string]string, error) {
	m.baseStore.pathMu.Lock()
	defer m.baseStore.pathMu.Unlock()
	if err := m.baseStore.loadLocked(); err != nil {
		return nil, err
	}
	if err := m.baseStore.Err(); err != nil {
		return nil, err
	}
	values, err := parseMap(m.baseStore.Get())
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out, nil
}

// ReplaceAll atomically replaces the complete map in one durable transaction.
// The previous content is retained until the full replacement commits, so a
// partial write cannot leave a half-applied configuration behind. It is the
// write path restore uses to publish a recovered configuration.
func (m *MapStore) ReplaceAll(values map[string]string) error {
	m.baseStore.pathMu.Lock()
	defer m.baseStore.pathMu.Unlock()
	if err := m.baseStore.loadLocked(); err != nil {
		return err
	}
	if err := m.baseStore.Err(); err != nil {
		return err
	}
	next := make(map[string]string, len(values))
	for key, value := range values {
		next[key] = value
	}
	data, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("serialize map storage: %w", err)
	}
	return m.baseStore.setLocked(data)
}

// InstallationKeyPath returns the location of the installation key file and
// whether it is operator-managed (set via GATESENTRY_INSTALLATION_KEY_FILE)
// rather than data-directory-managed. Backup includes the key so encrypted
// stores can be recovered after a total data-directory loss; restore does not
// overwrite a live key, because the restored logical content is re-encrypted
// with the current installation key.
func InstallationKeyPath() (path string, managed bool) {
	return installationKeyPath()
}
