package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"strings"

	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// InspectionDeps bundles the live state the read-only HTTPS inspection status
// endpoint needs. None of the fields are mutated by the handler; the endpoint
// only reads settings, the no-bump filter list, and recorded decisions.
type InspectionDeps struct {
	Settings *gatesentry2storage.MapStore
	Filters  *[]gatesentryFilters.GSFilter
	Logger   *gatesentryLogger.Log
}

// noBumpHandles identifies the filter that owns inspection exclusions. It is
// the existing url/https_dontbump filter ("Exception Hosts"); the endpoint
// surfaces it without duplicating the list or introducing a second owner.
const noBumpHandles = "url/https_dontbump"

// inspectionCoverageCaveat is the honest framing the UI shows next to the
// counts so an operator does not equate "inspection enabled" with "every
// client inspected" or "coverage verified".
const inspectionCoverageCaveat = "These counts describe recorded decisions, not total traffic. Enabling inspection does not inspect every client, and unknown or unavailable state is shown explicitly."

// InspectionStatus is the read-only response for GET /api/certificate/inspection.
// It never carries keypem, credentials, browsing content, or stable device
// identifiers.
type InspectionStatus struct {
	Enabled          bool        `json:"enabled"`
	EnabledRaw       string      `json:"enabled_raw"`
	Certificate      *CertDetail `json:"certificate,omitempty"`
	CertificateError string      `json:"certificate_error,omitempty"`
	Exclusions       []string    `json:"exclusions"`
	ExclusionNote    string      `json:"exclusion_note"`
	InspectCount     int         `json:"inspect_count"`
	BypassCount      int         `json:"bypass_count"`
	Window           string      `json:"window"`
	From             int64       `json:"from"`
	To               int64       `json:"to"`
	CoverageCaveat   string      `json:"coverage_caveat"`
}

// GSApiInspectionGET returns the read-only HTTPS inspection status: whether
// MITM is enabled, the parsed CA certificate subject and expiry (never the
// private key), the configured no-bump exclusion list, and recorded
// inspect-versus-bypass decision counts with their query window. It is a
// read-only report: it never mutates configuration, rotates certificates, or
// writes to the decision log.
func GSApiInspectionGET(w http.ResponseWriter, r *http.Request, deps InspectionDeps) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	status := buildInspectionStatus(deps)
	json.NewEncoder(w).Encode(status)
}

// buildInspectionStatus assembles the read-only report. It is extracted so
// tests can exercise the logic without an HTTP round trip.
func buildInspectionStatus(deps InspectionDeps) InspectionStatus {
	status := InspectionStatus{
		ExclusionNote:  "Hosts listed here are tunnelled without HTTPS inspection. This is the existing Exception Hosts list (url/https_dontbump); it is not malware protection.",
		CoverageCaveat: inspectionCoverageCaveat,
	}
	if deps.Settings != nil {
		raw, err := deps.Settings.GetE("enable_https_filtering")
		if err == nil {
			status.EnabledRaw = raw
			status.Enabled = strings.EqualFold(raw, "true")
		}
	}
	if deps.Settings != nil {
		detail, err := ParseCertDetail(deps.Settings)
		if err != nil {
			status.CertificateError = err.Error()
		} else {
			status.Certificate = &detail
		}
	} else {
		status.CertificateError = "settings store is unavailable"
	}
	status.Exclusions = collectExclusions(deps.Filters)
	if deps.Logger != nil {
		summary, err := deps.Logger.DecisionSummary(gatesentryLogger.DecisionFilter{})
		if err == nil {
			status.Window = summary.Window
			status.From = summary.From
			status.To = summary.To
			status.InspectCount = summary.ByAction["inspect"]
			status.BypassCount = summary.ByAction["bypass"]
		}
	}
	return status
}

// collectExclusions reads the host list from the existing no-bump filter
// (url/https_dontbump, filter name "Exception Hosts"). It does not create a
// duplicate exclusion list; it reads the single source of truth.
func collectExclusions(filters *[]gatesentryFilters.GSFilter) []string {
	if filters == nil {
		return []string{}
	}
	for _, f := range *filters {
		if f.Handles == noBumpHandles {
			hosts := make([]string, 0, len(f.FileContents))
			for _, line := range f.FileContents {
				if line.Content != "" {
					hosts = append(hosts, line.Content)
				}
			}
			return hosts
		}
	}
	return []string{}
}
