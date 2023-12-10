package gatesentryproxy

import (
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

const FILTER_TIME = "timeallowed"
const FILTER_USER_ACCESS_DISABLED = "blockinternet"
const FILTER_ACCESS_URL = "url"
const PROXY_ACTION_SSL_DIRECT = "ssldirect"
const FILTER_FILE_TYPE = "contenttypeblocked"

const (
	ProxyActionBlockedTextContent     GatesentryTypes.ProxyAction = "blocked_text_content"
	ProxyActionBlockedMediaContent    GatesentryTypes.ProxyAction = "blocked_media_content"
	ProxyActionBlockedFileType        GatesentryTypes.ProxyAction = "blocked_file_type"
	ProxyActionBlockedTime            GatesentryTypes.ProxyAction = "blocked_time"
	ProxyActionBlockedRule            GatesentryTypes.ProxyAction = GatesentryTypes.ProxyActionBlocked
	ProxyActionBlockedInternetForUser GatesentryTypes.ProxyAction = "blocked_internet_for_user"
	ProxyActionUserNotFound           GatesentryTypes.ProxyAction = "user_not_found"
	ProxyActionUserActive             GatesentryTypes.ProxyAction = "user_active"
	ProxyActionBlockedUrl             GatesentryTypes.ProxyAction = "blocked_url"
	ProxyActionSSLDirect              GatesentryTypes.ProxyAction = "ssldirect"
	ProxyActionSSLBump                GatesentryTypes.ProxyAction = "ssl-bump"
	ProxyActionFilterError            GatesentryTypes.ProxyAction = "filtererror"
	ProxyActionFilterNone             GatesentryTypes.ProxyAction = "filternone"
)

var EMPTY_BYTES = []byte("")
var BLOCKED_URL_BYTES = []byte("Blocked URL")
var BLOCKED_INTERNET_BYTES = []byte("Internet access has been blocked by your administrator")
var BLOCKED_ERROR_HIJACK_BYTES = []byte("[SSL Bump] - Error hijacking request")
var PROXY_ERROR_UNABLE_TO_READ_DATA = []byte("Error: Unable to read data")
var PROXY_ERROR_UNABLE_TO_MARSHALL_DATA_FOR_SCANNING = []byte("Error: Unable to marshall data for scanning")
var PROXY_ERROR_UNABLE_TO_COPY_DATA = []byte("Error: Unable to copy data")
var BLOCKED_CONTENT_TYPE = []byte("This content type is blocked by your administrator")
var BLOCKED_CONTENT_TEXT = []byte("This content is blocked by your administrator")

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
