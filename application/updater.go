package gatesentryf

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"runtime"
	// "strconv"
)

func GetUpdateBinaryURL(basepoint string) string {

	GSVerString := GetApplicationVersion()
	url := basepoint
	url += "/"
	url += "gs-" + runtime.GOOS
	url += "-" + runtime.GOARCH
	url += "&myver=" + GSVerString
	url += "&iid=" + INSTALLATIONID
	_ = GSVerString
	// url += "&myver=" + GSVerString
	// url += "&iid=" + INSTALLATIONID
	log.Println("Update Binary URL = " + url)
	return url
}

func GetUpdateBinaryURLOld(basepoint string) string {

	GSVerString := GetApplicationVersion()
	url := basepoint
	url += "/updates/bin"
	url += "?goos=" + runtime.GOOS
	url += "&goarch=" + runtime.GOARCH
	url += "&myver=" + GSVerString
	url += "&iid=" + INSTALLATIONID
	log.Println("Update Binary URL = " + url)
	return url
}

func GetFileHash(binpath string) string {
	f, err := os.Open(binpath)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	// encoded := base64.StdEncoding.EncodeToString([]byte( h.Sum(nil) ))
	encoded := hex.EncodeToString(h.Sum(nil))

	return encoded
}

func ValidateUpdateHashFromServer(hash string) bool {
	log.Println("Running hash Validator")

	return false
}
