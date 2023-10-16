package gatesentry2filters

import (
	"log"
	"strings"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterUrlExceptionUrls(f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	log.Println("Exception filter running for = " + content)
	for _, v := range f.FileContents {
		// log.Println("Comparing ", content , " against ", v.Content );
		if strings.Contains(v.Content, content) || strings.Contains(content, v.Content) || strings.Contains(content+":443", v.Content) {
			responder.Blocked = true
		}
	}
}
