package gatesentryproxy

const FILTER_TIME = "timeallowed"
const FILTER_USER_ACCESS_DISABLED = "blockinternet"
const FILTER_ACCESS_URL = "url"
const PROXY_ACTION_SSL_DIRECT = "ssldirect"
const FILTER_FILE_TYPE = "contenttypeblocked"

type ProxyAction string

const (
	ProxyActionBlockedTextContent     ProxyAction = "blocked_text_content"
	ProxyActionBlockedMediaContent    ProxyAction = "blocked_media_content"
	ProxyActionBlockedFileType        ProxyAction = "blocked_file_type"
	ProxyActionBlockedTime            ProxyAction = "blocked_time"
	ProxyActionBlockedInternetForUser ProxyAction = "blocked_internet_for_user"
	ProxyActionBlockedUrl             ProxyAction = "blocked_url"
	ProxyActionSSLDirect              ProxyAction = "ssldirect"
	ProxyActionSSLBump                ProxyAction = "ssl-bump"
	ProxyActionFilterError            ProxyAction = "filtererror"
	ProxyActionFilterNone             ProxyAction = "filternone"
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
