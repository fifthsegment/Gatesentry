package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"

	gatesentryDiagnostics "bitbucket.org/abdullah_irfan/gatesentryf/diagnostics"
)

type DiagnosticsDeps struct {
	Diagnostics        gatesentryDiagnostics.Deps
	ApplicationVersion func() string
}

func GSApiDiagnosticsGET(w http.ResponseWriter, _ *http.Request, deps DiagnosticsDeps) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(gatesentryDiagnostics.Run(deps.Diagnostics))
}

func GSApiDiagnosticsBundleGET(w http.ResponseWriter, _ *http.Request, deps DiagnosticsDeps) {
	version := ""
	if deps.ApplicationVersion != nil {
		version = deps.ApplicationVersion()
	}
	data, err := gatesentryDiagnostics.CreateBundle(deps.Diagnostics, version)
	if err != nil {
		http.Error(w, `{"error":"Unable to create support bundle"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "attachment; filename=gatesentry-support-bundle.json")
	_, _ = w.Write(data)
}
