package gatesentryf

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/internalfiles"
	gatesentry2proxy "bitbucket.org/abdullah_irfan/gatesentryf/proxy"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"bitbucket.org/abdullah_irfan/gatesentryproxy"

	// "gatesentry2/internalfiles"
	// "io/ioutil"
	// "os"
	// "runtime"
	// "syscall"
	// "path/filepath"
	"io/ioutil"
	"os"
	"runtime"
	"syscall"

	gatesentry2filters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

//   _____       _       _____            _
//  |  __ \     | |     /  ___|          | |
//  | |  \/ __ _| |_ ___\ `--.  ___ _ __ | |_ _ __ _   _
//  | | __ / _` | __/ _ \`--. \/ _ \ '_ \| __| '__| | | |
//  | |_\ \ (_| | ||  __/\__/ /  __/ | | | |_| |  | |_| |
//   \____/\__,_|\__\___\____/ \___|_| |_|\__|_|   \__, |
//                                                  __/ |
//                                                 |___/

const GSKEEPALIVETIMEOUT = 60 * 20 //minutes
const NONALIVESBEFOREKILL = 20
const CONSUMPTIONUPDATEINTERVAL = 60 * 10
const NONCONSUMPTIONUPDATESBEFOREKILL = 24

var INSTALLATIONID = "a"
var GSAPIBASEPOINT = "a"
var GSBASEDIR = "./"
var GSBASEPATH = "/"

// const INSTALLATIONID = "3";
var GSVerString = ""

func SetGSVer(v string) {
	GSVerString = v
}

func SetInstallationID(a string) {
	INSTALLATIONID = a
}

func SetAPIBaseEndpoint(a string) {
	GSAPIBASEPOINT = a
}

type GSRuntime struct {
	WebServerPort               int
	FilterFiles                 map[string]string
	MemLog                      []GSFilterLog
	MemLogSz                    int
	Filters                     []gatesentry2filters.GSFilter
	GSWebSettings               *gatesentry2storage.MapStore
	GSSettings                  *gatesentry2storage.MapStore
	GSUpdateLog                 *gatesentry2storage.MapStore
	Logger                      *gatesentry2logger.Log
	Proxy                       *gatesentry2proxy.GSProxy
	AuthUsers                   []GatesentryTypes.GSUser
	FailedConsumptionUpdates    int
	GSUserDataSaverRunning      bool
	GSKeepSentryAliveRunning    bool
	GSConsumptionUpdaterRunning bool
	DNSServerChannel            chan int
	BoundAddress                *string
	DnsServerInfo               *GatesentryTypes.DnsServerInfo
}

func SetBaseDir(a string) {
	GSBASEDIR = a
}

func GetBaseDir() string {
	return GSBASEDIR
}

// SetBasePath sets the URL base path for reverse proxy deployments.
// Normalizes to ensure leading slash, strips trailing slash (unless root "/").
// e.g., "gatesentry" → "/gatesentry", "/gatesentry/" → "/gatesentry", "" → "/"
func SetBasePath(p string) {
	if p == "" || p == "/" {
		GSBASEPATH = "/"
		return
	}
	// Ensure leading slash
	if p[0] != '/' {
		p = "/" + p
	}
	// Strip trailing slash
	if len(p) > 1 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	GSBASEPATH = p
}

func GetBasePath() string {
	return GSBASEPATH
}

func (R *GSRuntime) GSWasUpdated() {
	t := time.Now()
	ts := t.String()
	prevversions := R.GSUpdateLog.Get("versions")
	R.GSUpdateLog.Update("versions", prevversions+R.GSSettings.Get("version")+" - "+R.GetApplicationVersion()+" on = "+ts+",")
	log.Println("GateSentry was updated.")
	R.GSSettings.Update("version", R.GetApplicationVersion())
}

func (R *GSRuntime) UpdateConsumption(consumedBytes int64) {

}

func InitTasks() {
	if runtime.GOOS == "windows" {
		data, err := internalfiles.Asset("zoneinfo.zip")
		if err == nil {
			log.Println("Creating a zoneinfo file")
			err = ioutil.WriteFile(GSBASEDIR+"zoneinfo.zip", data, 0755)
			if err == nil {
				log.Println("Setting zoneinfo env variable")
				zz := "C:\\Users\\dell\\Downloads\\gs\\zoneinfo.zip"
				os.Setenv("ZONEINFO", zz)
				// os.Setenv("ZONEINFO", GSBASEDIR + "\\zoneinfo.zip" )
				syscall.Setenv("ZONEINFO", zz)
				log.Println(os.Getenv("ZONEINFO"))
				log.Println(syscall.Getenv("ZONEINFO"))
			} else {
				log.Println(err.Error())
			}
		} else {
			log.Println(err.Error())
		}
	}
}

