package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const numRuns = 3

func main() {
	// Set up the proxy
	proxyURL, err := url.Parse("http://guest:password@10.1.0.141:10413")
	if err != nil {
		panic(err)
	}
	httpTransport := &http.Transport{
		// add proxy credentials

		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: httpTransport,
	}

	// List of websites to visit
	websites := []string{"https://edition.cnn.com", "https://nrk.no", "https://www.reddit.com"}

	for _, website := range websites {
		var totalDuration time.Duration

		for i := 0; i < numRuns; i++ {
			start := time.Now()

			// Fetch the HTML
			resp, err := httpClient.Get(website)
			if err != nil {
				fmt.Println(err)
				continue
			}
			defer resp.Body.Close()

			// Parse the HTML
			doc, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				fmt.Println(err)
				continue
			}

			// Find and download assets
			doc.Find("img").Each(func(index int, element *goquery.Selection) {
				src, exists := element.Attr("src")
				if exists {
					_, err := httpClient.Get(src)
					if err != nil {
						fmt.Println(err)
					}
				}
			})

			elapsed := time.Since(start)
			totalDuration += elapsed
		}

		// Calculate and print the average time
		averageDuration := totalDuration / numRuns
		fmt.Printf("Average time taken to download assets from %s: %s\n", website, averageDuration)
	}
}
