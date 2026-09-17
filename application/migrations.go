package gatesentryf

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

// SettingsSchemaKey names the persisted settings content schema version. It is
// distinct from the application version in the "version" key, which only
// records which binary last wrote the store.
const SettingsSchemaKey = "settings_schema_version"

// CurrentSettingsSchema is the schema written by this binary. Version 1 is the
// first explicitly versioned settings content format. Pre-schema stores are
// migrated by settingsMigration1.
const CurrentSettingsSchema = 1

// settingsMigration describes one ordered content migration for GSSettings.
// Ownership is explicit: runtime calls this registry before default seeding,
// and storage remains owner of envelope/encryption formats.
type settingsMigration struct {
	version int
	apply   func(values map[string]string) (map[string]string, error)
}

// settingsMigrations is ordered by version. Entry i converts the store from
// version i-1 to version i (entry.version is the version it produces).
var settingsMigrations = []settingsMigration{
	{
		version: 1,
		apply: func(values map[string]string) (map[string]string, error) {
			// Migration 1 normalizes the legacy bare-array rules value into
			// the RuleList format later code writes. Missing optional
			// settings stay missing here; runtime default seeding fills them.
			rulesJSON, ok := values["rules"]
			if !ok || rulesJSON == "" {
				return values, nil
			}
			var ruleList GatesentryTypes.RuleList
			if err := json.Unmarshal([]byte(rulesJSON), &ruleList); err == nil {
				encoded, err := json.Marshal(ruleList)
				if err != nil {
					return nil, fmt.Errorf("normalize rules list: %w", err)
				}
				if encoded := string(encoded); encoded != rulesJSON {
					values["rules"] = encoded
				}
				return values, nil
			}
			var rules []GatesentryTypes.Rule
			if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
				// Malformed rules fail migration instead of being silently
				// replaced: configuration corruption must be visible.
				return nil, fmt.Errorf("migrate legacy rules value: %w", err)
			}
			if rules == nil {
				rules = []GatesentryTypes.Rule{}
			}
			encoded, err := json.Marshal(GatesentryTypes.RuleList{Rules: rules})
			if err != nil {
				return nil, fmt.Errorf("encode migrated rules list: %w", err)
			}
			values["rules"] = string(encoded)
			return values, nil
		},
	},
}

// MigrateSettings applies ordered GSSettings content migrations and stamps the
// resulting schema version. A store from a newer GateSentry fails closed so an
// older binary cannot overwrite a schema it does not understand. Each
// migration is one atomic MapStore transaction; pre-write errors leave the
// previous file unchanged.
func MigrateSettings(store *gatesentry2storage.MapStore) error {
	if store == nil {
		return errors.New("migrate settings: store is nil")
	}
	if err := store.Err(); err != nil {
		return fmt.Errorf("migrate settings: %w", err)
	}
	rawVersion, err := store.GetE(SettingsSchemaKey)
	if err != nil {
		return fmt.Errorf("migrate settings: read schema version: %w", err)
	}
	var current int
	if rawVersion != "" {
		current, err = strconv.Atoi(rawVersion)
		if err != nil {
			return fmt.Errorf("migrate settings: parse schema version %q: %w", rawVersion, err)
		}
		if current < 0 {
			return fmt.Errorf("migrate settings: schema version %d is negative", current)
		}
		if current > CurrentSettingsSchema {
			return fmt.Errorf("migrate settings: settings schema %d is newer than this release supports (%d); restore the matching newer GateSentry binary or the pre-upgrade data directory backup, then retry", current, CurrentSettingsSchema)
		}
	}
	for _, migration := range settingsMigrations {
		if current >= migration.version {
			continue
		}
		if err := store.UpdateMap(func(values map[string]string) error {
			next, err := migration.apply(values)
			if err != nil {
				return err
			}
			next[SettingsSchemaKey] = strconv.Itoa(migration.version)
			for key, value := range next {
				values[key] = value
			}
			return nil
		}); err != nil {
			return fmt.Errorf("migrate settings to schema %d: %w", migration.version, err)
		}
	}
	return nil
}
