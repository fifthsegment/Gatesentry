package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"errors"
	"fmt"

	application "bitbucket.org/abdullah_irfan/gatesentryf"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	filters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gresponder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	"bitbucket.org/abdullah_irfan/gatesentryproxy"
	"github.com/jpillora/overseer"
	"github.com/kardianos/service"
	"github.com/steakknife/devnull"
)

// proxyActionToDecisionAction maps a proxy ProxyAction to the shared
// six-value DecisionAction vocabulary so the proxy speaks the same
// language as DNS and content decisions.
func proxyActionToDecisionAction(action gatesentryproxy.ProxyAction) gatesentryPolicy.DecisionAction {
	switch action {
	case gatesentryproxy.ProxyActionBlockedUrl,
		gatesentryproxy.ProxyActionBlockedTime,
		gatesentryproxy.ProxyActionBlockedFileType,
		gatesentryproxy.ProxyActionBlockedMediaContent,
		gatesentryproxy.ProxyActionBlockedTextContent,
		gatesentryproxy.ProxyActionBlockedInternetForUser:
		return gatesentryPolicy.ActionDecisionBlock
	case gatesentryproxy.ProxyActionSSLBump:
		return gatesentryPolicy.ActionDecisionInspect
	case gatesentryproxy.ProxyActionSSLDirect:
		return gatesentryPolicy.ActionDecisionBypass
	case gatesentryproxy.ProxyActionFilterError,
		gatesentryproxy.ProxyActionUserNotFound:
		return gatesentryPolicy.ActionDecisionError
	case gatesentryproxy.ProxyActionUserActive,
		gatesentryproxy.ProxyActionFilterNone:
		return gatesentryPolicy.ActionDecisionAllow
	default:
		return gatesentryPolicy.ActionDecisionUnknown
	}
}

// proxyLayerToDecisionLayer maps the plain-string layer carried by
// GSLogData to the canonical DecisionLayer.
func proxyLayerToDecisionLayer(layer string) gatesentryPolicy.DecisionLayer {
	switch layer {
	case "transparent_proxy":
		return gatesentryPolicy.LayerTransparentProxy
	case "content":
		return gatesentryPolicy.LayerContent
	default:
		return gatesentryPolicy.LayerExplicitProxy
	}
}

// proxyActionProvenance returns the matched-rule label and reason for
// a ProxyAction, so every proxy decision explains itself without a
// second source of truth.
func proxyActionProvenance(action gatesentryproxy.ProxyAction) (matchedRule, reason string) {
	switch action {
	case gatesentryproxy.ProxyActionBlockedUrl:
		return "url filter", "blocked URL"
	case gatesentryproxy.ProxyActionBlockedTime:
		return "time filter", "blocked by schedule"
	case gatesentryproxy.ProxyActionBlockedFileType:
		return "file type filter", "blocked file type"
	case gatesentryproxy.ProxyActionBlockedMediaContent:
		return "media content filter", "blocked media content"
	case gatesentryproxy.ProxyActionBlockedTextContent:
		return "text content filter", "blocked text content"
	case gatesentryproxy.ProxyActionBlockedInternetForUser:
		return "user access", "internet blocked for user"
	case gatesentryproxy.ProxyActionSSLBump:
		return "ssl bump", "TLS inspection enabled"
	case gatesentryproxy.ProxyActionSSLDirect:
		return "ssl direct", "direct tunnel without inspection"
	case gatesentryproxy.ProxyActionFilterError:
		return "", "filter error"
	case gatesentryproxy.ProxyActionUserNotFound:
		return "auth", "user not found"
	case gatesentryproxy.ProxyActionUserActive:
		return "auth", "user active"
	case gatesentryproxy.ProxyActionFilterNone:
		return "", "no matching rule"
	default:
		return "", "unknown"
	}
}

// domainFromURL extracts the hostname from a request URL for decision
// provenance. It falls back to the raw string if parsing fails.
func domainFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Hostname()
}

// policyProxyDecision returns the proxy enforcement decision for a request
// context. The policy service owns every policy decision: a group, its rules,
// scoped exceptions, and pauses all live in one evaluator, so there is no
// second rule engine to fall through to. A nil result means no group decided
// the request and the gateway's own settings stay in charge.
func policyProxyDecision(clientIP, user, domain string) interface{} {
	policyService := gatesentryDnsServer.GetPolicyService()
	if policyService == nil {
		return nil
	}
	identity := policyService.ResolveIdentity(clientIP, user)
	match := policyService.EvaluateProxy(identity, domain)
	if !match.Matched {
		return nil
	}
	return match
}

