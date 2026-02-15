package gatesentryf

import (
	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func RunFilter(filterType string, content string, responder *gatesentry2responder.GSFilterResponder) {
	for _, v := range R.Filters {
		v.Handle(content, filterType, responder)
	}
}
