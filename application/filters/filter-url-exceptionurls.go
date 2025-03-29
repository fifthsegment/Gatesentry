package gatesentry2filters

import (
	"context"
	"log"
	"strings"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterUrlExceptionUrls(ctx context.Context, f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	log.Println("Exception filter running for = " + content)
	for _, v := range f.FileContents {
		select {
		case <-ctx.Done():
			log.Println("FilterUrlExceptionUrls operation canceled or timed out")
			return
		default:
			// Continue processing
		}

		log.Println("Comparing ", content, " against ", v.Content)
		if strings.Contains(v.Content, content) || strings.Contains(content, v.Content) || strings.Contains(content+":443", v.Content) {
			responder.SetBlocked(true)
		}
	}
}
