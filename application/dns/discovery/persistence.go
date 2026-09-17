package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DeviceAssignment is the durable subset of Device. Discovery owns observed
// identity at runtime (IPs, last-seen, sources, online state); this record
// owns only user-managed identity and the stable identifiers needed to merge
// re-discovered traffic into the same device.
type DeviceAssignment struct {
	ID         string    `json:"id"`
	DNSName    string    `json:"dns_name"`
	ManualName string    `json:"manual_name,omitempty"`
	Owner      string    `json:"owner,omitempty"`
	Category   string    `json:"category,omitempty"`
	Hostnames  []string  `json:"hostnames,omitempty"`
	MDNSNames  []string  `json:"mdns_names,omitempty"`
	MACs       []string  `json:"macs,omitempty"`
	FirstSeen  time.Time `json:"first_seen"`
	Persistent bool      `json:"persistent"`
}

// deviceAssignments is the persisted format. Version 1 is the first explicitly
// versioned device assignment format.
type deviceAssignments struct {
	Version     int                         `json:"version"`
	Assignments map[string]DeviceAssignment `json:"assignments"`
}

const (
	DeviceAssignmentsKey     = "assignments"
	deviceAssignmentsVersion = 1
)

// Persistence couples a DeviceStore to one durable destination for user-managed
// device identity. A nil Persistence disables durability, which keeps store
// construction and discovery tests independent of storage setup.
type Persistence struct {
	store assignmentStore
}

// assignmentStore is the minimal durable transaction interface. It is
// implemented by storage.MapStore operations so the real encrypted, atomic,
// error-aware path is exercised.
type assignmentStore interface {
	UpdateValue(key string, update func(string) (string, error)) error
	GetE(key string) (string, error)
}

