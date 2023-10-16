package gatesentry2filters

import (
	"log"
	"os"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

var GSBASEDIR = "./"

func SetBaseDir(a string) {
	GSBASEDIR = a
}

func NewGSFilter(
	handles string,
	n string,
	Id string,
	filename string,
	hasStrength bool,
	description string,
	handlerFunc func(*GSFilter, string, *gatesentry2responder.GSFilterResponder),
) *GSFilter {
	log.Println("Registering new filter to handle = " + handles)
	filter := &GSFilter{
		Handles:     handles,
		FileName:    filename,
		FilterName:  n,
		HasStrength: hasStrength,
		Description: description,
	}
	filter.Handler = handlerFunc
	filter.Id = Id
	filter.LoadFilterFile()
	return filter
}

func (f *GSFilter) Handle(content string, contentType string, responder *gatesentry2responder.GSFilterResponder) {
	if contentType == f.Handles {
		f.Handler(f, content, responder)
	}
}

func LoadFilters(filters []GSFilter) []GSFilter {
	basepath := GSBASEDIR + "filterfiles/"
	e, _ := exists(basepath)
	if !e {
		os.Mkdir(basepath, 0777)
	}

	f := NewGSFilter(
		"url/all_exception_urls",
		"Exception URLs",
		"JHGJiwjkGOeglsd",
		basepath+"exceptionsitelist.json",
		false,
		"Exception sites that get wrongly blocked can be entered here. For sites mentioned here GateSentry simply doesn't touch their traffic.",
		FilterUrlExceptionUrls,
	)
	filters = append(filters, *f)

	f = NewGSFilter("url/https_dontbump", "Exception Hosts", "CeBqssmRbqXzbHR", basepath+"dontbump.json", false, "Add host names here that you want to allow on your network, regardless of whether they contain blocked content or not. Also add any hosts that completely break down on HTTPS filtering, so GateSentry won't touch traffic from those. This section is very helpful in allowing apps that can detect Man in the middle filtering by GateSentry. For example Snapchat, Facebook, Instagram etc. To allow Snapchat add app.snapchat.com here, to block it remove that entry from here. Similarly for Instagram, currently 2 entries are required: i.instagram.com and graph.instagram.com. ", FilterUrlDontBump)
	filters = append(filters, *f)

	f = NewGSFilter("url/all_blocked_mimes", "Blocked content types", "JHGJiwjkGOeglsk", basepath+"blockedmimes.json", false, "Add MIME type headers for blocked file types here. For example to block PNG/JPEG images on your network add 2 entries: image/png and image/jpeg here.", FilterBlockedMimes)
	filters = append(filters, *f)

	f = NewGSFilter("url/all_blocked_urls", "Blocked URLs", "bTXmTXgTuXpJuOZ", basepath+"blockedsites.json", false, "Sites to never allow on your network.", FilterUrlBlockedHosts)
	filters = append(filters, *f)

	f = NewGSFilter("text/html", "Keywords to Block", "bVxTPTOXiqGRbhF", basepath+"stopwords.json", true, "Whenever a blocked keyword is found on a webpage, it will be assigned a strength score based on each occurence. If the score exceeds the strictness threshold the page gets blocked.", FilterWords)
	filters = append(filters, *f)

	return filters
}
