package discovery

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// defaultDebounceInterval is the minimum time between successive disk writes.
	// Mutations that occur within this window are batched into a single write.
	defaultDebounceInterval = 5 * time.Second
)

// persistState holds the debounce timer and file path for device store persistence.
type persistState struct {
	mu       sync.Mutex
	filePath string
	timer    *time.Timer
	dirty    bool
	interval time.Duration
}

// SetPersistPath enables automatic persistence for this device store.
// Devices with a DNSName will be saved to the given file path whenever
// the store is mutated (debounced to avoid excessive disk I/O).
//
// If the file already exists, it is loaded immediately, populating the
// store with previously saved devices before any discovery sources run.
func (ds *DeviceStore) SetPersistPath(filePath string) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[Discovery] Warning: cannot create persist directory %s: %v", dir, err)
	}

	ds.persist = &persistState{
		filePath: filePath,
		interval: defaultDebounceInterval,
	}

	// Load existing data
	if err := ds.loadFromDisk(); err != nil {
		log.Printf("[Discovery] No saved devices loaded: %v", err)
	}
}

// persistedDevice is the JSON-serializable subset of Device that we save.
// We include all identity and network fields, but omit transient state
// like Online (which is recalculated from LastSeen on startup).
type persistedDevice struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name"`
	DNSName     string            `json:"dns_name"`
	Hostnames   []string          `json:"hostnames,omitempty"`
	MDNSNames   []string          `json:"mdns_names,omitempty"`
	MACs        []string          `json:"macs,omitempty"`
	IPv4        string            `json:"ipv4,omitempty"`
	IPv6        string            `json:"ipv6,omitempty"`
	Source      DiscoverySource   `json:"source"`
	Sources     []DiscoverySource `json:"sources,omitempty"`
	FirstSeen   time.Time         `json:"first_seen"`
	LastSeen    time.Time         `json:"last_seen"`
	ManualName  string            `json:"manual_name,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Category    string            `json:"category,omitempty"`
	Persistent  bool              `json:"persistent"`
}

// persistedStore is the top-level JSON structure written to disk.
type persistedStore struct {
	Version int               `json:"version"`
	Saved   time.Time         `json:"saved"`
	Devices []persistedDevice `json:"devices"`
}

// scheduleSave marks the store as dirty and schedules a debounced disk write.
// Called from mutating methods (UpsertDevice, UpdateDeviceIP, etc.) while
// the store's mu is still held — the actual I/O happens asynchronously.
func (ds *DeviceStore) scheduleSave() {
	if ds.persist == nil {
		return
	}
	ps := ds.persist
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.dirty = true
	if ps.timer != nil {
		ps.timer.Stop()
	}
	ps.timer = time.AfterFunc(ps.interval, func() {
		if err := ds.saveToDisk(); err != nil {
			log.Printf("[Discovery] Failed to persist devices: %v", err)
		}
	})
}

// saveToDisk serializes all devices with a DNSName to the persist file.
// Ephemeral IP-only passive entries are excluded to avoid clutter.
func (ds *DeviceStore) saveToDisk() error {
	if ds.persist == nil {
		return nil
	}
	ps := ds.persist
	ps.mu.Lock()
	ps.dirty = false
	ps.mu.Unlock()

	ds.mu.RLock()
	var devices []persistedDevice
	for _, d := range ds.devices {
		// Only persist devices that have a DNS name or are marked persistent.
		// Ephemeral passive entries (IP-only, no hostname) are rediscovered quickly.
		if d.DNSName == "" && !d.Persistent {
			continue
		}
		devices = append(devices, persistedDevice{
			ID:          d.ID,
			DisplayName: d.DisplayName,
			DNSName:     d.DNSName,
			Hostnames:   d.Hostnames,
			MDNSNames:   d.MDNSNames,
			MACs:        d.MACs,
			IPv4:        d.IPv4,
			IPv6:        d.IPv6,
			Source:      d.Source,
			Sources:     d.Sources,
			FirstSeen:   d.FirstSeen,
			LastSeen:    d.LastSeen,
			ManualName:  d.ManualName,
			Owner:       d.Owner,
			Category:    d.Category,
			Persistent:  d.Persistent,
		})
	}
	ds.mu.RUnlock()

	store := persistedStore{
		Version: 1,
		Saved:   time.Now(),
		Devices: devices,
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	// Write atomically: write to temp file, then rename
	tmpPath := ps.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, ps.filePath); err != nil {
		// Fallback: direct write if rename fails (cross-device)
		return os.WriteFile(ps.filePath, data, 0644)
	}

	log.Printf("[Discovery] Persisted %d devices to %s", len(devices), ps.filePath)
	return nil
}

// loadFromDisk reads previously persisted devices and populates the store.
// Existing in-memory devices are NOT overwritten — loaded devices are only
// added if their ID is not already present (discovery sources that ran
// before load take precedence).
func (ds *DeviceStore) loadFromDisk() error {
	if ds.persist == nil {
		return nil
	}

	data, err := os.ReadFile(ds.persist.filePath)
	if err != nil {
		return err
	}

	var store persistedStore
	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	loaded := 0
	for _, pd := range store.Devices {
		if pd.ID == "" || pd.DNSName == "" {
			continue
		}
		// Don't overwrite devices that were already discovered since boot
		if _, exists := ds.devices[pd.ID]; exists {
			continue
		}
		// Also skip if a device with the same DNSName was already discovered
		// (it may have a different generated ID but represent the same host).
		alreadyKnown := false
		for _, existing := range ds.devices {
			if existing.DNSName == pd.DNSName {
				alreadyKnown = true
				break
			}
		}
		if alreadyKnown {
			continue
		}

		d := &Device{
			ID:          pd.ID,
			DisplayName: pd.DisplayName,
			DNSName:     pd.DNSName,
			Hostnames:   pd.Hostnames,
			MDNSNames:   pd.MDNSNames,
			MACs:        pd.MACs,
			IPv4:        pd.IPv4,
			IPv6:        pd.IPv6,
			Source:      pd.Source,
			Sources:     pd.Sources,
			FirstSeen:   pd.FirstSeen,
			LastSeen:    pd.LastSeen,
			ManualName:  pd.ManualName,
			Owner:       pd.Owner,
			Category:    pd.Category,
			Persistent:  pd.Persistent,
			Online:      false, // will be updated by MarkOffline or next observation
		}
		ds.devices[d.ID] = d
		loaded++
	}

	if loaded > 0 {
		ds.rebuildIndexes()
		log.Printf("[Discovery] Loaded %d devices from %s (saved: %s)",
			loaded, ds.persist.filePath, store.Saved.Format(time.RFC3339))
	}

	return nil
}

// SaveNow forces an immediate synchronous save, bypassing the debounce timer.
// Used during graceful shutdown to ensure no data is lost.
func (ds *DeviceStore) SaveNow() error {
	if ds.persist == nil {
		return nil
	}
	// Cancel any pending debounce timer
	ps := ds.persist
	ps.mu.Lock()
	if ps.timer != nil {
		ps.timer.Stop()
		ps.timer = nil
	}
	ps.mu.Unlock()

	return ds.saveToDisk()
}
