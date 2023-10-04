package gatesentryf

import (
	"log"
	"runtime"

	gscommonweb "bitbucket.org/abdullah_irfan/gatesentryf/commonweb"
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
	return gscommonweb.GetFileHash(binpath)
	// f, err := os.Open(binpath)
	// if err != nil {
	// 	return err;
	// }
	// defer f.Close();

	// h:= sha256.New();
	// if _, err := io.Copy(h,f); err != nil {
	// 	return err;
	// }
	// // encoded := base64.StdEncoding.EncodeToString([]byte( h.Sum(nil) ))
	// encoded := hex.EncodeToString(h.Sum(nil))
}

func ValidateUpdateHashFromServer(hash string) bool {
	log.Println("Running hash Validator")

	return false
}

// import (
// 	"fmt"
// 	"log"
// 	"net/http"
// 	"time"
// 	"github.com/jpillora/overseer"
// 	"github.com/jpillora/overseer/fetcher"
// 	"os"
// 	"crypto/sha256"
// 	"io"
// 	"encoding/hex"
// 	"errors"
// )

// func validateHashFromServer(hash string) bool {
// 	fmt.Printf("Validating hash = " + hash );
// 	return false;
// }

// //create another main() to run the overseer process
// //and then convert your old main() into a 'prog(state)'
// func preupgradeCheck(binpath string) error {
// 	fmt.Println("Pre upgrade check = "+binpath)
// 	f, err := os.Open(binpath)
// 	if err != nil {
// 		return err;
// 	}
// 	defer f.Close();

// 	h:= sha256.New();
// 	if _, err := io.Copy(h,f); err != nil {
// 		return err;
// 	}
// 	// encoded := base64.StdEncoding.EncodeToString([]byte( h.Sum(nil) ))
// 	encoded := hex.EncodeToString(h.Sum(nil))
// 	// fmt.Println(encoded)
// 	if ( !validateHashFromServer(encoded) ){
// 		return errors.New("Unable to validate hash from server")
// 	}
// 	// fmt.Printf( "% x", h.Sum(nil) )
// 	return nil;
// }

// func main() {
// 	overseer.Run(overseer.Config{
// 		Program: prog,
// 		Address: ":3000",
// 		Fetcher: &fetcher.HTTP{
// 			URL:      "http://localhost:1000/updater.bin",
// 			Interval: 3600 * time.Second,
// 		},
// 		PreUpgrade:preupgradeCheck,
// 	})
// }

// //prog(state) runs in a child process
// func prog(state overseer.State) {
// 	log.Printf("app (%s) listening...", state.ID)
// 	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		fmt.Fprintf(w, "app (%s) says hello\n", state.ID)
// 	}))
// 	http.Serve(state.Listener, nil)
// }
