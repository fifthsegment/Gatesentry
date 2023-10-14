package gatesentryWebserver

import (
	"encoding/json"
	"log"
	"net/http"

	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	"github.com/gorilla/mux"
)

var getAllFilters = func(w http.ResponseWriter, r *http.Request, Filters *[]gatesentryFilters.GSFilter) {
	filterList := []gatesentryFilters.GSAPIStructFilter{}
	for _, v := range *Filters {
		filterList = append(filterList, gatesentryFilters.GSAPIStructFilter{
			Id:      v.Id,
			Name:    v.FilterName,
			Handles: v.Handles,
			Entries: v.FileContents,
		})
	}
	SendJSON(w, filterList)
}

var getSingleFilter = func(w http.ResponseWriter, r *http.Request, Filters *[]gatesentryFilters.GSFilter) {
	vars := mux.Vars(r)
	requestedId := vars["id"]
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
	SendJSON(w, filterList)
}

var postSingleFilter = func(w http.ResponseWriter, r *http.Request, Filters *[]gatesentryFilters.GSFilter) {
	vars := mux.Vars(r)
	requestedId := vars["id"]

	var dataReceived []gatesentryFilters.GSFILTERLINE
	ParseJSONRequest(r, &dataReceived)

	for _, v := range *Filters {
		if v.Id == requestedId {
			data, err := json.MarshalIndent(dataReceived, "", "  ")
			if err != nil {
				log.Println("Error marshalling data")
				SendError(w, err, http.StatusInternalServerError)
				return
			}
			log.Println("Data received = " + string(data))
			gatesentryFilters.GSSaveFilterFile(v.FileName, string(data))
		}
	}
	SendJSON(w, OkResponse{Response: "Ok!"})
}
