package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"
	"time"

	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/badoux/checkmail"
	"github.com/kataras/iris/v12"
)

func GSApiSettingsGET(ctx iris.Context, settings *gatesentryWebserverTypes.SettingsStore) {
	requestedId := ctx.Params().Get("id")
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
		ctx.JSON(struct{ Value string }{Value: value})
		break
	case "blocktimes", "strictness", "timezone", "idemail", "enable_https_filtering", "capem", "keypem", "enable_dns_server":
		value := settings.Get(requestedId)
		ctx.JSON(struct {
			Key   string
			Value string
		}{Key: requestedId, Value: value})
		break
	case "timenow":
		t := time.Now()
		loc, _ := time.LoadLocation(settings.Get("timezone"))
		t = t.In(loc)
		value := t.Format(time.UnixDate)

		ctx.JSON(struct {
			Key   string
			Value string
		}{Key: requestedId, Value: value})
		break
	}
}

func GSApiSettingsPOST(ctx iris.Context, settings *gatesentryWebserverTypes.SettingsStore) {
	requestedId := ctx.Params().Get("id")
	_ = requestedId

	var temp gatesentryWebserverTypes.Datareceiver
	err := ctx.ReadJSON(&temp)
	_ = err
	if err != nil {
		return
	}
	switch requestedId {
	case "idemail":
		err := checkmail.ValidateFormat(temp.Value)
		if err != nil {
			temp.Value = "ERROR: Unable to Validate your email"
			// fmt.Printf("Code: %s, Msg: %s", smtpErr.Code(), smtpErr)
			// fmt.Fprint(w, "Unable to validate your email address.");
			ctx.JSON(temp)
			return
		}

	}

	if requestedId == "general_settings" {
		log.Println("Updating general settings")
		general_settings_parsed := gatesentryWebserverTypes.GSGeneral_Settings{}
		json.Unmarshal([]byte(temp.Value), &general_settings_parsed)
		pwd := general_settings_parsed.AdminPassword
		if pwd != "" {
			settings.Set(requestedId, temp.Value)
		} else {
			general_settings_parsed.AdminPassword = settings.GetAdminPassword()
			// convert general_settings_parsed to json
			valueJson, err := json.Marshal(general_settings_parsed)
			if err != nil {
				log.Fatal("Unable to marshal general settings")
			} else {
				//convert valuejSON to string
				settings.Set(requestedId, string(valueJson))
			}
		}
	}

	settings.InitGatesentry()
	// fmt.Println( temp );
	ctx.JSON(temp)
}
