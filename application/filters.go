package gatesentryf

import (
	"log"
	"net/http"

	gatesentry2filters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2proxy "bitbucket.org/abdullah_irfan/gatesentryf/proxy"
	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	"github.com/elazarl/goproxy"
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

var FilterHosts goproxy.FuncReqHandler = func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {

	// fmt.Println(url)
	// if ( strings.Contains(url, "127.0.0.1") ){
	// 	return r,nil
	// }
	log.Println("Running filterhosts")
	responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
	blockedtimes := R.GSSettings.Get("blocktimes")
	gatesentry2filters.RunTimeFilter(responder, blockedtimes, "Asia/Karachi")
	if responder.Blocked {
		return r, goproxy.NewResponse(r, goproxy.ContentTypeHtml, http.StatusForbidden, gatesentry2responder.BuildResponsePage([]string{"No-Internet time"}, -1))
	}

	responder = &gatesentry2responder.GSFilterResponder{Blocked: false}
	RunFilter("url/all_exception_urls", r.URL.String(), responder)
	if responder.Blocked {
		gatesentry2proxy.SetSessionData(ctx, "DONT_TOUCH", true)
		// A blocked here means the url is present in the list
		// The list in this case happens to be the exception site list
		// So a url on the list means to not to block it.
		return r, nil
	}

	responder = &gatesentry2responder.GSFilterResponder{Blocked: false}
	RunFilter("url/all_blocked_urls", r.URL.String(), responder)
	if responder.Blocked {
		return r, goproxy.NewResponse(r, goproxy.ContentTypeHtml, http.StatusForbidden, gatesentry2responder.BuildResponsePage([]string{"URL Blocked"}, -1))
	}
	return r, nil
}