// AttachPersistence gives the store a durable destination and restores the
// persisted assignment subset into memory. It fails closed on malformed,
// unsupported, or unreadable data so a startup cannot silently drop device
// assignments. Assignment records are not the source of truth for observed
// identity: merge fields are restored and discovery refreshes the rest.
func (ds *DeviceStore) AttachPersistence(store assignmentStore) error {
	if store == nil {
		return errors.New("attach device persistence: store is nil")
	}
	p := &Persistence{store: store}
	current, err := store.GetE(DeviceAssignmentsKey)
	if err != nil {
		return fmt.Errorf("attach device persistence: %w", err)
	}
	assignments, err := decodeDeviceAssignments(current)
	if err != nil {
		return fmt.Errorf("attach device persistence: %w", err)
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.persistence != nil {
		return errors.New("attach device persistence: already attached")
	}
	now := time.Now()
	for id, assignment := range assignments.Assignments {
		device := assignmentDevice(assignment)
		if existing := ds.devices[id]; existing != nil {
			device = mergePersistedDevice(existing, device)
		}
		if device.FirstSeen.IsZero() {
			device.FirstSeen = now
		}
		device.LastSeen = now
		device.Online = true
		if device.DNSName == "" {
			device.DNSName = ds.deriveDNSName(&device)
		}
		device.DisplayName = device.GetDisplayName()
		ds.devices[id] = &device
	}
	ds.persistence = p
	ds.rebuildIndexes()
	return nil
}

func (p *Persistence) persist(device Device) error {
	return p.store.UpdateValue(DeviceAssignmentsKey, func(current string) (string, error) {
		decoded, err := decodeDeviceAssignments(current)
		if err != nil {
			return "", err
		}
		decoded.Assignments[device.ID] = DeviceAssignment{
			ID:         device.ID,
			DNSName:    device.DNSName,
			ManualName: device.ManualName,
			Owner:      device.Owner,
			Category:   device.Category,
			Hostnames:  device.Hostnames,
			MDNSNames:  device.MDNSNames,
			MACs:       device.MACs,
			FirstSeen:  device.FirstSeen,
			Persistent: device.Persistent,
		}
		return encodeDeviceAssignments(decoded)
	})
}

func (p *Persistence) remove(id string) error {
	return p.store.UpdateValue(DeviceAssignmentsKey, func(current string) (string, error) {
		decoded, err := decodeDeviceAssignments(current)
		if err != nil {
			return "", err
		}
		if _, exists := decoded.Assignments[id]; !exists {
			return current, nil
		}
		delete(decoded.Assignments, id)
		return encodeDeviceAssignments(decoded)
	})
}

// persistIfChanged writes only when the durable subset differs from the stored
// record. Observed-only churn (IP, last-seen, online, sources) therefore does
// not create writes, which keeps discovery traffic from writing user data.
func (p *Persistence) persistIfChanged(device Device) error {
	current, err := p.store.GetE(DeviceAssignmentsKey)
	if err != nil {
		return fmt.Errorf("read device assignments: %w", err)
	}
	decoded, err := decodeDeviceAssignments(current)
	if err != nil {
		return fmt.Errorf("read device assignments: %w", err)
	}
	existing, ok := decoded.Assignments[device.ID]
	if ok && assignmentEqual(existing, assignmentRecord(device)) {
		return nil
	}
	return p.persist(device)
}

func assignmentRecord(device Device) DeviceAssignment {
	return DeviceAssignment{
		ID:         device.ID,
		DNSName:    device.DNSName,
		ManualName: device.ManualName,
		Owner:      device.Owner,
		Category:   device.Category,
		Hostnames:  device.Hostnames,
		MDNSNames:  device.MDNSNames,
		MACs:       device.MACs,
		FirstSeen:  device.FirstSeen,
		Persistent: device.Persistent,
	}
}

func assignmentEqual(a, b DeviceAssignment) bool {
	if a.ID != b.ID || a.DNSName != b.DNSName || a.ManualName != b.ManualName || a.Owner != b.Owner || a.Category != b.Category || a.Persistent != b.Persistent || !a.FirstSeen.Equal(b.FirstSeen) {
		return false
	}
	if len(a.Hostnames) != len(b.Hostnames) || len(a.MDNSNames) != len(b.MDNSNames) || len(a.MACs) != len(b.MACs) {
		return false
	}
	for i, value := range a.Hostnames {
		if b.Hostnames[i] != value {
			return false
		}
	}
	for i, value := range a.MDNSNames {
		if b.MDNSNames[i] != value {
			return false
		}
	}
	for i, value := range a.MACs {
		if b.MACs[i] != value {
			return false
		}
	}
	return true
}

func assignmentDevice(assignment DeviceAssignment) Device {
	return Device{
		ID:         assignment.ID,
		DNSName:    assignment.DNSName,
		ManualName: assignment.ManualName,
		Owner:      assignment.Owner,
		Category:   assignment.Category,
		Hostnames:  assignment.Hostnames,
		MDNSNames:  assignment.MDNSNames,
		MACs:       assignment.MACs,
		FirstSeen:  assignment.FirstSeen,
		Persistent: assignment.Persistent,
	}
}

func mergePersistedDevice(existing *Device, persisted Device) Device {
	merged := *existing
	if persisted.DNSName != "" {
		merged.DNSName = persisted.DNSName
	}
	merged.ManualName = persisted.ManualName
	merged.Owner = persisted.Owner
	merged.Category = persisted.Category
	merged.Hostnames = mergeStringSlice(existing.Hostnames, persisted.Hostnames)
	merged.MDNSNames = mergeStringSlice(existing.MDNSNames, persisted.MDNSNames)
	merged.MACs = mergeStringSlice(existing.MACs, persisted.MACs)
	if !persisted.FirstSeen.IsZero() {
		merged.FirstSeen = persisted.FirstSeen
	}
	merged.Persistent = existing.Persistent || persisted.Persistent
	merged.DisplayName = merged.GetDisplayName()
	return merged
}

func decodeDeviceAssignments(current string) (deviceAssignments, error) {
	decoded := deviceAssignments{Version: deviceAssignmentsVersion, Assignments: map[string]DeviceAssignment{}}
	if current == "" {
		return decoded, nil
	}
	if err := json.Unmarshal([]byte(current), &decoded); err != nil {
		return deviceAssignments{}, fmt.Errorf("parse device assignments: %w", err)
	}
	if decoded.Version != deviceAssignmentsVersion {
		return deviceAssignments{}, fmt.Errorf("device assignments version %d is not supported (expected %d); restore the matching GateSentry binary or the pre-upgrade data directory backup", decoded.Version, deviceAssignmentsVersion)
	}
	if decoded.Assignments == nil {
		return deviceAssignments{}, errors.New("device assignments payload must contain an assignments object")
	}
	for id, assignment := range decoded.Assignments {
		if id == "" {
			return deviceAssignments{}, errors.New("device assignment contains an empty id")
		}
		if assignment.ID != id {
			return deviceAssignments{}, fmt.Errorf("device assignment key %q does not match record id %q", id, assignment.ID)
		}
	}
	return decoded, nil
}

func encodeDeviceAssignments(assignments deviceAssignments) (string, error) {
	assignments.Version = deviceAssignmentsVersion
	if assignments.Assignments == nil {
		assignments.Assignments = map[string]DeviceAssignment{}
	}
	b, err := json.Marshal(assignments)
	if err != nil {
		return "", fmt.Errorf("serialize device assignments: %w", err)
	}
	return string(b), nil
}
