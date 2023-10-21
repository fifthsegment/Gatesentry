package gatesentry2filters

import gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"

type GSFilter struct {
	Id           string
	FilterName   string //This remains constant
	Description  string
	HasStrength  bool
	Handles      string //This is a unique identifier for each filter as well
	FileName     string
	Handler      func(*GSFilter, string, *gatesentry2responder.GSFilterResponder)
	FileContents []GsFilterLine
	Strictness   int
}

type GsFilterLine struct {
	Content string `json:"Content"`
	Score   int    `json:"Score"`
}

type GSAPIStructFilter struct {
	Id          string
	Name        string
	Handles     string
	Description string
	HasStrength bool
	Entries     []GsFilterLine
}
