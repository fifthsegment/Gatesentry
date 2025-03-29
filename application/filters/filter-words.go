package gatesentry2filters

import (
	"context"
	"fmt"
	"log"
	"regexp"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

func FilterWords(ctx context.Context, f *GSFilter, content string, responder *gatesentry2responder.GSFilterResponder) {
	ReasonsForBlocking := []string{}
	pts := 0

	// Create a channel to signal completion
	done := make(chan struct{})
	defer close(done)

	// Use a goroutine to handle the filtering logic
	go func() {
		// Iterate over all filter content
		for _, v := range f.FileContents {
			// Check if the context is canceled or expired
			select {
			case <-ctx.Done():
				log.Println("FilterWords operation canceled or timed out")
				return
			default:
				// Continue processing
			}

			// Compile regex to match the word with word boundaries on both sides
			re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(v.Content) + `\b`)
			if err != nil {
				log.Printf("Invalid regex pattern: %v\n", err)
				continue
			}

			// Find all matches in the content
			matches := re.FindAllString(content, -1)

			// Count matches and update points
			found := len(matches)
			pts += found * v.Score

			// If the word is found, log the reason
			if found > 0 {
				reason := fmt.Sprintf("Found <u>%s</u> %d times, weightage of each time = %d <!-- %s --->", "blocked word", found, v.Score, v.Content)
				ReasonsForBlocking = append(ReasonsForBlocking, reason)
			}

			// If total score exceeds strictness, set responder and exit
			if pts > f.Strictness {
				responder.Score = pts
				responder.Blocked = true
				responder.Reasons = ReasonsForBlocking
				log.Println("Blocking content due to score threshold breach. Score:", pts)
				done <- struct{}{} // Signal completion
				return
			}
		}

		// If the loop completes, set responder to not blocked
		log.Println("Final Score:", pts, "Strictness:", f.Strictness)
		done <- struct{}{} // Signal completion
	}()

	// Wait for completion or context cancellation
	select {
	case <-done:
		// Completed successfully
	case <-ctx.Done():
		// Context canceled or timed out
		log.Println("FilterWords operation canceled or timed out")
	}
}
