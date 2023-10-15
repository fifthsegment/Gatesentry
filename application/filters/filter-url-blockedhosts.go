package gatesentry2filters

import (
	// "fmt"

	"strings"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterUrlBlockedHosts(f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {

	for _, v := range f.FileContents {
		if strings.Contains(content, v.Content) {
			responder.Blocked = true
		}
	}
}
