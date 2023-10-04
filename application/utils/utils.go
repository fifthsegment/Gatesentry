package gatesentry2utils;
import (
	// "bytes"
	"encoding/base64"
	// "io/ioutil"
	"net/http"
	"strings"
	"strconv"
	"math/rand"
	"time"
	// "github.com/elazarl/goproxy"
)

var proxyAuthorizationHeader = "Proxy-Authorization"

func GetUserFromAuthHeader(req *http.Request)(string, string){
	authheader := strings.SplitN(req.Header.Get(proxyAuthorizationHeader), " ", 2)
	// req.Header.Del(proxyAuthorizationHeader)
	if len(authheader) != 2 || authheader[0] != "Basic" {
		return "",""
	}
	userpassraw, err := base64.StdEncoding.DecodeString(authheader[1])
	if err != nil {
		return "",""
	}
	userpass := strings.SplitN(string(userpassraw), ":", 2)
	if len(userpass) != 2 {
		return "",""
	}
	return userpass[0], userpass[1]
}

func RemoveAuthorizationHeader(req *http.Request){
	req.Header.Del(proxyAuthorizationHeader);
}

func Int64toString(u int64) string {
	return strconv.FormatInt(u, 10)
}

func RandomString(n int ) string{
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
	    letterIdxBits = 6                    // 6 bits to represent a letter index
	    letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	    letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	)

	var src = rand.NewSource(time.Now().UnixNano())
    b := make([]byte, n)
    // A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
    for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
        if remain == 0 {
            cache, remain = src.Int63(), letterIdxMax
        }
        if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
            b[i] = letterBytes[idx]
            i--
        }
        cache >>= letterIdxBits
        remain--
    }

    return string(b)
}