func (R *GSRuntime) Init() {
	startuptext := ` +-+-+-+-+-+-+-+-+-+-+
    |G|a|t|e|S|e|n|t|r|y|
    +-+-+-+-+-+-+-+-+-+-+`
	fmt.Println(startuptext)

	// kill process on port 53
	if runtime.GOOS == "windows" {
		cmd := exec.Command("netstat", "-ano", "|", "findstr", "53")
		out, err := cmd.Output()
		if err != nil {
			log.Println(err.Error())
		}
		log.Println(string(out))
	} else {
		cmd := exec.Command("lsof", "-i", ":53")
		out, err := cmd.Output()
		if err != nil {
			log.Println(err.Error())
		}
		log.Println(string(out))
	}

	InitTasks()
	R.MemLogSz = 1024
	R.MemLog = make([]GSFilterLog, R.MemLogSz)
	R.DnsServerInfo = &GatesentryTypes.DnsServerInfo{}

	filters := []gatesentry2filters.GSFilter{}
	gatesentry2filters.SetBaseDir(GSBASEDIR)
	R.Filters = gatesentry2filters.LoadFilters(filters)
	R.AuthUsers = []GatesentryTypes.GSUser{}
	gatesentry2storage.SetBaseDir(GSBASEDIR)
	log.Println("Making a new MapStore for GSSettings")
	R.GSSettings = gatesentry2storage.NewMapStore("GSSettings", true)
	R.GSUpdateLog = gatesentry2storage.NewMapStore("GSUpdateLog", false)
	R.GSSettings.SetDefault("strictness", "2000")
	R.GSSettings.SetDefault("general_settings", "{\"log_location\": \"./log.db\", \"admin_password\": \"admin\", \"admin_username\": \"admin\" }")
	R.GSSettings.SetDefault("blocktimes", "{\"fromhours\":0,\"tohours\":0,\"fromminutes\":58,\"tominutes\":59}")
	R.GSSettings.SetDefault("authusers", "[{\"username\": \"guest\", \"password\": \"\",\"Base64String\":\"Z3Vlc3Q6cGFzc3dvcmQ=\", \"allowaccess\": true, \"dataconsumed\": 0 }]")
	// R.GSSettings.Update("authusers", "[{\"user\": \"guest\", \"pass\": \"guest\", \"allowaccess\": true, \"dataconsumed\": 0 }]" );
	R.GSSettings.SetDefault("EnableUsers", "false")
	R.GSSettings.SetDefault("NonAlives", "0")
	R.GSSettings.SetDefault("Noheartbeat", "0")
	R.GSSettings.SetDefault("Noheartbeatmessage", "")
	R.GSSettings.SetDefault("timezone", "Europe/Oslo")
	R.GSSettings.SetDefault("enable_https_filtering", "false")
	R.GSSettings.SetDefault("enable_dns_server", "true")
	// Use environment variable for DNS resolver if set, otherwise use default
	// Environment variable takes precedence over stored settings to allow
	// containerized/deployment-time configuration
	if envResolver := os.Getenv("GATESENTRY_DNS_RESOLVER"); envResolver != "" {
		// Normalize resolver address - ensure port is included
		// Use net.SplitHostPort to properly handle IPv6 addresses
		dnsResolverValue := envResolver
		_, _, err := net.SplitHostPort(envResolver)
		if err != nil {
			// No port specified, add default :53
			// net.JoinHostPort handles IPv6 bracketing automatically
			dnsResolverValue = net.JoinHostPort(envResolver, "53")
		}
		log.Printf("[DNS] Using resolver from environment (overrides settings): %s", dnsResolverValue)
		R.GSSettings.Update("dns_resolver", dnsResolverValue)
	} else {
		R.GSSettings.SetDefault("dns_resolver", "8.8.8.8:53")
	}
	R.GSSettings.SetDefault("idemail", "")
	R.GSSettings.SetDefault("enable_ai_image_filtering", "false")
	R.GSSettings.SetDefault("ai_scanner_url", "")

	R.GSSettings.SetDefault("version", R.GetApplicationVersion())
	R.GSUpdateLog.SetDefault("versions", "")

	log.Println("Version from file = " + R.GSSettings.Get("version"))
	if R.GetApplicationVersion() != R.GSSettings.Get("version") {
		R.GSWasUpdated()
	} else {

		R.GSSettings.Update("version", R.GetApplicationVersion())
	}
	R.GSSettings.SetDefault("capem", `-----BEGIN CERTIFICATE-----
MIIFFzCCAv+gAwIBAgIURmnEBuLr2cgTyvzT8Wq768X41z0wDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAwwQR2F0ZVNlbnRyeUZpbHRlcjAeFw0yNTAyMjUxMTU2MDRa
Fw0yNzAzMTcxMTU2MDRaMBsxGTAXBgNVBAMMEEdhdGVTZW50cnlGaWx0ZXIwggIi
MA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQDBBzEQSOgshnS2BKyKXvRCv9Sk
/2QJYJz6/AML6C37vcEtRx8tQkXJoEXnxIfhGSK15Xm5zdShTzyC2OXsie6KZspo
/+K7Cv07C0zVVbrDmE3rjoiNYgEKlJtbrHYtEPsQwSd0TKhKQW+txv3PPkB3FhGx
eNjHyUtl3Qo8yq/dLarF90TjNKCSA63dQd9VV90mgg1LxZTFoGkS4Ae4Onxj9Zs1
vy4jEjHDZ9V98OsGwe4QwADRT6vqs9v3Ng2r1vmuKdWRIsQ+dR6ulv4M9At+YMZ4
Sd8xVV5IODgdPnWh0pxP/CKejVIAUjTkvQ2pw2R/7hywyE/vjz5RwZ7T3vkeCXlI
TgXScWjttNuebyci7Ub0BBTyaGXHSGPua5myrPb9nPu6LrazBv3BoO/FEvZRj7Na
+mgvs6j7XNMDBotuPeE0Fz/VWLDNU846X2D5c8HMfn8635CDxRG/F4SsFkqyEMOf
NXW/X01v+pVc5MDafG0+IAAssqTw1rRANE0jzB03BjX5OSMxf3kHjhqF6QiYHp9F
0jv8QWTm9b/IvoOXIXJYaShh6313WPvwJfPButSg0eMh9Fp9zYfEX+yRX5Zv7OOU
1QXlbcIu9IUG8M2xRiNLFVWLkjPC6sAiHNplJ5tPW0chF1XpyOaEnWTLRumgWger
OPSSYiY88iK5fN/KKwIDAQABo1MwUTAdBgNVHQ4EFgQUlVleVZqWSkX0ygNBDG2C
WjYH5OgwHwYDVR0jBBgwFoAUlVleVZqWSkX0ygNBDG2CWjYH5OgwDwYDVR0TAQH/
BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAgEAvQgPHNKn49/fBvBd+atTrs5KvudX
DFMKk6zrPe9STZwQCjpHpEREXinFNPJRFEmaTT7Im+AA09n+bR+YDErswKdv2Tof
4muxNw8gv4uph6vpRG54h4Ox0v949+c1rOGP7u35IITcGHPES6NMrqnlaR6M2Tnt
KQZKLxMDPl4B/E2TrA6m+aw3yQS2bDx8weZ7mMIwrB19tm2iJoGr38Cy2KyX2E8s
Gxauz8moXaKKDKXHJLZxwQk/SSd7WaWT0kjIQ5JiM6vywkKwtG/JlVlh9lk1jiVE
RZBzdYH/9YZKy59XH+FFI4pyKTm55aGtH76PUG7/X5ehXIHQCU8OpliPNtobZ9ni
x6Wa61sN38IhfRkwZqOV6AGE/HYqTmGGZviuluRDK/SQB041V0j+6mm0ql5WNzcx
wUsaI+1ZZUCZ7OhJuO7gn4VLsvyKfU3zAIFP/oiuj9XzkMsatdGNc1SeNUoos8yU
03evBxoEMTCHwdNCBQcxRboaefCsBPEgBq3bWJiz0IRLg/CxaeJ0ZRG0pcggA3hR
ILnnVNXSvvo5UuIXr9RyLQmkFtIAVvOBEqG6ua7CXgQifZnmVzvOf7DiQ1DKpT5D
HOku5ntRzKZF0EaKMndLxE7ui+NJOtz4VN8H1qmnHgejFNJRANQQmkLToB1wRQ+6
mMYOORHnp9ly0p8=
-----END CERTIFICATE-----`)
	R.GSSettings.SetDefault("keypem", `-----BEGIN PRIVATE KEY-----
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
llHxr1oRgfKfh/NFn7AGoS8sGIRVE80P
-----END PRIVATE KEY-----`)
	R.GSSettings.SetDefault("dns_custom_entries", `[
		"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
		"https://raw.githubusercontent.com/anudeepND/blacklist/master/adservers.txt",
		"https://v.firebog.net/hosts/AdguardDNS.txt",
		"https://raw.githubusercontent.com/PolishFiltersTeam/KADhosts/master/KADhosts.txt",
		"https://raw.githubusercontent.com/FadeMind/hosts.extras/master/add.Spam/hosts",
		"https://v.firebog.net/hosts/static/w3kbl.txt",
		"https://adaway.org/hosts.txt",
		"https://v.firebog.net/hosts/RPiList-Phishing.txt",
		"https://v.firebog.net/hosts/RPiList-Malware.txt",
		"https://gitlab.com/quidsup/notrack-blocklists/raw/master/notrack-malware.txt",
		"https://pgl.yoyo.org/adservers/serverlist.php?hostformat=hosts&showintro=0&mimetype=plaintext",
		"https://bitbucket.org/ethanr/dns-blacklists/raw/8575c9f96e5b4a1308f2f12394abd86d0927a4a0/bad_lists/Mandiant_APT1_Report_Appendix_D.txt",
		"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/popupads-onlydomains.txt",
		"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/tif-onlydomains.txt"
	]`)

	general_settings := R.GSSettings.Get("general_settings")
	general_settings_parsed := gatesentryWebserverTypes.GSGeneral_Settings{}
	json.Unmarshal([]byte(general_settings), &general_settings_parsed)

	log_location := general_settings_parsed.LogLocation
	log_location = strings.Replace(log_location, "./", "", -1)
	if !strings.HasPrefix(log_location, "/tmp") {
		log_location = GSBASEDIR + log_location
	}

	if R.Logger == nil {
		R.Logger = gatesentry2logger.NewLogger(log_location)
	} else {
		log.Println("Gatesentry Logger already exists")
	}

	for i := 0; i < len(R.Filters); i++ {
		// log.Println( R.GSSettings.GetInt("strictness") )
		R.Filters[i].Strictness = R.GSSettings.GetInt("strictness")
	}
	R.LoadUsers()
	//
	R.GSUserRunDataSaver()

	//R.KeepAliveMonitor()

	/**
	 * 1.73
	 * Disabling Consumption Updater
	 * Consumption Updater sends utilization data (bytes passed through GS) to the main server
	 */
	ConsumptionUpdater()

	R.ReloadCertificate()

	go func() {
		dnsEnabled := R.GSSettings.Get("enable_dns_server")
		log.Println("DNS server setting = " + dnsEnabled)
		if dnsEnabled == "true" {
			R.DNSServerChannel <- 1
		} else {
			R.DNSServerChannel <- 2
		}
	}()
}

