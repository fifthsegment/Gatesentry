package gatesentry2filters

import (
	"log"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterUrlDontBump(f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	for _, v := range f.FileContents {
		// The url from the filter is in the form of url:443
		// For example for https://slack.com it is slack.com:443
		log.Println("Comparing ", content, " against internal = ", v.Content)
		if content == v.Content+":443" || content == v.Content {
			log.Println("URL found in list = " + v.Content)
			responder.Blocked = true
		}
	}
}
