package diagnostics

import (
	"encoding/json"
	"fmt"
	"time"
)

type Bundle struct {
	FormatVersion      int                      `json:"format_version"`
	CreatedAt          time.Time                `json:"created_at"`
	ApplicationVersion string                   `json:"application_version,omitempty"`
	Diagnostics        Report                   `json:"diagnostics"`
	Settings           map[string]string        `json:"settings"`
	Filters            []map[string]interface{} `json:"filters"`
}

// CreateBundle builds a previewable JSON document from an explicit safe
// allowlist. It deliberately excludes browsing logs, device identities,
// credentials, private keys, certificate contents, and image payloads.
func CreateBundle(deps Deps, applicationVersion string) ([]byte, error) {
	if deps.Settings == nil {
		return nil, fmt.Errorf("diagnostics: settings store is unavailable")
	}
	values, err := deps.Settings.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("diagnostics: snapshot settings: %w", err)
	}
	report := Run(deps)
	bundle := Bundle{FormatVersion: 1, CreatedAt: report.CheckedAt, ApplicationVersion: applicationVersion, Diagnostics: report, Settings: RedactSettings(values), Filters: FilterSummary(deps.Filters)}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("diagnostics: encode support bundle: %w", err)
	}
	return append(data, byte(10)), nil
}