var GSPROXYPORT = "10413"
var GSWEBADMINPORT = "10786"
var GSBASEDIR = ""
var Baseendpointv2 = "https://www.gatesentryfilter.com/api/"
var GATESENTRY_VERSION = "1.27.0"
var GS_BOUND_ADDRESS = ":"
var R *application.GSRuntime

// func ConvertWebPToJPEG(webpData []byte) ([]byte, error) {
// 	// Decode webp bytes to image.Image
// 	img, err := webp.Decode(bytes.NewReader(webpData))
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Encode image.Image to jpeg
// 	var jpegBuf bytes.Buffer
// 	err = jpeg.Encode(&jpegBuf, img, nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return jpegBuf.Bytes(), nil
// }

type ContentScannerInput struct {
	Content     []byte
	ContentType string
	Url         string
}

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	log.Println("Starting up GateSentry")
	p.exit = make(chan struct{})
	startup := make(chan error, 1)
	go p.run(startup)
	return <-startup
}
func (p *program) run(startup chan<- error) error {
	if err := runGateSentry(startup); err != nil {
		log.Printf("GateSentry stopped with error: %v", err)
		return err
	}
	for {
		select {
		case <-p.exit:
			log.Println("Stopping GateSentry")
			application.Stop()
			return nil
		}
	}
}
func (p *program) Stop(s service.Service) error {
	close(p.exit)
	return nil
}

func preupgradeCheck(binpath string) error {
	fmt.Println("Pre upgrade check = " + binpath)
	encoded := application.GetFileHash(binpath)

	if !application.ValidateUpdateHashFromServer(encoded) {
		return errors.New("Unable to validate hash from server")
	}

	return nil
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]) + string(os.PathSeparator) + "gatesentry")
	if err != nil {
		log.Fatal(err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0755)
		if err != nil {
			log.Fatal(err)
		}
	}

	GSBASEDIR = dir
	application.SetBaseDir(dir + "/")
	_ = devnull.Writer
	// log.SetOutput(devnull.Writer)
	overseer.SanityCheck()
	baseendpoint := "http://gatesentryreflector.abdullahirfan.com/api"
	baseendpointforupdates := "http://gatesentryupdates.abdullahirfan.com/api"

	_ = baseendpointforupdates

	updaterInterval := time.Second * 60 * 30

	application.SetGSVer(GATESENTRY_VERSION)
	application.SetAPIBaseEndpoint(baseendpoint)
	// macaddr:=getMac();
	// APPLicense := BuildInstallationIDFromMac(macaddr)
	APPLicense := "NONEXISTENT"
	application.SetInstallationID(APPLicense)

	serviceflag := ""

	flag.StringVar(&serviceflag, "service", "", "")
	flag.Parse()
	// serviceflag = "stop"
	// fmt.Println( len(serviceflag) )
	// return;
	if serviceflag == "start" || serviceflag == "stop" || serviceflag == "uninstall" || serviceflag == "restart" || serviceflag == "install" {
		RunGateSentryServiceRunner(serviceflag)
	} else {
		url := application.GetUpdateBinaryURLOld(baseendpointforupdates)
		_ = updaterInterval
		_ = url
		RunGateSentryServiceRunner("")

	}

}

func prog(state overseer.State) {
	RunGateSentryServiceRunner("")
}

