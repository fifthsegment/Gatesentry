package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

var proxyUrl string

const GATESENTRY_CERTIFICATE_COMMON_NAME = "GateSentryFilter"
const BLOCKED_URLS_FILTER = "Blocked URLs"
const HTTPS_EXCEPTION_SITE = "https://www.github.com"
const HTTPS_BUMP_SITE = "https://www.google.com"
const HTTP_BLOCKED_SITE = "http://www.snapads.com"
const HTTPS_BLOCKED_SITE = "https://www.snapads.com"
const GATESENTRY_ADMIN_USERNAME = "admin"
const GATESENTRY_ADMIN_PASSWORD = "admin"

var GATESENTRY_WEBSERVER_BASE_ENDPOINT = "http://localhost:" + GSWEBADMINPORT + "/api"

func TestMain(m *testing.M) {
	// Start your proxy server here
	go main() // Assume startProxyServer starts your proxy
	proxyUrl = "http://localhost:" + GSPROXYPORT

	// Run tests
	code := m.Run()

	// Shutdown code if needed

	os.Exit(code)
}

func redirectLogs() {
	// set log to dev null
	f, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
	log.SetOutput(f)
}

func disableDNSBlacklistDownloads() {
	// Disable DNS blacklist downloads
	R.GSSettings.Update("dns_custom_entries", "[]")
	time.Sleep(1 * time.Second)
	R.Init()
	time.Sleep(1 * time.Second)
}

func TestProxyServer(t *testing.T) {

	fmt.Println("Starting tests...")
	time.Sleep(2 * time.Second)
	fmt.Println("Disabling DNS blacklist downloads")
	disableDNSBlacklistDownloads()

	t.Run("Test if the url block filter works", func(t *testing.T) {
		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			t.Fatal(err)
		}
		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 10 * time.Second,
		}

		url := ""
		for _, filter := range R.Filters {
			if filter.FilterName == BLOCKED_URLS_FILTER && len(filter.FileContents) > 0 {
				url = filter.FileContents[0].Content
			}
		}

		if url == "" {
			t.Fatal("No blocked URLs found")
		}

		fmt.Println("Checking if url = " + HTTP_BLOCKED_SITE + " is blocked")

		resp, err := httpClient.Get(HTTP_BLOCKED_SITE)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		time.Sleep(1 * time.Second)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodyStr := string(body)

		if !strings.Contains(bodyStr, "blocked URL") {
			t.Fatalf("Expected body to contain 'URL Blocked', but got %s", bodyStr)
		}

		fmt.Println("Checking if url = " + HTTPS_BLOCKED_SITE + " is blocked")

		resp, err = httpClient.Get(HTTPS_BLOCKED_SITE)
		if err != nil {
			fmt.Println("Error doing a GET for HTTPS blocked site")
			t.Fatal(err)

		}
		defer resp.Body.Close()
		time.Sleep(1 * time.Second)
		body, err = io.ReadAll(resp.Body)

		if err != nil {

			t.Fatal(err)
		}
		bodyStr = string(body)

		if !strings.Contains(bodyStr, "blocked URL") {
			t.Fatalf("Expected body to contain 'URL Blocked', but got %s", bodyStr)
		}
	})

	t.Run("Test if enabling https bumping actually bumps traffic", func(t *testing.T) {
		redirectLogs()
		enable_filtering := R.GSSettings.Get("enable_https_filtering")
		fmt.Println("Enable filtering = " + enable_filtering)
		R.GSSettings.Update("enable_https_filtering", "true")
		fmt.Println("Updated settings for https filtering")
		time.Sleep(1 * time.Second)
		enable_filtering = R.GSSettings.Get("enable_https_filtering")
		fmt.Println("Enable filtering = " + enable_filtering)
		R.Init()
		time.Sleep(1 * time.Second)

		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			t.Fatal(err)
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Don't check certificate
				},
			},
		}

		resp, err := httpClient.Get(HTTPS_BUMP_SITE)
		if err != nil {
			t.Fatalf("Traffic was not bumped. Got error: %s", err.Error())
		}
		defer resp.Body.Close()

		realCertSubject := "Some expected subject"
		proxyCertSubject := resp.TLS.PeerCertificates[0].Subject.CommonName

		isBumped := false
		for _, cert := range resp.TLS.PeerCertificates {
			if cert.Issuer.CommonName == GATESENTRY_CERTIFICATE_COMMON_NAME {
				isBumped = true
				break
			}
		}

		if !isBumped {
			t.Fatalf("Traffic was not bumped. Got cert subject: %s", proxyCertSubject)
		} else {
			t.Logf("Traffic was bumped. Expected %s but got %s", realCertSubject, proxyCertSubject)
		}
	})

	t.Run("Test if exception https site is not bumped", func(t *testing.T) {
		enable_filtering := R.GSSettings.Get("enable_https_filtering")
		fmt.Println("Enable filtering = " + enable_filtering)

		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			t.Fatal(err)
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Don't check certificate
				},
			},
		}

		resp, err := httpClient.Get(HTTPS_EXCEPTION_SITE)
		if err != nil {
			t.Fatalf("Got error: %s", err.Error())
		}
		defer resp.Body.Close()

		realCertSubject := "Some expected subject"
		proxyCertSubject := resp.TLS.PeerCertificates[0].Subject.CommonName

		isBumped := false
		for _, cert := range resp.TLS.PeerCertificates {
			if cert.Issuer.CommonName == GATESENTRY_CERTIFICATE_COMMON_NAME {
				isBumped = true
				break
			}
		}

		if isBumped {
			t.Fatalf("Traffic was not bumped. Got cert subject: %s", proxyCertSubject)
		} else {
			t.Logf("Traffic was bumped. Expected %s but got %s", realCertSubject, proxyCertSubject)
		}
	})

	t.Run("Test if disabling https bumping works", func(t *testing.T) {
		redirectLogs()
		R.GSSettings.Update("enable_https_filtering", "false")
		fmt.Println("Updated settings for https filtering")
		time.Sleep(1 * time.Second)
		enable_filtering := R.GSSettings.Get("enable_https_filtering")
		fmt.Println("Enable filtering = " + enable_filtering)
		R.Init()
		time.Sleep(1 * time.Second)

		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			t.Fatal(err)
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: false, // Don't check certificate
				},
			},
		}

		resp, err := httpClient.Get("https://www.google.com")
		if err != nil {
			// this is the actual test
			t.Fatal(err)
		}
		defer resp.Body.Close()
	})

	t.Run("Test if webserver login works with the default user", func(t *testing.T) {
		username := GATESENTRY_ADMIN_USERNAME
		password := GATESENTRY_ADMIN_PASSWORD

		payload := map[string]string{"username": username, "pass": password}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			t.Fatal("Failed to marshal JSON for sending:", err)
		}

		resp, err := http.Post(GATESENTRY_WEBSERVER_BASE_ENDPOINT+"/auth/token", "application/json", bytes.NewBuffer(jsonData))

		if err != nil {
			t.Fatal("Failed to get token:", err)
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal("Failed to read body:", err)
		}

		// Extract token from response
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal("Failed to unmarshal response:", err)
		}
		token, ok := result["Jwtoken"].(string)
		if !ok {
			t.Fatal("Token not found in response")
		}

		// Make GET request to /filters using the token
		req, err := http.NewRequest("GET", GATESENTRY_WEBSERVER_BASE_ENDPOINT+"/filters", nil)
		if err != nil {
			t.Fatal("Failed to create request:", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal("Failed to get filters:", err)
		}
		defer resp.Body.Close()

		// Check for 200 status code
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

	})

}
