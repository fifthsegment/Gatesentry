package gatesentry2filters

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func FilterYoutube(ctx context.Context, url1 string) {
	select {
	case <-ctx.Done():
		log.Println("FilterYoutube operation canceled or timed out")
		return
	default:
		// Continue processing
	}

	// Extract video ID
	parts := strings.Split(url1, "/")
	if len(parts) < 5 {
		log.Println("Invalid URL format")
		return
	}
	videoID := parts[4]

	// Extract sqp parameter
	sqpIndex := strings.Index(url1, "sqp=")
	if sqpIndex == -1 {
		log.Println("sqp parameter not found in URL")
		return
	}
	sqpEndIndex := strings.Index(url1[sqpIndex:], "|48")
	if sqpEndIndex == -1 {
		sqpEndIndex = len(url1) - sqpIndex
	} else {
		sqpEndIndex += sqpIndex
	}
	sqp := url1[sqpIndex:sqpEndIndex]

	// Extract sigh parameter
	sighIndex := strings.LastIndex(url1, "rs$")
	if sighIndex == -1 {
		log.Println("sigh parameter not found in URL")
		return
	}
	sigh := url1[sighIndex:]

	// Construct the new URL
	url2 := fmt.Sprintf("https://i.ytimg.com/sb/%s/storyboard3_L2/M2.jpg?%s&sigh=%s", videoID, sqp, sigh)
	fmt.Println(url2)
}
