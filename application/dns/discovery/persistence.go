package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// DeviceAssignment is the durable subset of Device. Discovery owns observed
// identity at runtime (IPs, last-seen, sources, online state); this record
// owns only user-managed identity and the stable identifiers needed to merge
// re-discovered traffic into the same device.
type DeviceAssignment struct {
	ID             string              `json:"id"`
	DNSName        string              `json:"dns_name"`
	ManualName     string              `json:"manual_name,omitempty"`
	Owner          string              `json:"owner,omitempty"`
	Category       string              `json:"category,omitempty"`
	Hostnames      []string            `json:"hostnames,omitempty"`
	MDNSNames      []string            `json:"mdns_names,omitempty"`
	MACs           []string            `json:"macs,omitempty"`
	FirstSeen      time.Time           `json:"first_seen"`
	Persistent     bool                `json:"persistent"`
	TailscaleNodes []TailscaleIdentity `json:"tailscale_nodes,omitempty"`
}

// deviceAssignments is the persisted format. Version 2 adds durable linked
// Tailscale node identities while keeping runtime addresses out of storage.
type deviceAssignments struct {
	Version     int                         `json:"version"`
	Assignments map[string]DeviceAssignment `json:"assignments"`
}

const (
	DeviceAssignmentsKey     = "assignments"
	deviceAssignmentsVersion = 2
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
	return ds.AttachPersistenceWithProtectedIDs(store, nil)
}

