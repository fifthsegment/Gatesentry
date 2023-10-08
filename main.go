package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"image/jpeg"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"errors"
	"fmt"
	"io/ioutil"

	application "bitbucket.org/abdullah_irfan/gatesentryf"
	filters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gresponder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	"bitbucket.org/abdullah_irfan/gatesentryproxy"
	"github.com/jpillora/overseer"
	"github.com/kardianos/service"
	"github.com/steakknife/devnull"
	"golang.org/x/image/webp"
)

var GSPROXYPORT = "10413"
var GSBASEDIR = ""
var Baseendpointv2 = "https://www.applicationilter.com/api/"
var GATESENTRY_VERSION = "1.9"

var contentTypeToExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/avif": ".avif",
	"":           "",
}

type InferenceDetectionCategory struct {
	Class string  `json:"class"`
	Score float64 `json:"score"`
}

type InferenceResponse struct {
	Category   string                       `json:"category"`
	Confidence int                          `json:"confidence"`
	Detections []InferenceDetectionCategory `json:"detections"`
}

func ConvertWebPToJPEG(webpData []byte) ([]byte, error) {
	// Decode webp bytes to image.Image
	img, err := webp.Decode(bytes.NewReader(webpData))
	if err != nil {
		return nil, err
	}

	// Encode image.Image to jpeg
	var jpegBuf bytes.Buffer
	err = jpeg.Encode(&jpegBuf, img, nil)
	if err != nil {
		return nil, err
	}

	return jpegBuf.Bytes(), nil
}

type ContentScannerInput struct {
	Content     []byte
	ContentType string
	Url         string
}

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	log.Println("Starting up GateSentry")
	p.exit = make(chan struct{})
	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() error {
	RunGateSentry()
	for {
		select {

		case <-p.exit:
			log.Println("Stopping GateSentry")
			application.Stop()
			return nil
		}
	}
}
func (p *program) Stop(s service.Service) error {
	close(p.exit)
	return nil
}

func preupgradeCheck(binpath string) error {
	fmt.Println("Pre upgrade check = " + binpath)
	encoded := application.GetFileHash(binpath)

	if !application.ValidateUpdateHashFromServer(encoded) {
		return errors.New("Unable to validate hash from server")
	}

	return nil
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}

	GSBASEDIR = dir
	application.SetBaseDir(dir + "/")
	_ = devnull.Writer
	// log.SetOutput(devnull.Writer)
	overseer.SanityCheck()
	baseendpoint := "http://gatesentryreflector.abdullahirfan.com/api"
	baseendpointforupdates := "http://gatesentryupdates.abdullahirfan.com/api"

	_ = baseendpointforupdates

	updaterInterval := time.Second * 60 * 30
	version, err := strconv.ParseFloat(GATESENTRY_VERSION, 32)
	if err != nil {
		log.Fatal(err)
	}
	application.SetGSVer(float32(version))
	application.SetAPIBaseEndpoint(baseendpoint)
	// macaddr:=getMac();
	// APPLicense := BuildInstallationIDFromMac(macaddr)
	APPLicense := "NONEXISTENT"
	application.SetInstallationID(APPLicense)

	serviceflag := ""

	flag.StringVar(&serviceflag, "service", "", "")
	flag.Parse()
	// serviceflag = "stop"
	// fmt.Println( len(serviceflag) )
	// return;
	if serviceflag == "start" || serviceflag == "stop" || serviceflag == "uninstall" || serviceflag == "restart" || serviceflag == "install" {
		RunGateSentryServiceRunner(serviceflag)
	} else {
		url := application.GetUpdateBinaryURLOld(baseendpointforupdates)
		_ = updaterInterval
		_ = url
		RunGateSentryServiceRunner("")

	}

}

func prog(state overseer.State) {
	RunGateSentryServiceRunner("")
}

