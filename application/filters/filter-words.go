package gatesentry2filters

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterWords(f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	ReasonsForBlocking := []string{}
	pts := 0

	// Pre-process content to lowercase for case-insensitive matching
	lowerContent := strings.ToLower(content)

	// Iterate over all filter content
	for _, v := range f.FileContents {
		// Convert filter word to lowercase
		lowerVContent := strings.ToLower(v.Content)

		// Compile regex to match the word with word boundaries on both sides
		// `\b` ensures the word is not part of another word (i.e., space or string boundary)
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(lowerVContent) + `\b`)
		if err != nil {
			log.Printf("Invalid regex pattern: %v\n", err)
			continue
		}

		// Find all matches in the content
		matches := re.FindAllString(lowerContent, -1)

		// Count matches and update points
		found := len(matches)
		pts += found * v.Score

		// If the word is found, log the reason
		if found > 0 {
			reason := fmt.Sprintf("Found <u>%s</u> %d times, weightage of each time = %d", v.Content, found, v.Score)
			ReasonsForBlocking = append(ReasonsForBlocking, reason)
		}

		// If total score exceeds strictness, set responder and exit
		if pts > f.Strictness {
			responder.Score = pts
			responder.Blocked = true
			responder.Reasons = ReasonsForBlocking
			log.Println("Blocking content due to score threshold breach. Score:", pts)
			return // No need to process further once blocked
		}
	}

	// If the loop completes, set responder to not blocked
	log.Println("Final Score:", pts, "Strictness:", f.Strictness)
}
