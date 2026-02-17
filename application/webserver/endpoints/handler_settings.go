package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"
	"time"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/badoux/checkmail"
)

func GSApiSettingsGET(requestedId string, settings *gatesentry2storage.MapStore) interface{} {
	switch requestedId {
	case "general_settings":
		value := settings.Get(requestedId)
		general_settings_parsed := gatesentryWebserverTypes.GSGeneral_Settings{}
		json.Unmarshal([]byte(value), &general_settings_parsed)
		general_settings_parsed.AdminPassword = ""
		valueJson, err := json.Marshal(general_settings_parsed)
		if err != nil {
			value = settings.Get(requestedId)
		} else {
			value = string(valueJson)
		}
		return struct{ Value string }{Value: value}
	case "blocktimes", "strictness", "timezone", "idemail", "enable_https_filtering", "capem", "keypem", "enable_dns_server", "enable_dns_filtering", "dns_custom_entries", "dns_domain_lists", "dns_whitelist_domain_lists", "ai_scanner_url", "enable_ai_image_filtering", "EnableUsers", "dns_resolver", "wpad_enabled", "wpad_proxy_host", "wpad_proxy_port", "wpad_bypass_domain_lists", "dns_local_zone", "ddns_enabled", "ddns_tsig_required", "ddns_tsig_key_name", "ddns_tsig_key_secret":
		value := settings.Get(requestedId)
		return struct {
			Key   string
			Value string
		}{Key: requestedId, Value: value}
	case "timenow":
		t := time.Now()
		loc, _ := time.LoadLocation(settings.Get("timezone"))
		t = t.In(loc)
		value := t.Format(time.UnixDate)

		return struct {
			Key   string
			Value string
		}{Key: requestedId, Value: value}
	}
	return nil
}

func GSApiSettingsPOST(requestedId string, settings *gatesentry2storage.MapStore, temp gatesentryWebserverTypes.Datareceiver) interface{} {

	switch requestedId {
	case "idemail":
		err := checkmail.ValidateFormat(temp.Value)
		if err != nil {
			temp.Value = "ERROR: Unable to Validate your email"
			// fmt.Printf("Code: %s, Msg: %s", smtpErr.Code(), smtpErr)
			// fmt.Fprint(w, "Unable to validate your email address.");
			return temp
		}
	}

	if requestedId == "general_settings" {
		log.Println("Updating general settings")
		general_settings_parsed := gatesentryWebserverTypes.GSGeneral_Settings{}
		json.Unmarshal([]byte(temp.Value), &general_settings_parsed)
		pwd := general_settings_parsed.AdminPassword
		if pwd != "" {
			settings.Update(requestedId, temp.Value)
		} else {
			general_settings_parsed.AdminPassword = gatesentryWebserverTypes.GetAdminPassword(settings)
			// convert general_settings_parsed to json
			valueJson, err := json.Marshal(general_settings_parsed)
			if err != nil {
				log.Fatal("Unable to marshal general settings")
			} else {
				//convert valuejSON to string
				settings.Update(requestedId, string(valueJson))
			}
		}
	}

	if requestedId == "dns_custom_entries" ||
		requestedId == "dns_domain_lists" ||
		requestedId == "dns_whitelist_domain_lists" ||
		requestedId == "enable_dns_server" ||
		requestedId == "enable_dns_filtering" ||
		requestedId == "enable_https_filtering" ||
		requestedId == "enable_ai_image_filtering" ||
		requestedId == "ai_scanner_url" ||
		requestedId == "EnableUsers" ||
		requestedId == "strictness" ||
		requestedId == "capem" ||
		requestedId == "keypem" ||
		requestedId == "dns_resolver" ||
		requestedId == "wpad_enabled" ||
		requestedId == "wpad_proxy_host" ||
		requestedId == "wpad_proxy_port" ||
		requestedId == "wpad_bypass_domain_lists" ||
		requestedId == "dns_local_zone" ||
		requestedId == "ddns_enabled" ||
		requestedId == "ddns_tsig_required" ||
		requestedId == "ddns_tsig_key_name" ||
		requestedId == "ddns_tsig_key_secret" {
		settings.Update(requestedId, temp.Value)
		if requestedId == "dns_resolver" {
			gatesentryDnsServer.SetExternalResolver(temp.Value)
		}
		// Update DNS zones at runtime
		if requestedId == "dns_local_zone" {
			gatesentryDnsServer.SetDNSZones(temp.Value)
		}
		// Immediately reload the proxy certificate when either PEM is updated
		if requestedId == "capem" || requestedId == "keypem" {
			ReloadProxyCertificate(settings)
		}
		// Sync WPAD DNS interception with the setting
		if requestedId == "wpad_enabled" {
			gatesentryDnsServer.SetWPADEnabled(temp.Value == "true")
		}
		// Sync DNS domain filtering with the setting
		if requestedId == "enable_dns_filtering" {
			gatesentryDnsServer.SetDNSFilteringEnabled(temp.Value == "true")
		}
		// Sync DDNS settings at runtime
		if requestedId == "ddns_enabled" {
			gatesentryDnsServer.SetDDNSEnabled(temp.Value == "true")
		}
		if requestedId == "ddns_tsig_required" {
			gatesentryDnsServer.SetDDNSTSIGRequired(temp.Value == "true")
		}
		if requestedId == "ddns_tsig_key_name" || requestedId == "ddns_tsig_key_secret" {
			gatesentryDnsServer.UpdateTSIGKey(settings.Get("ddns_tsig_key_name"), settings.Get("ddns_tsig_key_secret"))
		}
	}

	// fmt.Println( temp );
	return temp
}
