package gatesentry2filters

import (
	"strings"
	// "fmt"
	"log"
	"strconv"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	// "gatesentry2/proxy"
)

func FilterWords(f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {

	ReasonsForBlocking := []string{}
	pts := 0
	// fmt.Println( pts );
	for _, v := range f.FileContents {
		// fmt.Println(  )
		found := strings.Count(strings.ToLower(content), strings.ToLower(v.Content))
		pts += found * v.Score
		// fmt.Println("Found " + v.Content + " times = " + strconv.Itoa( found ));
		if found > 0 {

			ReasonsForBlocking = append(ReasonsForBlocking, "Found <u>"+v.Content+"</u> "+strconv.Itoa(found)+" times, weightage of each time = "+strconv.Itoa(v.Score))
			// fmt.Println("Found " + v.Content + " " + strconv.Itoa(pts) + " times ");
		}

		// fmt.Println( "Total score = " + strconv.Itoa(pts) );
		if pts > f.Strictness {
			responder.Score = pts
			responder.Blocked = true
			responder.Reasons = ReasonsForBlocking
		}
	}

	log.Println("Score = " + strconv.Itoa(pts) + " strictness = " + strconv.Itoa(f.Strictness))

}

// func loadfilter(){
// 	gatesentry2.NewGSFilter("text/html", "filterfiles/stopwords.json")
// }