// Service setup.
//
//	Define service config.
//	Create the service.
//	Setup the logger.
//	Handle service controls (optional).
//	Run the service.
func RunGateSentryServiceRunner(svcFlag string) {

	svcConfig := &service.Config{
		Name:        "gatesentry",
		DisplayName: "gatesentry",
		Description: "A web filtering proxy",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	// logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(svcFlag) != 0 {
		err := service.Control(s, svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		// logger.Error(err)
	}
}

func GetRuntime() *application.GSRuntime {
	return application.R
}

func readRuntimeSetting(key, conservativeFallback string) string {
	value, err := R.GSSettings.GetE(key)
	if err != nil {
		log.Printf("Storage error while reading setting %q; retaining known-good value: %v", key, err)
		if value == "" {
			return conservativeFallback
		}
	}
	return value
}

func RunGateSentry() error {
	return runGateSentry(nil)
}

func runGateSentry(startup chan<- error) (returnErr error) {
	startupSignaled := false
	signalStartup := func(err error) {
		if startup != nil && !startupSignaled {
			startup <- err
			startupSignaled = true
		}
	}
	defer func() { signalStartup(returnErr) }()
	// Configure optimization settings via environment variables
	if os.Getenv("GS_DEBUG_LOGGING") == "true" {
		gatesentryproxy.DebugLogging = true
		log.Println("[CONFIG] Debug logging enabled")
	}

	// Allow customizing max content scan size for memory-constrained environments
	if maxScanSize := os.Getenv("GS_MAX_SCAN_SIZE_MB"); maxScanSize != "" {
		if size, err := strconv.ParseInt(maxScanSize, 10, 64); err == nil && size > 0 && size <= 1000 {
			gatesentryproxy.MaxContentScanSize = size * 1024 * 1024 // Convert MB to bytes
			log.Printf("[CONFIG] Max content scan size set to %d MB", size)
		} else if err == nil && size > 1000 {
			log.Printf("[CONFIG] Max content scan size %d MB is too large, using default 10 MB", size)
		} else {
			log.Printf("[CONFIG] Invalid max content scan size value: %s, using default", maxScanSize)
		}
	}

	if transparentPort := os.Getenv("GS_TRANSPARENT_PROXY_PORT"); transparentPort != "" {
		if port, err := strconv.Atoi(transparentPort); err == nil && port > 0 && port <= 65535 {
			gatesentryproxy.SetTransparentProxyPort(port)
			log.Printf("[CONFIG] Transparent proxy port set to %d", port)
		} else {
			log.Printf("[CONFIG] Invalid transparent proxy port: %s, using default %d", transparentPort, gatesentryproxy.GetTransparentProxyPort())
		}
	}

	transparentProxyDisabled := os.Getenv("GS_TRANSPARENT_PROXY") == "false"

	webadminport, err := strconv.Atoi(GSWEBADMINPORT)
	if err != nil {
		return fmt.Errorf("parse web admin port: %w", err)
	}
	R, err = application.Start(webadminport)
	if err != nil {
		return fmt.Errorf("initialize GateSentry: %w", err)
	}
	R.BoundAddress = &GS_BOUND_ADDRESS

	application.StartBonjour()
	gatesentryproxy.InitProxy()
	ngp := gatesentryproxy.NewGSProxy()

	ngp.AuthHandler = func(authheader string) bool {
		if gatesentryproxy.DebugLogging {
			log.Println("Auth header = " + authheader)
		}
		return R.IsUserValid(authheader)
	}

	ngp.ContentHandler = func(gafd *gatesentryproxy.GSContentFilterData) {
		if strings.Contains(gafd.ContentType, "html") {
			responder := &gresponder.GSFilterResponder{Blocked: false}
			application.RunFilter("text/html", string(gafd.Content), responder)
			if responder.Blocked {
				gafd.FilterResponse = []byte(gresponder.BuildResponsePage(responder.Reasons, responder.Score))
				gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedTextContent
			}
		} else {
			mode := readRuntimeSetting("ai_image_filtering_mode", "disabled")
			if mode == filters.AIModeGrok || mode == filters.AIModeChatGPT || mode == filters.AIModeLocal {
				model, key, baseURL := "", "", ""
				switch mode {
				case filters.AIModeGrok:
					key, model = readRuntimeSetting("ai_grok_api_key", ""), readRuntimeSetting("ai_grok_model", "")
				case filters.AIModeChatGPT:
					key, model = readRuntimeSetting("ai_openai_api_key", ""), readRuntimeSetting("ai_openai_model", "")
				case filters.AIModeLocal:
					baseURL, model = readRuntimeSetting("ai_local_llm_url", ""), readRuntimeSetting("ai_local_llm_model", "")
				}
				verdict, err := filters.ClassifyImage(context.Background(), filters.VisionRequest{Provider: mode, APIKey: key, BaseURL: baseURL, Model: model, ContentType: gafd.ContentType, Image: gafd.Content})
				if err == nil && verdict.ShouldBlock(50) {
					gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedMediaContent
					gafd.FilterResponse = []byte(verdict.Reason)
				}
			} else if filters.ShouldRunLegacyImageScanner(readRuntimeSetting("ai_image_filtering_mode", "disabled"), readRuntimeSetting("enable_ai_image_filtering", "false"), readRuntimeSetting("ai_scanner_url", "")) {
				// application.RunFilter("images", string(gafd.Content), responder)
				ai_service_url := readRuntimeSetting("ai_scanner_url", "")
				filters.FilterImagesAI(gafd, ai_service_url)
			}
		}
	}

	ngp.DoMitm = func(host string) bool {
		enable_filtering := readRuntimeSetting("enable_https_filtering", "false")
		if enable_filtering == "true" {
			responder := &gresponder.GSFilterResponder{Blocked: false}
			application.RunFilter("url/https_dontbump", host, responder)
			if responder.Blocked {
				return false
			}
		}
		return enable_filtering == "true"
	}

	ngp.ContentSizeHandler = func(gafd gatesentryproxy.GSContentSizeFilterData) {
		// Simplified: no goroutine overhead for simple operations on low-spec hardware
		// UpdateConsumption is empty, UpdateUserData is simple
		length := gafd.ContentSize
		R.UpdateUserData(gafd.User, uint64(length))
		// R.UpdateConsumption is currently a no-op
	}

	ngp.ContentTypeHandler = func(gafd *gatesentryproxy.GSContentTypeFilterData) {
		contentType := gafd.ContentType
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/all_blocked_mimes", contentType, responder)
		if responder.Blocked {
			// rs.Changed = true
			message := "This content type has been blocked on this network."
			if contentType == "image/png" || contentType == "image/jpeg" || contentType == "image/jpg" || "image/gif" == contentType || "image/webp" == contentType {
				transparentImageBytes, _ := filters.Asset("app/transparent.png")
				gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedFileType
				gafd.FilterResponse = transparentImageBytes
			} else {
				gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedFileType
				gafd.FilterResponse = []byte(gresponder.BuildGeneralResponsePage([]string{message}, -1))
			}
		}
	}

	ngp.TimeAccessHandler = func(gafd *gatesentryproxy.GSTimeAccessFilterData) {
		blockedtimes := readRuntimeSetting("blocktimes", "")
		responder := &gresponder.GSFilterResponder{Blocked: false}
		timezone := readRuntimeSetting("timezone", "UTC")
		filters.RunTimeFilter(responder, blockedtimes, timezone)
		// user := gpt.User
		if responder.Blocked {
			gafd.FilterResponse = []byte(gresponder.BuildGeneralResponsePage([]string{"Internet access on this network has been disabled because the current time has been specified as a blocked time period in GateSentry's settings."}, -1))
		}
	}

	ngp.IsExceptionUrl = func(url string) bool {
		host := url
		log.Println("Running exception handler for = ", host)
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/all_exception_urls", host, responder)
		return responder.Blocked
	}

	ngp.UserAccessHandler = func(gafd *gatesentryproxy.GSUserAccessFilterData) {
		log.Println("Running user access handler")
		if R.UserExists(gafd.User) {
			if R.IsUserActive(gafd.User) {
				gafd.FilterResponseAction = gatesentryproxy.ProxyActionUserActive
			} else {
				gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedInternetForUser
				gafd.FilterResponse = []byte(gresponder.BuildGeneralResponsePage([]string{"Your access has been disabled by the administrator of this network."}, -1))
			}
		} else {
			gafd.FilterResponseAction = gatesentryproxy.ProxyActionUserNotFound
		}
	}

	ngp.IsAuthEnabled = func() bool {
		// *bytesReceived = ResponseAuthError
		temp := readRuntimeSetting("EnableUsers", "false")
		return temp == "true"
	}

	ngp.UrlAccessHandler = func(gafd *gatesentryproxy.GSUrlFilterData) {
		host := gafd.Url
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/all_blocked_urls", host, responder)
		if responder.Blocked {
			gafd.FilterResponseAction = gatesentryproxy.ProxyActionBlockedUrl
			gafd.FilterResponse = []byte(gresponder.BuildGeneralResponsePage([]string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1))
		}
	}

	ngp.LogHandler = func(gafd gatesentryproxy.GSLogData) {
		action := proxyActionToDecisionAction(gafd.Action)
		layer := proxyLayerToDecisionLayer(gafd.Layer)
		decision := gatesentryPolicy.NewDecision(action, layer, domainFromURL(gafd.Url))
		decision.URL = gafd.Url
		decision.ClientIP = gafd.ClientIP
		decision.ResponseType = string(gafd.Action)
		decision.MatchedRule, decision.Reason = proxyActionProvenance(gafd.Action)
		// Resolve identity for device/group provenance. ClientIP is a
		// lookup key only; the authenticated user is kept distinct.
		policySvc := gatesentryDnsServer.GetPolicyService()
		if policySvc != nil {
			identity := policySvc.ResolveIdentity(gafd.ClientIP, gafd.User)
			decision = decision.WithIdentity(identity).WithPolicyRevision(policySvc.Snapshot().Version)
		}
		R.Logger.LogDecision(decision)
	}

	ngp.RuleMatchHandler = func(domain string, user string, clientIP string) interface{} {
		// Policy groups and their rules are the only rule source. clientIP is
		// only a device lookup key, never an identity.
		return policyProxyDecision(clientIP, user, domain)
	}

	// Transparent/routed connections are evidence an address is active even
	// when no DNS query ever arrives (DoH upstreams, cached resolvers), so the
	// proxy feeds the device inventory the addresses it sees. The store
	// deduplicates by IP and refreshes LastSeen, keeping per-device policy
	// bound to routed addresses; a nil store (tests) is simply ignored.
	ngp.DeviceObservationHandler = func(clientIP string) {
		if store := gatesentryDnsServer.GetDeviceStore(); store != nil {
			store.ObservePassiveQuery(clientIP)
		}
	}

	ngp.ProxyErrorHandler = func(gafd *gatesentryproxy.GSProxyErrorData) {
		// clienterror := string(*bytesReceived)
		msg := "Proxy Error. Unable to fulfill your request. <br/><strong>" + gafd.Error + "</strong>."
		switch gafd.Error {
		case "EOF":
			msg = "Proxy Error. Unable to fulfill your request at this time. Please try again in a few seconds."
			break
		default:
			break
		}
		*&gafd.FilterResponse = []byte(gresponder.BuildGeneralResponsePage([]string{msg}, -1))
	}

	// Making a comm channel for our internal dns server
	go application.DNSServerThread(application.GetBaseDir(), R.Logger, R.DNSServerChannel, R.GSSettings, R.GSDevices, R.DnsServerInfo)

	addr := "0.0.0.0:"
	addr += GSPROXYPORT

	ttt := time.NewTicker(time.Second * 10)
	portavailable := false
	for {
		fmt.Println("Listening for proxy connections on : " + GSPROXYPORT)
		ln, err := net.Listen("tcp", ":"+GSPROXYPORT)
		if err != nil {
			fmt.Println("Port is not open for proxy")
		} else {
			portavailable = true
			err = ln.Close()
			fmt.Println("Listening on address:", ln.Addr().String())
			boundAddresses := []string{}
			host, _ := os.Hostname()
			addrs, _ := net.LookupIP(host)
			for _, addr := range addrs {
				if ipv4 := addr.To4(); ipv4 != nil {
					// GS_BOUND_ADDRESS += ipv4.String() + ","
					boundAddresses = append(boundAddresses, ipv4.String()+":"+GSPROXYPORT)
				}
			}
			GS_BOUND_ADDRESS = strings.Join(boundAddresses, ",")

		}

		if portavailable {
			break
		}
		<-ttt.C
	}

	// if portavailable {}

	capembytes := []byte(readRuntimeSetting("capem", ""))
	keypembytes := []byte(readRuntimeSetting("keypem", ""))

	gatesentryproxy.InitWithDataCerts(capembytes, keypembytes)
	proxyListener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen for proxy connections: %w", err)
	}
	signalStartup(nil)
	proxyHandler := gatesentryproxy.ProxyHandler{Iproxy: ngp}

	// ResponseAuthError := []byte(gresponder.BuildGeneralResponsePage([]string{"Your access has been disabled."}, -1))
	// ResponseAccessNotActiveError := []byte(gresponder.BuildGeneralResponsePage([]string{"Your access has been disabled by the administrator of this network."}, -1))

	// ngp.RegisterHandler("proxyerror", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	clienterror := string(*bytesReceived)
	// 	msg := "Proxy Error. Unable to fulfill your request. <br/><strong>" + clienterror + "</strong>."
	// 	switch clienterror {
	// 	case "EOF":
	// 		msg = "Proxy Error. Unable to fulfill your request at this time. Please try again in a few seconds."
	// 		break
	// 	default:
	// 		break
	// 	}
	// 	*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{msg}, -1))
	// })

	// // Should the Proxy MITM this traffic or not
	// ngp.RegisterHandler("mitm", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	log.Println("Running MITM handler")
	// 	// log.Println("GPT = ", gpt)

	// 	enable_filtering := R.GSSettings.GetOrDefault("enable_https_filtering", "")
	// 	log.Println("MITM Handler - enable_https_filtering = " + enable_filtering)
	// 	if enable_filtering == "true" {
	// 		rs.Changed = true
	// 	} else {
	// 		rs.Changed = false
	// 	}
	// 	gpt.DontTouch = true
	// 	// if enable_filtering != "true" {
	// 	// 	gpt.DontTouch = true
	// 	// 	rs.Changed = true
	// 	// 	return
	// 	// }
	// 	// else {
	// 	// 	rs.Changed = false
	// 	// 	gpt.DontTouch = true
	// 	// }
	// 	// host := string(*bytesReceived)
	// 	// responder := &gresponder.GSFilterResponder{Blocked: false}
	// 	// application.RunFilter("url/https_dontbump", host, responder)
	// 	// if responder.Blocked {
	// 	// 	gpt.DontTouch = true
	// 	// 	rs.Changed = true
	// 	// 	return
	// 	// }
	// })

	// ngp.RegisterHandler("except_urls", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	host := string(*bytesReceived)
	// 	log.Println("Running exception handler for = ", host)
	// 	responder := &gresponder.GSFilterResponder{Blocked: false}
	// 	application.RunFilter("url/all_exception_urls", host, responder)
	// 	if responder.Blocked {
	// 		gpt.DontTouch = true
	// 		log.Println("URL found in exception = ", host)
	// 		// *s = []byte(gresponder.BuildGeneralResponsePage( []string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1));
	// 		rs.Changed = true
	// 	}
	// })
	// CONTENT FILTER
	// ngp.RegisterHandler("content", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	log.Println("Running content handler")
	// 	// log.Println("GPT = ", gpt)
	// 	responder := &gresponder.GSFilterResponder{Blocked: false}
	// 	// if !gpt.DontTouch {
	// 	application.RunFilter("text/html", string(*bytesReceived), responder)
	// 	log.Printf("Content filter input = %v", string(*bytesReceived))

	// 	log.Printf("Content filter blocked status = %v", responder.Blocked)

	// 	if responder.Blocked {
	// 		*bytesReceived = []byte(gresponder.BuildResponsePage(responder.Reasons, responder.Score))
	// 	}
	// 	rs.Changed = responder.Blocked
	// 	// }

	// })

	// URL CHECKER
	// ngp.RegisterHandler(gatesentryproxy.FILTER_ACCESS_URL, func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	host := string(*bytesReceived)
	// 	responder := &gresponder.GSFilterResponder{Blocked: false}
	// 	application.RunFilter("url/all_blocked_urls", host, responder)
	// 	if responder.Blocked {
	// 		*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1))
	// 		rs.Changed = true
	// 	}
	// })

	// ngp.RegisterHandler("contentlength", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	length := string(*bytesReceived)
	// 	go func() {
	// 		i, err := strconv.ParseUint(length, 10, 64)
	// 		if err == nil {
	// 			R.UpdateUserData(gpt.User, i)
	// 		}
	// 		consumedBytes, err := strconv.ParseInt(length, 10, 64)
	// 		if err == nil {
	// 			R.UpdateConsumption(consumedBytes)
	// 		}
	// 	}()
	// })

	ngp.RegisterHandler("blockinternet", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		if gpt.User == "admin" {
			rs.Changed = false
		}
	})

	// ngp.RegisterHandler("isauthuser", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	base64string := string(*bytesReceived)
	// 	rs.Changed = false
	// 	if R.IsUserValid(base64string) {
	// 		rs.Changed = true
	// 	}
	// 	log.Println("Status of authuser in isauthuser= ", rs.Changed)
	// })

	// ngp.RegisterHandler("isaccessactive", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	*bytesReceived = ResponseAccessNotActiveError
	// 	rs.Changed = false
	// 	rs.Data = []byte("NONE")
	// 	if R.UserExists(gpt.User) {
	// 		if R.IsUserActive(gpt.User) {
	// 			rs.Data = []byte("ACTIVE")
	// 			rs.Changed = true
	// 		} else {
	// 			rs.Data = []byte("INACTIVE")
	// 			rs.Changed = true
	// 		}
	// 	} else {
	// 		rs.Data = []byte("NOT_FOUND")
	// 		rs.Changed = true
	// 	}
	// })

	// ngp.RegisterHandler("authenabled", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	*bytesReceived = ResponseAuthError
	// 	temp := R.GSSettings.GetOrDefault("EnableUsers", "")
	// 	enableusers := false
	// 	if temp == "true" {
	// 		enableusers = true
	// 	}
	// 	rs.Changed = enableusers
	// })

	// ngp.RegisterHandler("log", func(contentToLog *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	url := string(*contentToLog)
	// 	user := gpt.User
	// 	actionTaken := string(gpt.ProxyActionToLog)
	// 	R.Logger.LogProxy(url, user, actionTaken)
	// })

	// ngp.RegisterHandler(gatesentryproxy.FILTER_TIME, func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	// url := string(*s)
	// 	rs.Changed = false
	// 	blockedtimes := R.GSSettings.GetOrDefault("blocktimes", "")
	// 	responder := &gresponder.GSFilterResponder{Blocked: false}
	// 	timezone := R.GSSettings.GetOrDefault("timezone", "")
	// 	filters.RunTimeFilter(responder, blockedtimes, timezone)
	// 	// user := gpt.User
	// 	if responder.Blocked {
	// 		rs.Changed = true
	// 		*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{"Internet access on this network has been disabled because the current time has been specified as a blocked time period in GateSentry's settings."}, -1))
	// 	}
	// })

	// ngp.RegisterHandler("prerequest", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	rs.Changed = false
	// })

	// ngp.RegisterHandler("youtube", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	url1 := "mainvideo url"
	// 	// Extract video ID
	// 	parts := strings.Split(url1, "/")
	// 	videoID := parts[4]

	// 	// Extract sqp parameter
	// 	sqpIndex := strings.Index(url1, "sqp=")
	// 	sqpEndIndex := strings.Index(url1[sqpIndex:], "|48")
	// 	if sqpEndIndex == -1 {
	// 		sqpEndIndex = len(url1) - sqpIndex
	// 	} else {
	// 		sqpEndIndex += sqpIndex
	// 	}
	// 	sqp := url1[sqpIndex:sqpEndIndex]

	// 	// Extract sigh parameter
	// 	sighIndex := strings.LastIndex(url1, "rs$")
	// 	sigh := url1[sighIndex:]

	// 	// Construct the new URL
	// 	url2 := fmt.Sprintf("https://i.ytimg.com/sb/%s/storyboard3_L2/M2.jpg?%s&sigh=%s", videoID, sqp, sigh)
	// 	fmt.Println(url2)
	// })

	// ngp.RegisterHandler("contentscannerMedia", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	rs.Changed = false
	// 	if R.GSSettings.GetOrDefault("enable_ai_image_filtering", "") == "true" && R.GSSettings.GetOrDefault("ai_scanner_url", "") != "" {
	// 		// convert bytes to json struct of type ContentScannerInput
	// 		var contentScannerInput ContentScannerInput
	// 		err := json.Unmarshal(*bytesReceived, &contentScannerInput)
	// 		if err != nil {
	// 			log.Println("Error unmarshalling content scanner input")
	// 		}
	// 		log.Println("Running content scanner for content type = " + contentScannerInput.ContentType)

	// 		if len(contentScannerInput.Content) < 6000 {
	// 			// continue
	// 		} else if (contentScannerInput.ContentType == "image/jpeg") || (contentScannerInput.ContentType == "image/jpg") || (contentScannerInput.ContentType == "image/png") || (contentScannerInput.ContentType == "image/gif") || (contentScannerInput.ContentType == "image/webp") || (contentScannerInput.ContentType == "image/avif") {
	// 			contentType := contentScannerInput.ContentType
	// 			log.Println("Running content scanner for image")

	// 			// if contentType == "image/jpg" || contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/gif" || contentType == "image/webp" {
	// 			var b bytes.Buffer
	// 			wr := multipart.NewWriter(&b)
	// 			// part, _ := wr.CreateFormFile("image", "uploaded_image"+contentTypeToExt[contentType])

	// 			if contentType == "image/webp" {
	// 				// convert webp to jpeg
	// 				jpegBytes, err := ConvertWebPToJPEG(contentScannerInput.Content)
	// 				if err != nil {
	// 					fmt.Println("Error converting webp to jpeg")
	// 				}
	// 				contentScannerInput.Content = jpegBytes
	// 				contentType = "image/jpeg"
	// 			}

	// 			// Create a new form header for the file

	// 			h := make(textproto.MIMEHeader)
	// 			// ext := contentTypeToExt[contentType]
	// 			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, "uploaded_image"))
	// 			part, _ := wr.CreatePart(h)

	// 			part.Write(*&contentScannerInput.Content)

	// 			b.Bytes()
	// 			wr.Close()

	// 			ai_service_url := R.GSSettings.GetOrDefault("ai_scanner_url", "")
	// 			resp, _ := http.Post(ai_service_url, wr.FormDataContentType(), &b)
	// 			if resp.StatusCode == http.StatusOK {
	// 				bytesLength := len(*bytesReceived)
	// 				// convert bytes length to string
	// 				//
	// 				bytesLengthString := strconv.Itoa(bytesLength)
	// 				log.Println("Inference for " + contentScannerInput.Url + " Content type = " + contentType + "Length = " + bytesLengthString)
	// 				respBytes, _ := io.ReadAll(resp.Body)
	// 				responseString := string(respBytes)
	// 				var inferenceResponse InferenceResponse
	// 				err := json.Unmarshal([]byte(respBytes), &inferenceResponse)
	// 				if err != nil {
	// 					fmt.Println("Error:", err)
	// 					return
	// 				}
	// 				log.Println("Inference Response = " + responseString)
	// 				if inferenceResponse.Category == "sexy" && inferenceResponse.Confidence > 85 {
	// 					rs.Changed = true
	// 				}
	// 				if inferenceResponse.Category == "porn" && inferenceResponse.Confidence > 85 {
	// 					rs.Changed = true
	// 				}
	// 				if len(inferenceResponse.Detections) > 0 {
	// 					var reasonForBlock []string
	// 					var conditionsMet = 0

	// 					for _, detection := range inferenceResponse.Detections {

	// 						if detection.Class == "FEMALE_GENITALIA_EXPOSED" && detection.Score > 0.4 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 2
	// 						}
	// 						if detection.Class == "FEMALE_BREAST_EXPOSED" && detection.Score > 0.4 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 2
	// 						}
	// 						if detection.Class == "FEMALE_BREAST_COVERED" && detection.Score > 0.4 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 1
	// 						}
	// 						if detection.Class == "BELLY_COVERED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 2
	// 						}
	// 						if detection.Class == "ARMPITS_EXPOSED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet++
	// 						}
	// 						if detection.Class == "MALE_GENITALIA_EXPOSED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 2
	// 						}
	// 						if detection.Class == "MALE_BREAST_EXPOSED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet++
	// 						}

	// 						if detection.Class == "BUTTOCKS_EXPOSED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 2
	// 						}

	// 						if detection.Class == "ANUS_EXPOSED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet += 2
	// 						}

	// 						if detection.Class == "BELLY_EXPOSED" && detection.Score > 0.5 {
	// 							reasonForBlock = append(reasonForBlock, " - "+detection.Class+" ("+strconv.FormatFloat(detection.Score, 'f', 2, 64)+")")
	// 							conditionsMet++
	// 						}

	// 					}
	// 					if conditionsMet >= 2 {
	// 						rs.Changed = true
	// 					}

	// 					jsonData, _ := json.Marshal(reasonForBlock)
	// 					rs.Data = jsonData

	// 				}

	// 			} else {
	// 				fmt.Println("Inference for Content type = " + contentType + " failed")
	// 				respBytes, _ := io.ReadAll(resp.Body)

	// 				fmt.Println("Inference Response = " + string(respBytes))
	// 			}
	// 			defer resp.Body.Close()

	// 		}
	// 		// }
	// 	}

	// })

	// ngp.RegisterHandler("contenttypeblocked", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
	// 	contentType := string(*bytesReceived)
	// 	responder := &gresponder.GSFilterResponder{Blocked: false}
	// 	application.RunFilter("url/all_blocked_mimes", contentType, responder)
	// 	// dictionary of contentType to file extension

	// 	if responder.Blocked {
	// 		rs.Changed = true
	// 		message := "This content type has been blocked on this network."
	// 		if contentType == "image/png" || contentType == "image/jpeg" || contentType == "image/jpg" || "image/gif" == contentType || "image/webp" == contentType {
	// 			dat, _ := filters.Asset("app/transparent.png")
	// 			*bytesReceived = dat
	// 		} else {
	// 			*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{message}, -1))
	// 		}
	// 	}
	// })

	if runtime.GOOS == "linux" && !transparentProxyDisabled {
		go func() {
			transparentAddr := "0.0.0.0:" + strconv.Itoa(gatesentryproxy.GetTransparentProxyPort())
			log.Printf("[Transparent] Starting transparent proxy server on %s", transparentAddr)

			transparentServer := gatesentryproxy.NewTransparentProxyServer(&proxyHandler)
			if err := transparentServer.Start(transparentAddr); err != nil {
				log.Printf("[Transparent] Warning: Could not start transparent proxy server: %v", err)
				log.Printf("[Transparent] Continuing without transparent proxy. Set GS_TRANSPARENT_PROXY=false to suppress this warning.")
			} else {
				gatesentryproxy.SetTransparentProxyEnabled(true)
			}
		}()
	}

	server := http.Server{Handler: proxyHandler}
	log.Printf("Starting up...Listening on = %s", addr)
	if err = server.Serve(tcpKeepAliveListener{proxyListener.(*net.TCPListener)}); err != nil {
		return err
	}
	return nil
}

func orPanic(err error) {
	if err != nil {
		panic(err)
	}
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}
