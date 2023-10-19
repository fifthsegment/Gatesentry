package gatesentry2filters

import (
	"fmt"
	"strings"
)

func FilterYoutube() {

	url1 := "mainvideo url"
	// Extract video ID
	parts := strings.Split(url1, "/")
	videoID := parts[4]

	// Extract sqp parameter
	sqpIndex := strings.Index(url1, "sqp=")
	sqpEndIndex := strings.Index(url1[sqpIndex:], "|48")
	if sqpEndIndex == -1 {
		sqpEndIndex = len(url1) - sqpIndex
	} else {
		sqpEndIndex += sqpIndex
	}
	sqp := url1[sqpIndex:sqpEndIndex]

	// Extract sigh parameter
	sighIndex := strings.LastIndex(url1, "rs$")
	sigh := url1[sighIndex:]

	// Construct the new URL
	url2 := fmt.Sprintf("https://i.ytimg.com/sb/%s/storyboard3_L2/M2.jpg?%s&sigh=%s", videoID, sqp, sigh)
	fmt.Println(url2)
}