func (R *GSRuntime) GetInstallationId() string {
	return INSTALLATIONID
}

func GetApplicationVersion() string {
	return GSVerString
}

func (R *GSRuntime) GetApplicationVersion() string {
	return GetApplicationVersion()
}

func (R *GSRuntime) ReloadCertificate() {
	capembytes := []byte(R.GSSettings.Get("capem"))
	keypembytes := []byte(R.GSSettings.Get("keypem"))

	gatesentryproxy.InitWithDataCerts(capembytes, keypembytes)
}

func (R *GSRuntime) GetTotalConsumptionData() (string, string) {
	dd, msg, err := GSGetConsumptionData(R.GetInstallationId())
	if err != nil {
		return "Unable to get Data", ""
	}
	data := GetHumanDataSize(dd)
	return data, msg
}

func (R *GSRuntime) OnHeartbeat() {
	R.GSSettings.Update("Noheartbeat", "0")
	R.GSSettings.Update("Noheartbeatmessage", "")
}

func (R *GSRuntime) OnNoHeartbeat(message string) {
	R.GSSettings.Update("Noheartbeat", "1")
	R.GSSettings.Update("Noheartbeatmessage", message)
}

func (R *GSRuntime) IsBlackoutModeActive() (bool, string) {
	/*
	   * Temporarily disabled
	   if ( R.FailedConsumptionUpdates > NONCONSUMPTIONUPDATESBEFOREKILL){
	      return true, "Unable to connect to GateSentry's Main Server.";
	   }
	*/
	active := R.GSSettings.Get("Noheartbeat")
	if active == "1" {
		message := R.GSSettings.Get("Noheartbeatmessage")
		if message == "EOF" {
			return true, "Unable to get a Keep Alive from GateSentry's Main Server."
		}
		return true, message
	}
	return false, ""
}
