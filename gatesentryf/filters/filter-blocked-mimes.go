package gatesentry2filters

import (
	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	// "fmt"
	"strings"
	// "log"
)

func FilterBlockedMimes(f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {

	for _, v := range f.FileContents {
		// fmt.Println( v )
		// log.Println( "Running for = " + v.Content + " against = " + content )
		if strings.Contains(content, v.Content) {
			responder.Blocked = true
		}
	}
}
