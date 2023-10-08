package gatesentryproxy

import "net/http"

func HandleWebsocketConnection(r *http.Request, w http.ResponseWriter) {
	http.Error(w, "Web sockets currently not supported", http.StatusBadRequest)
	return
}
