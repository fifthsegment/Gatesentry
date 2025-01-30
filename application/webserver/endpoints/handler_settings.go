package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"
	"time"

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
	case "blocktimes", "strictness", "timezone", "idemail", "enable_https_filtering", "capem", "keypem", "enable_dns_server", "dns_custom_entries", "ai_scanner_url", "enable_ai_image_filtering", "EnableUsers":
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
		requestedId == "enable_dns_server" ||
		requestedId == "enable_https_filtering" ||
		requestedId == "enable_ai_image_filtering" ||
		requestedId == "ai_scanner_url" || requestedId == "EnableUsers" || requestedId == "strictness" || requestedId == "capem" || requestedId == "keypem" {
		settings.Update(requestedId, temp.Value)
	}

	// fmt.Println( temp );
	return temp
}
