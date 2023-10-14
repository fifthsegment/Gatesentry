package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

func ApiToggleServer(id string, logger *gatesentryLogger.Log) interface{} {

	switch id {
	case "dns":
		// ctx.StatusCode(iris.StatusOK)
		// ctx.JSON(iris.Map{"success": true})
		return struct {
			Success bool `json:"success"`
		}{Success: true}
	case "http":
		// ctx.StatusCode(iris.StatusOK)
		// ctx.JSON(iris.Map{"success": true})
		return struct {
			Success bool `json:"success"`
		}{Success: true}
	default:
		// ctx.StatusCode(iris.StatusBadRequest)
		// ctx.JSON(iris.Map{"error": "Invalid id"})
		// return
		return struct {
			Error string `json:"error"`
		}{Error: "Invalid id"}

	}

}
