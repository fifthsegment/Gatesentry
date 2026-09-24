package policy

import "strings"

// Safe search is enforced by DNS: the search engines and YouTube publish
// restricted-mode host names, and answering the normal host name with the
// restricted one makes every client on the policy use it without a browser
// setting to undo. These are the providers' documented endpoints; mapping only
// the listed host names (never a whole zone) keeps the rest of each service
// working.
var safeSearchTargets = map[string]string{
	"www.youtube.com":          "restrict.youtube.com",
	"m.youtube.com":            "restrict.youtube.com",
	"youtubei.googleapis.com":  "restrict.youtube.com",
	"youtube.googleapis.com":   "restrict.youtube.com",
	"www.youtube-nocookie.com": "restrict.youtube.com",
	"www.bing.com":             "strict.bing.com",
	"duckduckgo.com":           "safe.duckduckgo.com",
	"www.duckduckgo.com":       "safe.duckduckgo.com",
}

// SafeSearchTarget returns the restricted-mode host name a safe-search policy
// answers for a query, or "" when the name is not a search engine host. Google
// serves search from a country domain per region, so every www.google.<tld>
// name maps to Google's SafeSearch endpoint.
func SafeSearchTarget(domain string) string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if target, ok := safeSearchTargets[domain]; ok {
		return target
	}
	if rest, ok := strings.CutPrefix(domain, "www.google."); ok && isGoogleSearchSuffix(rest) {
		return "forcesafesearch.google.com"
	}
	return ""
}

// isGoogleSearchSuffix accepts "com", a two-letter country code, and the
// "co.xx" / "com.xx" forms Google uses for its regional search domains.
func isGoogleSearchSuffix(rest string) bool {
	if rest == "com" {
		return true
	}
	parts := strings.Split(rest, ".")
	switch len(parts) {
	case 1:
		return len(parts[0]) == 2
	case 2:
		return (parts[0] == "co" || parts[0] == "com") && len(parts[1]) == 2
	}
	return false
}
