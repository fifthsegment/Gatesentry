package gatesentryDnsServer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
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

// legacyBlockListFile is the retired gateway-wide Block List filter.
const legacyBlockListFile = "filterfiles/blockedsites.json"

// legacyBlockListStock is the entry every installation shipped with. A list
// holding only it was never edited, so it is retired without being copied into
// every policy.
const legacyBlockListStock = "snapads.com"

// migrateLegacyBlockList moves the retired Block List into policies. The list
// applied to every proxied request, so each existing policy, the default
// included, gains its entries as blocked domains; DNS then enforces them too.
// The file is emptied afterwards, which makes the migration safe to run on
// every start.
func migrateLegacyBlockList(svc *gatesentryPolicy.Service, basePath string) error {
	if svc == nil {
		return nil
	}
	path := basePath + legacyBlockListFile
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy block list: %w", err)
	}
	var file struct {
		Keywords []struct {
			Content string `json:"Content"`
		} `json:"keywords"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("decode legacy block list: %w", err)
	}
	var domains []string
	for _, entry := range file.Keywords {
		pattern, err := gatesentryPolicy.NormalizeDomainPattern(entry.Content)
		if err != nil {
			log.Printf("[DNS] Legacy block list entry %q is not a domain; not migrated", entry.Content)
			continue
		}
		if pattern != "" {
			domains = append(domains, pattern)
		}
	}
	if len(domains) == 1 && domains[0] == legacyBlockListStock {
		domains = nil
	}
	if len(file.Keywords) == 0 {
		return nil
	}
	if len(domains) > 0 {
		for _, group := range svc.OrderedGroups() {
			group.BlockedDomains = append(group.BlockedDomains, domains...)
			if err := svc.UpdateGroup(group.ID, group); err != nil {
				return fmt.Errorf("add legacy block list to policy %q: %w", group.ID, err)
			}
		}
		if err := svc.Reload(); err != nil {
			return fmt.Errorf("reload policy after block list migration: %w", err)
		}
		log.Printf("[DNS] Moved %d legacy block list entries into every policy's blocked domains", len(domains))
	}
	if err := os.WriteFile(path, []byte(`{"keywords":[]}`), 0o600); err != nil {
		return fmt.Errorf("empty legacy block list: %w", err)
	}
	return nil
}
