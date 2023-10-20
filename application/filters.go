package gatesentryf

import (
	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	"gopkg.in/elazarl/goproxy.v1"
	// "strings"
)

func RunFilter(filterType string, content string, responder *gatesentry2responder.GSFilterResponder) {
	for _, v := range R.Filters {
		v.Handle(content, filterType, responder)
	}
}

var ConditionalMitm goproxy.FuncHttpsHandler = func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	responder := &gatesentry2responder.GSFilterResponder{Blocked: false}

	RunFilter("url/https_dontbump", host, responder)
	if responder.Blocked {

		// A blocked here means the url is present in the list
		// The list in this case happens to be the exception site list
		// So a url on the list means to not MITM it.
		return goproxy.OkConnect, host
	}
	return goproxy.MitmConnect, host
}
