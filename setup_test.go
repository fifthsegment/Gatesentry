package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

var (
	proxyURL                        string
	gatesentryWebserverBaseEndpoint string
)

const (
	gatesentryCertificateCommonName = "GateSentryFilter"
	blockedURLsFilter               = "Blocked URLs"
	httpsExceptionSite              = "https://www.github.com"
	httpsBumpSite                   = "https://www.google.com"
	httpBlockedSite                 = "http://www.snapads.com"
	httpsBlockedSite                = "https://www.snapads.com"
	gatesentryAdminUsername         = "admin"
	gatesentryAdminPassword         = "admin"
	testUserUsername                = "testuser123"
	testUserPassword                = "testpassword123"
	defaultTimeout                  = 30 * time.Second
	proxyReadyWaitTime              = 2 * time.Second
)

func TestMain(m *testing.M) {
	// Start proxy server in background
	go main()
	
	// Initialize test variables
	proxyURL = "http://localhost:" + GSPROXYPORT
	gatesentryWebserverBaseEndpoint = "http://localhost:" + GSWEBADMINPORT + "/api"

	// Wait for server to start
	time.Sleep(10 * time.Second)

	// Run tests
	code := m.Run()

	// Cleanup would go here if needed
	os.Exit(code)
}

func redirectLogs(tb testing.TB) {
	tb.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
	if err != nil {
		tb.Fatalf("Failed to open devnull: %v", err)
	}
	log.SetOutput(f)
	tb.Cleanup(func() {
		log.SetOutput(os.Stderr)
	})
}

func disableDNSBlacklistDownloads(tb testing.TB) {
	tb.Helper()
	R.GSSettings.Update("dns_custom_entries", "[]")
	time.Sleep(1 * time.Second)
	R.Init()
	time.Sleep(1 * time.Second)
}

func waitForProxyReady(tb testing.TB, proxyURLStr string, maxAttempts int) error {
	tb.Helper()
	parsedURL, err := url.Parse(proxyURLStr)
	if err != nil {
		return fmt.Errorf("failed to parse proxy URL: %w", err)
	}
	
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(parsedURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Required for testing
		},
		Timeout: proxyReadyWaitTime,
	}

	for i := 0; i < maxAttempts; i++ {
		resp, err := client.Head("http://example.com")
		if err == nil {
			resp.Body.Close()
			tb.Logf("Proxy server is ready")
			return nil
		}

		tb.Logf("Waiting for proxy to be ready (attempt %d/%d)...", i+1, maxAttempts)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("proxy server not ready after %d attempts", maxAttempts)
}
