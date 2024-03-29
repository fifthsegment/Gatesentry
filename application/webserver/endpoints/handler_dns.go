package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

func GSApiDNSInfo(dnsServerInfo *gatesentryTypes.DnsServerInfo) interface{} {

	return dnsServerInfo
}

func GSApiDNSEntriesCustom(data string, settings *gatesentry2storage.MapStore, runtime *gatesentryWebserverTypes.TemporaryRuntime) interface{} {

	// parse json string to struct
	var customEntries []gatesentryTypes.DNSCustomEntry
	json.Unmarshal([]byte(data), &customEntries)

	return struct {
		Data []gatesentryTypes.DNSCustomEntry `json:"data"`
	}{Data: customEntries}
}

func GSApiDNSSaveEntriesCustom(customEntries []gatesentryTypes.DNSCustomEntry, settings *gatesentry2storage.MapStore, runtime *gatesentryWebserverTypes.TemporaryRuntime) interface{} {
	// read json data from request body

	// check if no two entries have same domain
	customEntriesMap := make(map[string]bool)
	for _, entry := range customEntries {
		if _, ok := customEntriesMap[entry.Domain]; ok {
			//create error
			// BadResponse(ctx, errors.New("Two entries can't have the same domain"))
			return struct {
				Error string `json:"error"`
			}{Error: "Two entries can't have the same domain"}
		}
		customEntriesMap[entry.Domain] = true
	}

	// convert struct to json string
	jsonData, err := json.Marshal(customEntries)
	if err != nil {
		// BadResponse(ctx, err)
		return struct {
			Error string `json:"message"`
		}{Error: err.Error()}
	}

	// save json string to settings
	log.Println("[DNS] Saving custom entries = ", string(jsonData))
	settings.Update("DNS_custom_entries", string(jsonData))

	// ctx.JSON(struct {
	// 	Ok bool `json:"ok"`
	// }{Ok: true})
	return struct {
		Ok bool `json:"ok"`
	}{Ok: true}

}

func Error(s string) {
	panic("unimplemented")
}