// AttachPersistenceWithProtectedIDs restores assignments while retaining even
// legacy ID-only records referenced by policy, pauses, or exceptions. This
// prevents an inventory cleanup migration from orphaning durable policy state.
func (ds *DeviceStore) AttachPersistenceWithProtectedIDs(store assignmentStore, protectedIDs map[string]bool) error {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	if store == nil {
		return errors.New("attach device persistence: store is nil")
	}
	ds.mu.RLock()
	alreadyAttached := ds.persistence != nil
	ds.mu.RUnlock()
	if alreadyAttached {
		return errors.New("attach device persistence: already attached")
	}

	p := &Persistence{store: store}
	original, err := store.GetE(DeviceAssignmentsKey)
	if err != nil {
		return fmt.Errorf("attach device persistence: %w", err)
	}
	decoded, sourceVersion, err := decodeDeviceAssignmentsVersion(original)
	if err != nil {
		return fmt.Errorf("attach device persistence: %w", err)
	}
	assignments := migrateDeviceAssignments(decoded, sourceVersion, protectedIDs)
	if sourceVersion != deviceAssignmentsVersion || !reflect.DeepEqual(decoded, assignments) {
		encoded, err := encodeDeviceAssignments(assignments)
		if err != nil {
			return fmt.Errorf("attach device persistence: %w", err)
		}
		if err := store.UpdateValue(DeviceAssignmentsKey, func(current string) (string, error) {
			if current != original {
				return "", errors.New("device assignments changed during migration")
			}
			return encoded, nil
		}); err != nil {
			return fmt.Errorf("attach device persistence: %w", err)
		}
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.persistence != nil {
		return errors.New("attach device persistence: already attached")
	}
	for id, assignment := range assignments.Assignments {
		device := assignmentDevice(assignment)
		if existing := ds.devices[id]; existing != nil {
			device = mergePersistedDevice(existing, device)
		}
		device.LastSeen = time.Time{}
		device.Online = false
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
			ID:             device.ID,
			DNSName:        device.DNSName,
			ManualName:     device.ManualName,
			Owner:          device.Owner,
			Category:       device.Category,
			Hostnames:      device.Hostnames,
			MDNSNames:      device.MDNSNames,
			MACs:           device.MACs,
			FirstSeen:      device.FirstSeen,
			Persistent:     device.Persistent,
			TailscaleNodes: durableTailscaleIdentities(device.TailscaleNodes),
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
	return p.persistDevicesIfChanged([]Device{device})
}

// persistDevicesIfChanged applies a set of per-device durable mutations in one
// storage transaction. This keeps RAM and disk from splitting if a snapshot
// refresh touches several linked devices and the write fails.
func (p *Persistence) persistDevicesIfChanged(devices []Device) error {
	return p.store.UpdateValue(DeviceAssignmentsKey, func(current string) (string, error) {
		decoded, err := decodeDeviceAssignments(current)
		if err != nil {
			return "", fmt.Errorf("read device assignments: %w", err)
		}
		changed := false
		for _, device := range devices {
			existing, exists := decoded.Assignments[device.ID]
			if !shouldPersistDevice(device) {
				if exists {
					delete(decoded.Assignments, device.ID)
					changed = true
				}
				continue
			}
			next := assignmentRecord(device)
			if exists && assignmentEqual(existing, next) {
				continue
			}
			decoded.Assignments[device.ID] = next
			changed = true
		}
		if !changed {
			return current, nil
		}
		return encodeDeviceAssignments(decoded)
	})
}

func assignmentRecord(device Device) DeviceAssignment {
	return DeviceAssignment{
		ID:             device.ID,
		DNSName:        device.DNSName,
		ManualName:     device.ManualName,
		Owner:          device.Owner,
		Category:       device.Category,
		Hostnames:      device.Hostnames,
		MDNSNames:      device.MDNSNames,
		MACs:           device.MACs,
		FirstSeen:      device.FirstSeen,
		Persistent:     device.Persistent,
		TailscaleNodes: durableTailscaleIdentities(device.TailscaleNodes),
	}
}

func assignmentEqual(a, b DeviceAssignment) bool {
	return reflect.DeepEqual(a, b)
}

func assignmentDevice(assignment DeviceAssignment) Device {
	return Device{
		ID:             assignment.ID,
		DNSName:        assignment.DNSName,
		ManualName:     assignment.ManualName,
		Owner:          assignment.Owner,
		Category:       assignment.Category,
		Hostnames:      assignment.Hostnames,
		MDNSNames:      assignment.MDNSNames,
		MACs:           assignment.MACs,
		FirstSeen:      assignment.FirstSeen,
		Persistent:     assignment.Persistent,
		TailscaleNodes: durableTailscaleIdentities(assignment.TailscaleNodes),
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
	merged.TailscaleNodes = mergeTailscaleIdentities(existing.TailscaleNodes, persisted.TailscaleNodes)
	if !persisted.FirstSeen.IsZero() {
		merged.FirstSeen = persisted.FirstSeen
	}
	merged.Persistent = existing.Persistent || persisted.Persistent
	merged.DisplayName = merged.GetDisplayName()
	return merged
}

func decodeDeviceAssignments(current string) (deviceAssignments, error) {
	decoded, _, err := decodeDeviceAssignmentsVersion(current)
	return decoded, err
}

func decodeDeviceAssignmentsVersion(current string) (deviceAssignments, int, error) {
	decoded := deviceAssignments{Version: deviceAssignmentsVersion, Assignments: map[string]DeviceAssignment{}}
	if current == "" {
		return decoded, deviceAssignmentsVersion, nil
	}
	if err := json.Unmarshal([]byte(current), &decoded); err != nil {
		return deviceAssignments{}, 0, fmt.Errorf("parse device assignments: %w", err)
	}
	if decoded.Version != 1 && decoded.Version != deviceAssignmentsVersion {
		return deviceAssignments{}, 0, fmt.Errorf("device assignments version %d is not supported (expected 1 or %d); restore the matching GateSentry binary or the pre-upgrade data directory backup", decoded.Version, deviceAssignmentsVersion)
	}
	if decoded.Assignments == nil {
		return deviceAssignments{}, 0, errors.New("device assignments payload must contain an assignments object")
	}
	for id, assignment := range decoded.Assignments {
		if id == "" {
			return deviceAssignments{}, 0, errors.New("device assignment contains an empty id")
		}
		if assignment.ID != id {
			return deviceAssignments{}, 0, fmt.Errorf("device assignment key %q does not match record id %q", id, assignment.ID)
		}
	}
	if decoded.Version == deviceAssignmentsVersion {
		nodeOwners := make(map[string]string)
		for deviceID, assignment := range decoded.Assignments {
			for _, identity := range assignment.TailscaleNodes {
				nodeID := normalizeNodeID(identity.NodeID)
				if nodeID == "" {
					continue
				}
				if owner, exists := nodeOwners[nodeID]; exists && owner != deviceID {
					return deviceAssignments{}, 0, fmt.Errorf("Tailscale node %q is assigned to multiple devices (%q and %q)", nodeID, owner, deviceID)
				}
				nodeOwners[nodeID] = deviceID
			}
		}
	}
	return decoded, decoded.Version, nil
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

func shouldPersistDevice(device Device) bool {
	return device.Persistent || strings.TrimSpace(device.ManualName) != "" ||
		strings.TrimSpace(device.Owner) != "" || strings.TrimSpace(device.Category) != "" ||
		len(device.TailscaleNodes) > 0 || device.Source == SourceManual || device.HasSource(SourceManual)
}

func durableTailscaleIdentities(identities []TailscaleIdentity) []TailscaleIdentity {
	result := make([]TailscaleIdentity, 0, len(identities))
	for _, identity := range identities {
		if nodeID := normalizeNodeID(identity.NodeID); nodeID != "" {
			result = append(result, TailscaleIdentity{NodeID: nodeID, Name: strings.TrimSpace(identity.Name), DNSName: strings.TrimSpace(identity.DNSName)})
		}
	}
	return result
}

func migrateDeviceAssignments(assignments deviceAssignments, sourceVersion int, protectedIDs map[string]bool) deviceAssignments {
	migrated := deviceAssignments{Version: deviceAssignmentsVersion, Assignments: make(map[string]DeviceAssignment)}
	for id, assignment := range assignments.Assignments {
		assignment.ID = id
		assignment.TailscaleNodes = durableTailscaleIdentities(assignment.TailscaleNodes)

		keep := true
		if sourceVersion == 1 {
			// v1 persisted every observation. DNSName, Hostnames, MDNSNames, MACs,
			// and FirstSeen can all be generated from passive discovery, so none
			// proves that a user chose to keep the record. Persistent and the
			// editable assignment labels are the only v1 evidence of durable intent.
			keep = assignment.Persistent || strings.TrimSpace(assignment.ManualName) != "" ||
				strings.TrimSpace(assignment.Owner) != "" || strings.TrimSpace(assignment.Category) != ""
		}
		if keep || protectedIDs[id] {
			migrated.Assignments[id] = assignment
		}
	}
	return migrated
}
