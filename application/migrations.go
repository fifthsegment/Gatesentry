package gatesentryf

import (
	"errors"
	"fmt"
	"strconv"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
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
//
// The registry is empty at schema 1, which only declares that the settings
// content is versioned. The retired standalone rule store (the "rules" key) is
// deliberately not a migration here: its records are read once by the policy
// import, which accepts both shapes the key held, and the key is left
// byte-identical so the import is the single, auditable owner of that
// conversion. A store with no conversion to run is still stamped below.
var settingsMigrations = []settingsMigration{}

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
		current = migration.version
	}
	// A store whose content needed no conversion still records the schema it
	// now conforms to, so a later binary can tell which format it is reading.
	// Without this, a fresh install would look like a pre-schema store forever
	// and a future migration would run against content it already processed.
	if current < CurrentSettingsSchema {
		if err := store.Update(SettingsSchemaKey, strconv.Itoa(CurrentSettingsSchema)); err != nil {
			return fmt.Errorf("migrate settings: stamp schema %d: %w", CurrentSettingsSchema, err)
		}
	}
	return nil
}
