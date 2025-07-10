package gatesentry2filters

import (
	"context"
	"strings"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterBlockedMimes(ctx context.Context, f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	for _, v := range f.FileContents {
		// Check if the context is canceled or timed out
		select {
		case <-ctx.Done():
			// Exit early if the context is canceled
			return
		default:
			// Continue processing
		}

		// Check if the content contains the blocked MIME type
		if strings.Contains(content, v.Content) {
			responder.Blocked = true
			return // Exit early since the content is blocked
		}
	}
}
