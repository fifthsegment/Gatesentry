package gatesentryf

import gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"

// "strings"
// "strconv"
// "fmt";

func runfilterHandlers(content string, contentType string, responder *gatesentry2responder.GSFilterResponder) {
	for _, v := range R.Filters {
		v.Handle(content, contentType, responder)
	}
}

func Handle_Html_Response(s string) string {
	// fmt.Println("Received response");
	responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
	runfilterHandlers(s, "text/html", responder)
	if responder.Blocked {
		return gatesentry2responder.BuildResponsePage(responder.Reasons, responder.Score)
	}
	// fmt.Println( s );
	// fmt.Println( len(R.FileContents["stopwords"]) )

	return s
}
