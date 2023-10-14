package gatesentryf

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/internalfiles"
	gatesentry2proxy "bitbucket.org/abdullah_irfan/gatesentryf/proxy"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

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

// const INSTALLATIONID = "3";
var GSVer = float32(1.0)
var GSVerString = "1.0"

func SetGSVer(v float32) {
	GSVer = v
	GSVerString = fmt.Sprintf("%f", GSVer)
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
}

func SetBaseDir(a string) {
	GSBASEDIR = a
}

func GetBaseDir() string {
	return GSBASEDIR
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

func (R *GSRuntime) init() {
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
MIICxjCCAi+gAwIBAgIUTq5PcMI3QaCgB8dPvqRYv7QBTBswDQYJKoZIhvcNAQEL
BQAwdTELMAkGA1UEBhMCVVMxFTATBgNVBAcMDERlZmF1bHQgQ2l0eTEZMBcGA1UE
CgwQR2F0ZVNlbnRyeUZpbHRlcjEZMBcGA1UECwwQR2F0ZVNlbnRyeUZpbHRlcjEZ
MBcGA1UEAwwQR2F0ZVNlbnRyeUZpbHRlcjAeFw0yMTA5MTcwNTQ1MjNaFw0yNDEy
MzAwNTQ1MjNaMHUxCzAJBgNVBAYTAlVTMRUwEwYDVQQHDAxEZWZhdWx0IENpdHkx
GTAXBgNVBAoMEEdhdGVTZW50cnlGaWx0ZXIxGTAXBgNVBAsMEEdhdGVTZW50cnlG
aWx0ZXIxGTAXBgNVBAMMEEdhdGVTZW50cnlGaWx0ZXIwgZ8wDQYJKoZIhvcNAQEB
BQADgY0AMIGJAoGBAMjHspkfXfFf8VReL+XIwbuQ4tyoVYyF3ei5SiFDPV348qAF
ElNGXpxXtBo0wW4Ze4BrFq4hlCSlJ0Md+dCM9Ydv8ot4cTH0fBHyzyWFrM+4OGp7
7wt8c1MaitCXHQr/Qv3XaL310LhhFqHWVUHN2AnIC45bvHs4oBMPEgDeZ/XPAgMB
AAGjUzBRMB0GA1UdDgQWBBScjV6BX5IOujFu2zs1CIkX7/2mPDAfBgNVHSMEGDAW
gBScjV6BX5IOujFu2zs1CIkX7/2mPDAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4GBACyUOwcf04ILzpuBKFkqptW0d4s4dAZARlE689DwZwPA3fy6u5Lk
3mhs+KuZQwnuaXioKHO2ETY9tzWswPhJy6Er8ciDzLTNdtN4xGpBYD2Cq9J+NQlT
jf6P7vZONRTILl3/EGql4swxUTTPuvpIbkEECwPBBx+9say8e5fQ86zL
-----END CERTIFICATE-----`)
	R.GSSettings.SetDefault("keypem", `-----BEGIN PRIVATE KEY-----
MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAMjHspkfXfFf8VRe
L+XIwbuQ4tyoVYyF3ei5SiFDPV348qAFElNGXpxXtBo0wW4Ze4BrFq4hlCSlJ0Md
+dCM9Ydv8ot4cTH0fBHyzyWFrM+4OGp77wt8c1MaitCXHQr/Qv3XaL310LhhFqHW
VUHN2AnIC45bvHs4oBMPEgDeZ/XPAgMBAAECgYEAtE2JGDLv5QPYr4AJmVuIhozc
/XT5pkDM/+HtLSO55zrZf1QumbPW4KVt6h64GcwueSsx6dvjsmjRcldn8J21Gnp5
vwWHFhqlvARMGRhqb6CQt2BZyBTY4/0WJlzPB6R536clIPnl7B2KCI2k0vJ3bBl2
MFufx+wZqbUa+gViMLECQQD9ZREBjQTULpAKuQz+WN+ETz778Ca6l/vlRRbpMtsx
46/v147EUpsK77l5YEQ65ROBZSqFZT+nD3KemJ6/WY/3AkEAytgmS1B4lE8P0cD7
LZst8bJESPPN05zmUld0Bp51b7JXgkYXxhZZfPpTca2KyijkmmiqtJKOuYLbJCUW
alwC6QJADrgzP7LQZ/74cRcE0TWablYoI3x003wGru/Pf+ZrYz+FtdoAuhjOVtlM
Hefgrscl1etph+w0wWCdWOcmuZjbSwJAFmJD14vJwpP26u6gySeWqlVBs8szq2Zl
BDEiXJif3PORNI8HkJRmy6PUEXdVGXnpwCBMtiB2H4KRLCvrjVEaAQI/BfrMmS0q
r3jQJqBGV0HT9lE3lnKhJnetFM2muN57tCHRsAVIzepBTcZceFIvonkp2uILW/Gj
wR8g0gOPPV1l
-----END PRIVATE KEY-----`)
	R.GSSettings.SetDefault("dns_custom_entries", `[
		"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
		"https://easylist.to/easylist/easylist.txt",
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
		"https://bitbucket.org/ethanr/dns-blacklists/raw/8575c9f96e5b4a1308f2f12394abd86d0927a4a0/bad_lists/Mandiant_APT1_Report_Appendix_D.txt"
	]`)

	general_settings := R.GSSettings.Get("general_settings")
	general_settings_parsed := gatesentryWebserverTypes.GSGeneral_Settings{}
	json.Unmarshal([]byte(general_settings), &general_settings_parsed)

	log_location := general_settings_parsed.LogLocation
	log_location = strings.Replace(log_location, "./", "", -1)
	if !strings.HasPrefix(log_location, "/tmp") {
		log_location = GSBASEDIR + log_location
	}

	R.Logger = gatesentry2logger.NewLogger(log_location)

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
	ver := strconv.FormatFloat(float64(GSVer), 'g', -1, 32)
	return ver
}

func (R *GSRuntime) GetApplicationVersion() string {
	return GetApplicationVersion()
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
