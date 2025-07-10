package gatesentry2filters

import (
	"context"
	"log"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterUrlDontBump(ctx context.Context, f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	for _, v := range f.FileContents {
		select {
		case <-ctx.Done():
			log.Println("FilterUrlDontBump operation canceled or timed out")
			return
		default:
			// Continue processing
		}

		log.Println("Comparing ", content, " against internal = ", v.Content)
		if content == v.Content+":443" || content == v.Content {
			log.Println("URL found in list = " + v.Content)
			responder.Blocked = true
		}
	}
}
