package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"

	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
)

var GetAllFilters = func(Filters *[]gatesentryFilters.GSFilter) interface{} {
	filterList := []gatesentryFilters.GSAPIStructFilter{}
	for _, v := range *Filters {
		filterList = append(filterList, gatesentryFilters.GSAPIStructFilter{
			Id:      v.Id,
			Name:    v.FilterName,
			Handles: v.Handles,
			Entries: v.FileContents,
		})
	}
	return filterList
}

var GetSingleFilter = func(requestedId string, Filters *[]gatesentryFilters.GSFilter) interface{} {
	filterList := []gatesentryFilters.GSAPIStructFilter{}
	for _, v := range *Filters {
		if v.Id == requestedId {
			filterList = append(filterList, gatesentryFilters.GSAPIStructFilter{
				Id:      v.Id,
				Name:    v.FilterName,
				Handles: v.Handles,
				Entries: v.FileContents,
			})
		}
	}
	return filterList
}

var PostSingleFilter = func(requestedId string, dataReceived []gatesentryFilters.GSFILTERLINE, Filters *[]gatesentryFilters.GSFilter) interface{} {
	for _, v := range *Filters {
		if v.Id == requestedId {
			data, err := json.MarshalIndent(dataReceived, "", "  ")
			if err != nil {
				return struct {
					Response string `json:"response"`
					Error    string `json:"error"`
				}{Error: err.Error(), Response: "Error!"}
			}
			log.Println("Data received = " + string(data))
			gatesentryFilters.GSSaveFilterFile(v.FileName, string(data))
		}
	}
	return struct {
		Response string `json:"response"`
	}{"Ok!"}
}
