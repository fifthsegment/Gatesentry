package gatesentryWebserverEndpoints

import (
	"fmt"
	"html"
	"net/http"
)

// BlockedPageHTML returns the HTML for the DNS-level block page.
// This is served when a client requests a domain that was blocked at the DNS level.
// The DNS server resolves blocked domains to GateSentry's own IP, so the browser
// connects to GateSentry and receives this page instead of a connection error.
func BlockedPageHTML(host string) string {
	// Escape host to prevent XSS — host comes from the HTTP Host header
	// which is attacker-controlled.
	host = html.EscapeString(host)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Blocked - GateSentry</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #f5f5f5;
      color: #333;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
    }
    .container {
      text-align: center;
      background: #fff;
      border-radius: 12px;
      box-shadow: 0 4px 24px rgba(0,0,0,0.10);
      padding: 48px 40px;
      max-width: 520px;
      width: 90%%;
    }
    .shield {
      width: 80px;
      height: 80px;
      margin: 0 auto 24px;
    }
    .shield svg {
      width: 100%%;
      height: 100%%;
    }
    h1 {
      font-size: 24px;
      font-weight: 600;
      color: #d32f2f;
      margin-bottom: 12px;
    }
    .domain {
      font-size: 16px;
      color: #555;
      margin-bottom: 24px;
      word-break: break-all;
    }
    .domain strong {
      color: #222;
      font-weight: 600;
    }
    .message {
      font-size: 14px;
      color: #777;
      line-height: 1.6;
      margin-bottom: 24px;
    }
    .footer {
      font-size: 12px;
      color: #aaa;
      border-top: 1px solid #eee;
      padding-top: 16px;
    }
    .blocked-icon {
      display: inline-block;
      width: 64px;
      height: 64px;
      background: #ffebee;
      border-radius: 50%%;
      margin-bottom: 20px;
      line-height: 64px;
      font-size: 32px;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="blocked-icon">🛡️</div>
    <h1>Access Blocked</h1>
    <div class="domain">
      <strong>%s</strong>
    </div>
    <div class="message">
      This website has been blocked by GateSentry content filtering.<br>
      If you believe this is an error, please contact your network administrator.
    </div>
    <div class="footer">
      Protected by GateSentry Web Filter
    </div>
  </div>
</body>
</html>`, host)
}

// GSBlockedPageHandler returns an HTTP handler that serves a block page
// for DNS-blocked domains. When the DNS server resolves a blocked domain
// to GateSentry's IP, the browser connects here and receives this page.
func GSBlockedPageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Strip port if present
		if idx := len(host) - 1; idx > 0 {
			for i := idx; i >= 0; i-- {
				if host[i] == ':' {
					host = host[:i]
					break
				}
				if host[i] == ']' {
					// IPv6 address, stop
					break
				}
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(BlockedPageHTML(host)))
	}
}