// Service setup.
//
//	Define service config.
//	Create the service.
//	Setup the logger.
//	Handle service controls (optional).
//	Run the service.
func RunGateSentryServiceRunner(svcFlag string) {

	svcConfig := &service.Config{
		Name:        "GateSentry",
		DisplayName: "A web filtering proxy",
		Description: "A web filtering proxy",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	// logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(svcFlag) != 0 {
		err := service.Control(s, svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		// logger.Error(err)
	}
}

func BuildInstallationIDFromMac(mac string) string {
	a := strings.ToUpper(strings.Replace(mac, ":", "", -1))
	lic := "GA" + a + "TE" + "SE" + a + "NT"
	return lic
}

func saveToDisk(data []byte, fileExt string) {
	// Ensure the 'temp' directory exists
	if _, err := os.Stat("temp"); os.IsNotExist(err) {
		os.Mkdir("temp", 0755)
	}

	// Generate a random string for the filename
	rand.Seed(time.Now().UnixNano())
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomString := make([]byte, 10) // for example, 10 characters long
	for i := range randomString {
		randomString[i] = charset[rand.Intn(len(charset))]
	}

	// Create the file in the 'temp' directory with the random filename
	dst, err := os.Create(fmt.Sprintf("temp/%s%s", randomString, fileExt))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer dst.Close()
}

func RunGateSentry() {

	R := application.Start()
	application.StartBonjour()
	gatesentryproxy.InitProxy()
	ngp := gatesentryproxy.NewGSProxy()

	// Making a comm channel for our internal dns server
	go application.DNSServerThread(application.GetBaseDir(), R.Logger, R.DNSServerChannel, R.GSSettings)

	addr := "0.0.0.0:"
	addr += GSPROXYPORT

	ttt := time.NewTicker(time.Second * 10)
	portavailable := false
	for {
		fmt.Println("Listening for proxy connections on : " + GSPROXYPORT)
		ln, err := net.Listen("tcp", ":"+GSPROXYPORT)
		if err != nil {
			fmt.Println("Port is not open for proxy")
		} else {
			portavailable = true
			err = ln.Close()
		}

		if portavailable {
			break
		}
		<-ttt.C
	}
	// if portavailable {}

	capembytes := []byte(R.GSSettings.Get("capem"))
	keypembytes := []byte(R.GSSettings.Get("keypem"))

	gatesentryproxy.InitWithDataCerts(capembytes, keypembytes)
	proxyListener, err := net.Listen("tcp", addr)
	proxyHandler := gatesentryproxy.ProxyHandler{Iproxy: ngp}

	CannedResponsesAuthError := []byte(gresponder.BuildGeneralResponsePage([]string{"Your access has been disabled."}, -1))
	CannedResponseAccessNotActiveError := []byte(gresponder.BuildGeneralResponsePage([]string{"Your access has been disabled by the administrator of this network."}, -1))

	// CONTENT FILTER
	ngp.RegisterHandler("proxyerror", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		clienterror := string(*bytesReceived)
		msg := "Proxy Error. Unable to fulfill your request. <br/><strong>" + clienterror + "</strong>."
		switch clienterror {
		case "EOF":
			msg = "Proxy Error. Unable to fulfill your request at this time. Please try again in a few seconds."
			break
		default:
			break
		}
		*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{msg}, -1))
	})

	// Should the Proxy MITM this traffic or not
	ngp.RegisterHandler("mitm", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		log.Println("Running MITM handler")
		// log.Println("GPT = ", gpt)
		host := string(*bytesReceived)
		enable_filtering := R.GSSettings.Get("enable_https_filtering")
		log.Println("MITM Handler - enable_https_filtering = " + enable_filtering)
		if enable_filtering != "true" {
			gpt.DontTouch = true
			rs.Changed = true
			return
		}
		// else {
		// 	rs.Changed = false
		// 	gpt.DontTouch = true
		// }
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/https_dontbump", host, responder)
		if responder.Blocked {
			gpt.DontTouch = true
			rs.Changed = true
			return
		}
	})

	ngp.RegisterHandler("except_urls", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		host := string(*bytesReceived)
		log.Println("Running exception handler for = ", host)
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/all_exception_urls", host, responder)
		if responder.Blocked {
			gpt.DontTouch = true
			log.Println("URL found in exception = ", host)
			// *s = []byte(gresponder.BuildGeneralResponsePage( []string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1));
			rs.Changed = true
		}
	})
	// CONTENT FILTER
	ngp.RegisterHandler("content", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		log.Println("Running content handler")
		// log.Println("GPT = ", gpt)
		responder := &gresponder.GSFilterResponder{Blocked: false}
		if !gpt.DontTouch {
			application.RunFilter("text/html", string(*bytesReceived), responder)
			if responder.Blocked {
				*bytesReceived = []byte(gresponder.BuildResponsePage(responder.Reasons, responder.Score))
			}
			rs.Changed = responder.Blocked
		}

	})

	// URL CHECKER
	ngp.RegisterHandler("url", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		host := string(*bytesReceived)
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/all_blocked_urls", host, responder)
		if responder.Blocked {
			*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1))
			rs.Changed = true
		}
	})

	ngp.RegisterHandler("contentlength", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		length := string(*bytesReceived)
		go func() {
			i, err := strconv.ParseUint(length, 10, 64)
			if err == nil {
				R.UpdateUserData(gpt.User, i)
			}
			consumedBytes, err := strconv.ParseInt(length, 10, 64)
			if err == nil {
				R.UpdateConsumption(consumedBytes)
			}
		}()
	})

	ngp.RegisterHandler("blockinternet", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		if gpt.User == "admin" {
			rs.Changed = false
		}
	})

	ngp.RegisterHandler("isauthuser", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		base64string := string(*bytesReceived)
		rs.Changed = false
		if R.IsUserValid(base64string) {
			rs.Changed = true
		}
	})

	ngp.RegisterHandler("isaccessactive", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		*bytesReceived = CannedResponseAccessNotActiveError
		rs.Changed = false
		if R.IsUserActive(gpt.User) {
			rs.Changed = true
		}
	})

	ngp.RegisterHandler("authenabled", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		*bytesReceived = CannedResponsesAuthError
		temp := R.GSSettings.Get("EnableUsers")
		enableusers := false
		if temp == "true" {
			enableusers = true
		}
		rs.Changed = enableusers

	})

	ngp.RegisterHandler("log", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		url := string(*s)
		user := gpt.User
		R.Logger.Log(url, user)
	})

	ngp.RegisterHandler("timeallowed", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		// url := string(*s)
		rs.Changed = false
		blockedtimes := R.GSSettings.Get("blocktimes")
		responder := &gresponder.GSFilterResponder{Blocked: false}
		timezone := R.GSSettings.Get("timezone")
		filters.RunTimeFilter(responder, blockedtimes, timezone)
		// user := gpt.User
		if responder.Blocked {
			rs.Changed = true
			*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{"Internet access on this network has been disabled because the current time has been specified as a blocked time period in GateSentry's settings."}, -1))
		}
	})

	ngp.RegisterHandler("prerequest", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		rs.Changed = false
	})

	ngp.RegisterHandler("youtube", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		url1 := "mainvideo url"
		// Extract video ID
		parts := strings.Split(url1, "/")
		videoID := parts[4]

		// Extract sqp parameter
		sqpIndex := strings.Index(url1, "sqp=")
		sqpEndIndex := strings.Index(url1[sqpIndex:], "|48")
		if sqpEndIndex == -1 {
			sqpEndIndex = len(url1) - sqpIndex
		} else {
			sqpEndIndex += sqpIndex
		}
		sqp := url1[sqpIndex:sqpEndIndex]

		// Extract sigh parameter
		sighIndex := strings.LastIndex(url1, "rs$")
		sigh := url1[sighIndex:]

		// Construct the new URL
		url2 := fmt.Sprintf("https://i.ytimg.com/sb/%s/storyboard3_L2/M2.jpg?%s&sigh=%s", videoID, sqp, sigh)
		fmt.Println(url2)
	})

	ngp.RegisterHandler("contentscanner", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		rs.Changed = false
		log.Println("Running content scanner")

		// convert bytes to json struct of type ContentScannerInput
		var contentScannerInput ContentScannerInput
		err := json.Unmarshal(*bytesReceived, &contentScannerInput)
		if err != nil {
			log.Println("Error unmarshalling content scanner input")
		}
		log.Println("Running content scanner for content type = " + contentScannerInput.ContentType)

		if len(contentScannerInput.Content) < 6000 {
			// continue
		} else if (contentScannerInput.ContentType == "image/jpeg") || (contentScannerInput.ContentType == "image/jpg") || (contentScannerInput.ContentType == "image/png") || (contentScannerInput.ContentType == "image/gif") || (contentScannerInput.ContentType == "image/webp") || (contentScannerInput.ContentType == "image/avif") {
			contentType := contentScannerInput.ContentType
			log.Println("Running content scanner for image")

			// if contentType == "image/jpg" || contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/gif" || contentType == "image/webp" {
			var b bytes.Buffer
			wr := multipart.NewWriter(&b)
			// part, _ := wr.CreateFormFile("image", "uploaded_image"+contentTypeToExt[contentType])

			if contentType == "image/webp" {
				// convert webp to jpeg
				jpegBytes, err := ConvertWebPToJPEG(contentScannerInput.Content)
				if err != nil {
					fmt.Println("Error converting webp to jpeg")
				}
				contentScannerInput.Content = jpegBytes
				contentType = "image/jpeg"
			}

			// Create a new form header for the file
			// h := make(textproto.MIMEHeader)
			h := make(textproto.MIMEHeader)
			// ext := contentTypeToExt[contentType]
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, "uploaded_image"))
			// h.Set("Content-Type", contentType)

			// Create the form file with the custom header
			part, _ := wr.CreatePart(h)

			part.Write(*&contentScannerInput.Content)

			// save the file to disk
			// add randomness to file name and save it under folder called thumbs
			// generate random string
			b.Bytes()
			// saveToDisk(b.Bytes(), contentTypeToExt[contentType])
			wr.Close()

			// Log the request headers and body
			// fmt.Println("Request Headers:")
			// fmt.Println("Content-Type:", wr.FormDataContentType())
			// fmt.Println("\nRequest Body:")
			// fmt.Println(b.String())

			resp, _ := http.Post("http://10.1.0.115:8000/infer/onnx", wr.FormDataContentType(), &b)
			if resp.StatusCode == http.StatusOK {
				bytesLength := len(*bytesReceived)
				// convert bytes length to string
				//
				bytesLengthString := strconv.Itoa(bytesLength)
				fmt.Println("Inference for " + contentScannerInput.Url + " Content type = " + contentType + "Length = " + bytesLengthString)
				respBytes, _ := io.ReadAll(resp.Body)
				responseString := string(respBytes)
				var inferenceResponse InferenceResponse
				err := json.Unmarshal([]byte(respBytes), &inferenceResponse)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				fmt.Println("Inference Response = " + responseString)
				if inferenceResponse.Category == "sexy" && inferenceResponse.Confidence > 85 {
					rs.Changed = true
				}
				if inferenceResponse.Category == "porn" && inferenceResponse.Confidence > 85 {
					rs.Changed = true
				}
				if len(inferenceResponse.Detections) > 0 {
					// rs.Changed = true
					var conditionsMet = 0

					for _, detection := range inferenceResponse.Detections {

						if detection.Class == "FEMALE_GENITALIA_EXPOSED" && detection.Score > 0.4 {
							conditionsMet += 2
						}
						if detection.Class == "FEMALE_BREAST_EXPOSED" && detection.Score > 0.4 {
							conditionsMet += 2
						}
						if detection.Class == "FEMALE_BREAST_COVERED" && detection.Score > 0.4 {
							conditionsMet += 2
						}
						if detection.Class == "BELLY_COVERED" && detection.Score > 0.5 {
							conditionsMet += 2
						}
						if detection.Class == "ARMPITS_EXPOSED" && detection.Score > 0.5 {
							conditionsMet++
						}
						if detection.Class == "MALE_GENITALIA_EXPOSED" && detection.Score > 0.5 {
							conditionsMet += 2
						}
						if detection.Class == "MALE_BREAST_EXPOSED" && detection.Score > 0.5 {
							conditionsMet++
						}

						if detection.Class == "BUTTOCKS_EXPOSED" && detection.Score > 0.5 {
							conditionsMet += 2
						}

						if detection.Class == "ANUS_EXPOSED" && detection.Score > 0.5 {
							conditionsMet += 2
						}

						if detection.Class == "BELLY_EXPOSED" && detection.Score > 0.5 {
							conditionsMet++

						}

					}
					if conditionsMet >= 2 {
						rs.Changed = true
					}

				}

			} else {
				fmt.Println("Inference for Content type = " + contentType + " failed")
				respBytes, _ := io.ReadAll(resp.Body)

				fmt.Println("Inference Response = " + string(respBytes))
			}
			defer resp.Body.Close()
			// }
		}

	})

	ngp.RegisterHandler("contenttypeblocked", func(bytesReceived *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		contentType := string(*bytesReceived)
		responder := &gresponder.GSFilterResponder{Blocked: false}
		application.RunFilter("url/all_blocked_mimes", contentType, responder)
		// dictionary of contentType to file extension

		if responder.Blocked {
			rs.Changed = true
			message := "This content type has been blocked on this network."
			if contentType == "image/png" || contentType == "image/jpeg" || contentType == "image/jpg" || "image/gif" == contentType || "image/webp" == contentType {
				dat, _ := filters.Asset("app/transparent.png")
				*bytesReceived = dat
			} else {
				*bytesReceived = []byte(gresponder.BuildGeneralResponsePage([]string{message}, -1))
			}
		}
	})

	server := http.Server{Handler: proxyHandler}
	log.Printf("Starting up...Listening on = " + addr)
	err = server.Serve(tcpKeepAliveListener{proxyListener.(*net.TCPListener)})
	log.Fatal(err)

}

func orPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func macNotIn(check string) bool {
	check = strings.ToLower(check)
	if strings.Contains(check, "vmware") || strings.Contains(check, "docker") || strings.Contains(check, "virtualbox") || strings.Contains(check, "tredo tunneling") || strings.Contains(check, "microsoft") {
		return false
	}
	return true
}

func VerifyKey(key string) {
	key = strings.TrimSuffix(key, "\n")
	fmt.Println("Verifying key = " + key)
	url := Baseendpointv2 + "/verify/key"
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-KEY", key)
	// res, _ := client.Do(req)
	if res, err := client.Do(req); err != nil {
		// return nil, err
		fmt.Println(err)
		os.Exit(1)
	} else {
		defer res.Body.Close()
		// return readBody(resp.Body)
		if res.StatusCode == http.StatusOK {
			bodyBytes, err2 := ioutil.ReadAll(res.Body)
			if err2 != nil {
				fmt.Println("Unable to verify API key")
				os.Exit(1)
			}
			bodyString := string(bodyBytes)
			fmt.Println(bodyString)
		} else {
			fmt.Println("Error: Incorrect API key")
		}
	}
	// defer res.Body.Close()

}

func getMac() string {
	fmt.Println("Checking if API key exists")

	addrs, err := net.InterfaceAddrs()

	if err != nil {
		fmt.Println(err)
	}

	var currentIP, currentNetworkHardwareName string
	var goodIPs []string

	for _, address := range addrs {

		// check the address type and if it is not a loopback the display it
		// = GET LOCAL IP ADDRESS
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				// fmt.Println("Current IP address : ", ipnet.IP.String())
				currentIP = ipnet.IP.String()
				goodIPs = append(goodIPs, currentIP)
			}
		}
	}

	// get all the system's or local machine's network interfaces

	interfaces, _ := net.Interfaces()
	for _, interf := range interfaces {
		// fmt.Println(interf)
		if addrs, err := interf.Addrs(); err == nil {
			for _, addr := range addrs {
				// fmt.Println("[", index, "]", interf.Name, ">", addr)
				if macNotIn(interf.Name) {
					for i := 0; i < len(goodIPs); i++ {
						// only interested in the name with current IP address
						if strings.Contains(addr.String(), goodIPs[i]) {
							// fmt.Println("Use name : ", interf.Name)
							currentNetworkHardwareName = interf.Name
						}
					}

				}
				// currentNetworkHardwareName  = "";
			}
		}
	}

	// fmt.Println("------------------------------")

	// extract the hardware information base on the interface name
	// capture above
	netInterface, err := net.InterfaceByName(currentNetworkHardwareName)

	if err != nil {
		fmt.Println("Error: Unable to get device address, are you connected to the internet?")
		os.Exit(1)
	}

	_ = netInterface.Name
	macAddress := netInterface.HardwareAddr

	// fmt.Println("Hardware name : ", name)
	// fmt.Println("MAC address : ", macAddress)

	// verify if the MAC address can be parsed properly
	hwAddr, err := net.ParseMAC(macAddress.String())

	if err != nil {
		fmt.Println("Not able to parse MAC address : ", err)
		os.Exit(-1)
	}

	return hwAddr.String()
	// fmt.Printf("Physical hardware address : %s \n", hwAddr.String())
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away. (Copied from net/http package)
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
