package gatesentryDnsServer

import (
	"fmt"
	"log"
	"strings"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// migrateLegacyPolicy converts pre-policy configuration into explicit policy
// groups exactly once: legacy device owner/category metadata becomes group
// assignments, and records from the retired standalone rule store become
// unassigned groups whose rules reproduce what those rules enforced. Discovery
// owns the observations and storage owns durability, so the migration only
// reads the device and settings stores and writes policy records.
func migrateLegacyPolicy(svc *gatesentryPolicy.Service, settings *gatesentry2storage.MapStore) error {
	if svc == nil {
		return nil
	}
	if settings == nil {
		return nil
	}
	// Migration runs exactly once: only when no policy document exists yet.
	// A present key means either migration already ran or a document was
	// created through the API; re-running here would clobber those edits.
	raw, err := settings.GetE(gatesentryPolicy.StorageKey)
	if err != nil {
		return fmt.Errorf("read policy groups: %w", err)
	}
	if strings.TrimSpace(raw) != "" {
		return nil
	}
	var groups []gatesentryPolicy.PolicyGroup
	var assignments []gatesentryPolicy.DeviceAssignment
	if deviceStore != nil {
		devices := deviceStore.GetAllDevices()
		legacy := make(map[string]gatesentryPolicy.LegacyDevice, len(devices))
		for _, device := range devices {
			legacy[device.ID] = gatesentryPolicy.LegacyDevice{
				ID:       device.ID,
				Owner:    strings.TrimSpace(device.Owner),
				Category: strings.TrimSpace(device.Category),
			}
		}
		migrated, migratedAssignments, err := gatesentryPolicy.MigrateLegacyDevices(legacy)
		if err != nil {
			return fmt.Errorf("migrate legacy device metadata: %w", err)
		}
		groups = append(groups, migrated...)
		assignments = append(assignments, migratedAssignments...)
	}
	legacyRules, err := settings.GetE("rules")
	if err != nil {
		return fmt.Errorf("read retired rule store: %w", err)
	}
	decoded, err := gatesentryPolicy.DecodeLegacyRules(legacyRules)
	if err != nil {
		return fmt.Errorf("decode retired rule store: %w", err)
	}
	if len(decoded) > 0 {
		timezone, err := settings.GetE("timezone")
		if err != nil {
			return fmt.Errorf("read timezone: %w", err)
		}
		imported, err := gatesentryPolicy.MigrateLegacyRules(decoded, timezone)
		if err != nil {
			return fmt.Errorf("migrate retired rules: %w", err)
		}
		groups = append(groups, imported...)
		log.Printf("[DNS] Imported %d rule(s) from the retired rule store as unassigned policy groups; assign devices before they change filtering", len(imported))
	}
	if len(groups) == 0 && len(assignments) == 0 {
		return nil
	}
	if err := svc.SaveMigration(groups, assignments, "legacy_device_metadata_and_rules"); err != nil {
		return fmt.Errorf("persist migrated policy document: %w", err)
	}
	if err := svc.Reload(); err != nil {
		return fmt.Errorf("reload migrated policy: %w", err)
	}
	return nil
}
