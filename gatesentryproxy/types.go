package gatesentryproxy

import (
	"regexp"
	"strings"
	"sync"
)

// regexCache keeps compiled URL patterns: a policy is evaluated per request,
// and compiling the same pattern on every request would put regexp
// compilation on the hot path.
var regexCache sync.Map

func compileCached(pattern string) (*regexp.Regexp, error) {
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}

type GSProxyPassthru struct {
	Policy           *PolicyDecision
	DontTouch        bool
	User             string
	ProxyActionToLog ProxyAction
}

// PolicyDecision is the policy service's answer for one proxied request, in
// the proxy's own vocabulary so the proxy does not import the application.
//
// Block denies the whole request. BlockURLRegexes and BlockContentTypes deny
// only a matching request URL or response media type, which the proxy can see
// only inside the connection, so a decision carrying them asks for
// inspection. Allow exempts the request from the gateway-wide URL blocklist
// and content-type rules. Reason is shown on the block page and in the log.
type PolicyDecision struct {
	Block             bool
	Allow             bool
	Inspect           bool
	BlockURLRegexes   []string
	BlockContentTypes []string
	Reason            string
}

// BlocksURL reports whether one of the decision's URL patterns matches.
func (d *PolicyDecision) BlocksURL(requestURL string) bool {
	if d == nil {
		return false
	}
	for _, pattern := range d.BlockURLRegexes {
		if re, err := compileCached(pattern); err == nil && re.MatchString(requestURL) {
			return true
		}
	}
	return false
}

// BlocksContentType reports whether a response media type is one the decision
// blocks. Entries are prefixes, so "video/" covers every video type.
func (d *PolicyDecision) BlocksContentType(contentType string) bool {
	if d == nil || contentType == "" {
		return false
	}
	for _, blocked := range d.BlockContentTypes {
		if strings.HasPrefix(contentType, blocked) {
			return true
		}
	}
	return false
}

type GSResponder struct {
	Changed bool
	Data    []byte
}

type GSHandler struct {
	Id     string
	Handle func(*GSContentFilterData)
}

type GSUserCached struct {
	User string
	Pass string
}

type GSProxy struct {
	AuthHandler        func(authheader string) bool
	ContentHandler     func(*GSContentFilterData)
	ContentTypeHandler func(*GSContentTypeFilterData)
	ContentSizeHandler func(GSContentSizeFilterData)
	UserAccessHandler  func(*GSUserAccessFilterData)
	TimeAccessHandler  func(*GSTimeAccessFilterData)
	UrlAccessHandler   func(*GSUrlFilterData)
	ProxyErrorHandler  func(*GSProxyErrorData)
	DoMitm             func(host string) bool
	IsExceptionUrl     func(url string) bool
	IsAuthEnabled      func() bool
	LogHandler         func(GSLogData)
	// RuleMatchHandler returns the policy decision for a request, or nil when
	// no policy decides it and the gateway's own settings stay in charge.
	// ClientIP is the observed source address, used only as a device lookup
	// key; it is never itself treated as the authenticated identity.
	RuleMatchHandler func(domain string, user string, clientIP string) *PolicyDecision
	// DeviceObservationHandler is called (asynchronously by the caller) with
	// the observed source address of a proxied connection so the device
	// inventory can record it. Routed/transparent traffic arrives from
	// addresses the DNS path may never see (e.g. VPN or exit-node ranges),
	// so the proxy is itself evidence the address is active.
	DeviceObservationHandler func(clientIP string)
	// PolicyBlockPage renders the page shown when a policy denies a request.
	PolicyBlockPage func(reason string) []byte
	Handlers         map[string][]*GSHandler
	UsersCache       map[string]GSUserCached
}

// For the refactored filter input
type GSContentFilterData struct {
	Url                  string
	ContentType          string
	Content              []byte
	FilterResponse       []byte
	FilterResponseAction ProxyAction
}

type GSContentTypeFilterData struct {
	Url                  string
	ContentType          string
	FilterResponseAction ProxyAction
	FilterResponse       []byte
}

type GSContentSizeFilterData struct {
	Url         string
	ContentType string
	ContentSize int64
	User        string
}

type GSUserAccessFilterData struct {
	User                 string
	FilterResponseAction ProxyAction
	FilterResponse       []byte
}

type GSTimeAccessFilterData struct {
	Url                  string
	ContentType          string
	User                 string
	FilterResponseAction string
	FilterResponse       []byte
}

type GSLogData struct {
	Url         string
	ContentType string
	User        string
	Action      ProxyAction
	// ClientIP is the observed source address, used as a device lookup
	// key by the LogHandler closure that translates GSLogData into a
	// policy.Decision. It is never itself treated as the authenticated
	// identity.
	ClientIP string
	// Layer identifies which proxy path produced this log entry
	// ("explicit_proxy", "transparent_proxy", or "content"). The closure
	// maps it to a policy.DecisionLayer.
	Layer string
	// Reason names the policy rule that decided a policy block, so the log
	// says which rule to edit rather than only that something blocked.
	Reason string
}

type GSUrlFilterData struct {
	Url                  string
	User                 string
	FilterResponseAction ProxyAction
	FilterResponse       []byte
}

type GSProxyErrorData struct {
	Error          string
	FilterResponse []byte
}
