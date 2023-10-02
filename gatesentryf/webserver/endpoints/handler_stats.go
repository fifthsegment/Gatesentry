package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/kataras/iris/v12"
)

type URLGroup struct {
	URL   string `json:"url"`
	Count int    `json:"count"`
}

func ApiGetStats(ctx iris.Context, logger *gatesentryLogger.Log) {
	logEntries, err := logger.GetDNSLogs(1000)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to retrieve logs"})
		return
	}

	response := struct {
		Items []gatesentryLogger.LogEntry `json:"items"`
	}{
		Items: logEntries,
	}

	ctx.JSON(response)
}

func ApiGetStatsByURL(ctx iris.Context, logger *gatesentryLogger.Log) {
	logEntries, err := logger.GetDNSLogs(1000)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to retrieve logs"})
		return
	}

	// Group log entries by URL and count occurrences
	urlCounts := make(map[string]int)
	for _, entry := range logEntries {
		if entry.Type == "dns" {
			urlCounts[entry.URL]++
		}
	}

	// Convert grouped data into URLGroup slice
	groupedURLs := make([]URLGroup, 0, len(urlCounts))
	for url, count := range urlCounts {
		groupedURLs = append(groupedURLs, URLGroup{URL: url, Count: count})
	}

	response := struct {
		URLGroups []URLGroup `json:"items"`
	}{
		URLGroups: groupedURLs,
	}

	ctx.JSON(response)
}
