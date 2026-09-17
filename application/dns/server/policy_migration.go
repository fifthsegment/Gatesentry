package gatesentryDnsServer

import (
	"fmt"
	"strings"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// migrateLegacyDevicePolicy converts legacy device owner/category metadata
// into explicit policy groups exactly once. Discovery owns the observations;
// the policy service owns enforcement, so the migration only reads the device
// store and writes policy records. Existing user-based rules are untouched
// and continue to apply in their existing precedence stage.
func migrateLegacyDevicePolicy(svc *gatesentryPolicy.Service, settings *gatesentry2storage.MapStore) error {
	if svc == nil {
		return nil
	}
	if deviceStore == nil {
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
	devices := deviceStore.GetAllDevices()
	legacy := make(map[string]gatesentryPolicy.LegacyDevice, len(devices))
	for _, device := range devices {
		legacy[device.ID] = gatesentryPolicy.LegacyDevice{
			ID:       device.ID,
			Owner:    strings.TrimSpace(device.Owner),
			Category: strings.TrimSpace(device.Category),
		}
	}
	groups, assignments, err := gatesentryPolicy.MigrateLegacyDevices(legacy)
	if err != nil {
		return fmt.Errorf("migrate legacy device metadata: %w", err)
	}
	if len(groups) == 0 && len(assignments) == 0 {
		return nil
	}
	if err := svc.SaveMigration(groups, assignments, "legacy_device_metadata"); err != nil {
		return fmt.Errorf("persist migrated policy document: %w", err)
	}
	if err := svc.Reload(); err != nil {
		return fmt.Errorf("reload migrated policy: %w", err)
	}
	return nil
}
