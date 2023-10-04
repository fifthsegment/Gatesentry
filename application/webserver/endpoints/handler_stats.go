package gatesentryWebserverEndpoints

import (
	"sort"
	"strconv"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/kataras/iris/v12"
)

type URLGroup struct {
	URL   string `json:"host"`
	Count int    `json:"count"`
}

type HostGroupWithTotal struct {
	Total int        `json:"total"`
	Hosts []URLGroup `json:"hosts"`
}

type HostGroupSet map[string]HostGroupWithTotal

type HostGroupResponse struct {
	ItemsBlocked HostGroupSet `json:"blocked"`
	All          HostGroupSet `json:"all"`
}

func ApiGetStats(ctx iris.Context, logger *gatesentryLogger.Log) {
	fromTimeParam := ctx.URLParam("fromTime")

	// Parse the fromTimeParam to an integer
	fromTimeInt, err := strconv.Atoi(fromTimeParam)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid fromTime parameter"})
		return
	}

	logEntriesInterface, err := logger.GetLastXSecondsDNSLogs(int64(fromTimeInt), false)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to retrieve logs"})
		return
	}

	var logEntries []gatesentryLogger.LogEntry
	switch logs := logEntriesInterface.(type) {
	case []gatesentryLogger.LogEntry:
		logEntries = logs
	case map[string][]gatesentryLogger.LogEntry:
		for _, entries := range logs {
			logEntries = append(logEntries, entries...)
		}
	default:
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Invalid logs format"})
		return
	}

	response := struct {
		Items []gatesentryLogger.LogEntry `json:"items"`
	}{
		Items: logEntries,
	}

	ctx.JSON(response)
}

func SliceEntries(logs map[string][]gatesentryLogger.LogEntry, dnsResponseType string) map[string]HostGroupWithTotal {
	outputData := make(map[string]HostGroupWithTotal)
	for currentDate, entries := range logs {
		urlCounts := make(map[string]int)
		for _, entry := range entries {
			if entry.Type == "dns" && dnsResponseType == "all" {
				urlCounts[entry.URL]++
			} else if entry.Type == "dns" && entry.DNSResponseType == dnsResponseType {
				urlCounts[entry.URL]++
			}
		}
		groupedURLs := make([]URLGroup, 0, len(urlCounts))
		hostCount := 0
		for url, count := range urlCounts {
			groupedURLs = append(groupedURLs, URLGroup{URL: url, Count: count})
			// sort groupedUIRLs by count
			sort.Slice(groupedURLs, func(i, j int) bool {
				return groupedURLs[i].Count > groupedURLs[j].Count
			})
			hostCount += count
		}
		outputData[currentDate] = HostGroupWithTotal{
			Total: hostCount,
			Hosts: groupedURLs,
		}
	}
	return outputData
}

func ApiGetStatsByURL(ctx iris.Context, logger *gatesentryLogger.Log) {
	//DAY := 86400
	WEEK := 604800
	logEntriesInterface, err := logger.GetLastXSecondsDNSLogs(int64(WEEK), true)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to retrieve logs"})
		return
	}

	if logEntriesInterface == nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to retrieve logs"})
		return
	}

	var logEntries []gatesentryLogger.LogEntry
	switch logs := logEntriesInterface.(type) {
	case []gatesentryLogger.LogEntry:
		logEntries = logs
	case map[string][]gatesentryLogger.LogEntry:
		ctx.JSON(HostGroupResponse{
			ItemsBlocked: SliceEntries(logs, "blocked"),
			All:          SliceEntries(logs, "all"),
		})
		return
	default:
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Invalid logs format"})
		return
	}
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
