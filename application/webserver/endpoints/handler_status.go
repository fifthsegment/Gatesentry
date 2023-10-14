package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

type StatusResponse struct {
	ServerUrl string `json:"server_url"`
}

func ApiGetStatus(logger *gatesentryLogger.Log, boundAddress *string) interface{} {
	// get current server ip

	response := StatusResponse{
		ServerUrl: *boundAddress,
	}

	return response
}
