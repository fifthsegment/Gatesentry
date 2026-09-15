package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/badoux/checkmail"
)

func GSApiSettingsGET(requestedId string, settings *gatesentry2storage.MapStore) (interface{}, error) {
	switch requestedId {
	case "general_settings":
		value, err := settings.GetE(requestedId)
		if err != nil {
			return nil, err
		}
		general_settings_parsed := gatesentryWebserverTypes.GSGeneral_Settings{}
		if err := json.Unmarshal([]byte(value), &general_settings_parsed); err != nil {
			return nil, err
		}
		general_settings_parsed.AdminPassword = ""
		general_settings_parsed.AdminUser = ""
		valueJson, err := json.Marshal(general_settings_parsed)
		if err != nil {
			return nil, err
		} else {
			value = string(valueJson)
		}
		return struct{ Value string }{Value: value}, nil
	case "blocktimes", "strictness", "timezone", "idemail", "enable_https_filtering", "capem", "keypem", "enable_dns_server", "dns_custom_entries", "ai_scanner_url", "enable_ai_image_filtering", "ai_image_filtering_mode", "ai_grok_api_key", "ai_openai_api_key", "ai_local_llm_url", "ai_local_llm_model", "ai_grok_model", "ai_openai_model", "EnableUsers", "dns_resolver":
		value, err := settings.GetE(requestedId)
		if err != nil {
			return nil, err
		}
		if requestedId == "ai_grok_api_key" || requestedId == "ai_openai_api_key" {
			return struct {
				Key        string
				Configured bool
			}{Key: requestedId, Configured: strings.TrimSpace(value) != ""}, nil
		}
		if requestedId == "ai_image_filtering_mode" && strings.TrimSpace(value) == "" {
			value = "disabled"
		}
		return struct {
			Key   string
			Value string
		}{Key: requestedId, Value: value}, nil
	case "timenow":
		t := time.Now()
		timezone, err := settings.GetE("timezone")
		if err != nil {
			return nil, err
		}
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
		t = t.In(loc)
		value := t.Format(time.UnixDate)

		return struct {
			Key   string
			Value string
		}{Key: requestedId, Value: value}, nil
	}
	return nil, nil
}

func GSApiSettingsPOST(requestedId string, settings *gatesentry2storage.MapStore, temp gatesentryWebserverTypes.Datareceiver) (interface{}, error) {
	if requestedId == "ai_image_filtering_mode" {
		temp.Value = strings.ToLower(strings.TrimSpace(temp.Value))
		switch temp.Value {
		case "disabled", "grok", "chatgpt", "local":
		default:
			temp.Value = "ERROR: mode must be disabled, grok, chatgpt, or local"
			return temp, nil
		}
	}
	if requestedId == "ai_local_llm_url" {
		v := strings.TrimSpace(temp.Value)
		u, err := url.Parse(v)
		if v != "" && (err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https")) {
			temp.Value = "ERROR: Local LLM URL must be http or https with a host"
			return temp, nil
		}
		temp.Value = v
	}

	switch requestedId {
	case "idemail":
		err := checkmail.ValidateFormat(temp.Value)
		if err != nil {
			temp.Value = "ERROR: Unable to Validate your email"
			// fmt.Printf("Code: %s, Msg: %s", smtpErr.Code(), smtpErr)
			// fmt.Fprint(w, "Unable to validate your email address.");
			return temp, nil
		}
	}

	if requestedId == "general_settings" {
		log.Println("Updating general settings")
		submitted := gatesentryWebserverTypes.GSGeneral_Settings{}
		if err := json.Unmarshal([]byte(temp.Value), &submitted); err != nil {
			return nil, err
		}
		submitted.AdminPassword, submitted.AdminUser = "", ""
		if err := settings.UpdateValue(requestedId, func(current string) (string, error) {
			valueJSON, err := json.Marshal(submitted)
			if err != nil {
				return "", err
			}
			return string(valueJSON), nil
		}); err != nil {
			return nil, err
		}
	}

	if requestedId == "dns_custom_entries" ||
		requestedId == "enable_dns_server" ||
		requestedId == "enable_https_filtering" ||
		requestedId == "enable_ai_image_filtering" ||
		requestedId == "ai_scanner_url" ||
		requestedId == "ai_image_filtering_mode" || requestedId == "ai_grok_api_key" || requestedId == "ai_openai_api_key" || requestedId == "ai_local_llm_url" || requestedId == "ai_local_llm_model" || requestedId == "ai_grok_model" || requestedId == "ai_openai_model" ||
		requestedId == "EnableUsers" ||
		requestedId == "strictness" ||
		requestedId == "capem" ||
		requestedId == "keypem" ||
		requestedId == "dns_resolver" {
		updates := map[string]string{requestedId: temp.Value}
		if requestedId == "ai_image_filtering_mode" {
			updates["enable_ai_image_filtering"] = strconv.FormatBool(temp.Value != "disabled")
		}
		if err := settings.UpdateValues(updates); err != nil {
			return nil, err
		}
		if requestedId == "dns_resolver" {
			gatesentryDnsServer.SetExternalResolver(temp.Value)
		}
	}

	// fmt.Println( temp );
	return temp, nil
}
