package gatesentryproxy

var EMPTY_BYTES = []byte("")
var BLOCKED_URL_BYTES = []byte("Blocked URL")
var BLOCKED_INTERNET_BYTES = []byte("Internet access has been blocked by your administrator")
var BLOCKED_ERROR_HIJACK_BYTES = []byte("[SSL Bump] - Error hijacking request")

var ACTION_BLOCK_REQUEST = "block"
var ACTION_SSL_BUMP = "ssl-bump"
var ACTION_NONE = ""
