package gatesentryDnsServer

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func getFileSize(file *os.File) (int64, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}
	return fileInfo.Size(), nil
}

func LogQuery(domain string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	now := time.Now()
	normalizedDomain := strings.ToLower(domain)
	normalizedDomain = strings.TrimSuffix(normalizedDomain, ".")

	// Check file size and delete old file, create new file if needed
	fileSize, err := getFileSize(logsFile)
	if err != nil {
		log.Println("Error getting file size:", err)
		return
	}
	if fileSize >= 512*1024 { // 512 KB
		logsFile.Close()
		err := os.Remove(logsPath) // Delete old log file
		if err != nil {
			log.Println("Error deleting old logs file:", err)
			return
		}
		logsFile, err = os.OpenFile(logsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println("Error opening new logs file:", err)
			return
		}
	}

	queryLogs[normalizedDomain] = append(queryLogs[normalizedDomain], QueryLog{Domain: domain, Time: now})

	fileMutex.Lock()
	defer fileMutex.Unlock()

	logsFile.WriteString(fmt.Sprintf("%s|%s\n", now.Format(time.RFC3339), domain))
}

func PrintQueryLogsPeriodically() {
	for {
		time.Sleep(time.Hour) // Adjust the interval as needed

		logMutex.Lock()
		fmt.Println("Printing query logs...")
		// Process and display query logs
		currentTime := time.Now()
		for domain, logs := range queryLogs {
			var count int
			var logEntries []string
			for _, logEntry := range logs {
				if currentTime.Sub(logEntry.Time) <= time.Hour {
					count++
					logEntries = append(logEntries, logEntry.Time.Format(time.RFC3339))
				}
			}
			if count > 0 {
				fmt.Printf("Domain: %s, Count: %d, Last queried: %s\n", domain, count, strings.Join(logEntries, ", "))
			}
		}
		fmt.Println("====================================")

		// Clear logs older than a day
		queryLogs = ClearOldLogs(queryLogs, time.Hour*24)

		logMutex.Unlock()
	}
}

func ClearOldLogs(logs map[string][]QueryLog, maxAge time.Duration) map[string][]QueryLog {
	currentTime := time.Now()
	newLogs := make(map[string][]QueryLog)
	for domain, logEntries := range logs {
		var newLogEntries []QueryLog
		for _, logEntry := range logEntries {
			if currentTime.Sub(logEntry.Time) <= maxAge {
				newLogEntries = append(newLogEntries, logEntry)
			}
		}
		if len(newLogEntries) > 0 {
			newLogs[domain] = newLogEntries
		}
	}
	return newLogs
}

func InitializeLogs() {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	var err error
	logsFile, err = os.OpenFile(logsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening logs file:", err)
		os.Exit(1)
	}
}
