package tests

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
	GSPROXYPORT                     = "10413"
	GSWEBADMINPORT_DEFAULT          = "8080"
)

func TestMain(m *testing.M) {
	// Derive admin port from environment (non-privileged default for dev/CI)
	adminPort := os.Getenv("GS_ADMIN_PORT")
	if adminPort == "" {
		adminPort = GSWEBADMINPORT_DEFAULT
	}

	// Initialize test variables
	proxyURL = "http://localhost:" + GSPROXYPORT
	// Default GS_BASE_PATH is "/gatesentry", so the API lives under that prefix
	basePath := os.Getenv("GS_BASE_PATH")
	if basePath == "" {
		basePath = "/gatesentry"
	}
	if basePath == "/" {
		basePath = ""
	}
	gatesentryWebserverBaseEndpoint = "http://localhost:" + adminPort + basePath + "/api"

	// Wait for server to be ready (assumes it's already running via `make test`)
	fmt.Println("Waiting for Gatesentry server to be ready...")
	client := &http.Client{Timeout: 2 * time.Second}
	serverReady := false
	for i := 0; i < 10; i++ {
		resp, err := client.Get(gatesentryWebserverBaseEndpoint + "/about")
		if err == nil {
			resp.Body.Close()
			fmt.Println("Server is ready!")
			serverReady = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !serverReady {
		fmt.Println("SKIP: Gatesentry server not running. Start it with 'make test'.")
		os.Exit(0)
	}

	// Run tests
	code := m.Run()

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
