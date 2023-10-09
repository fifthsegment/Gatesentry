package gatesentryproxy

var EMPTY_BYTES = []byte("")
var BLOCKED_URL_BYTES = []byte("Blocked URL")
var BLOCKED_INTERNET_BYTES = []byte("Internet access has been blocked by your administrator")
var BLOCKED_ERROR_HIJACK_BYTES = []byte("[SSL Bump] - Error hijacking request")
var PROXY_ERROR_UNABLE_TO_READ_DATA = []byte("Error: Unable to read data")
var PROXY_ERROR_UNABLE_TO_MARSHALL_DATA_FOR_SCANNING = []byte("Error: Unable to marshall data for scanning")
var PROXY_ERROR_UNABLE_TO_COPY_DATA = []byte("Error: Unable to copy data")
var BLOCKED_CONTENT_TYPE = []byte("This content type is blocked by your administrator")

var ACTION_BLOCK_REQUEST = "block"
var ACTION_SSL_BUMP = "ssl-bump"
var ACTION_NONE = ""

var HOP_BY_HOP = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}
