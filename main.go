package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf"
	gatesentry2filters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
	"bitbucket.org/abdullah_irfan/gatesentryproxy"
	"github.com/jpillora/overseer"
	"github.com/kardianos/service"
	"github.com/steakknife/devnull"

	"errors"
	"fmt"
	"io/ioutil"
)

var GSPROXYPORT = "10413"
var GSBASEDIR = ""
var Baseendpointv2 = "https://www.gatesentryfilter.com/api/"

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
	// logger.Infof("I'm running %v.", service.Platform())
	// ticker := time.NewTicker(2 * time.Second)
	RunGateSentry()
	for {
		select {
		// case tm := <-ticker.C:
		// logger.Infof("Still running at %v...", tm)
		case <-p.exit:
			log.Println("Stopping GateSentry")
			// ticker.Stop()
			gatesentryf.Stop()
			return nil
		}
	}
}
func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	// logger.Info("I'm Stopping!")
	close(p.exit)
	return nil
}

// create another main() to run the overseer process
// and then convert your old main() into a 'prog(state)'
func preupgradeCheck(binpath string) error {
	fmt.Println("Pre upgrade check = " + binpath)
	// fmt.Println(encoded)
	encoded := gatesentryf.GetFileHash(binpath)

	if !gatesentryf.ValidateUpdateHashFromServer(encoded) {
		return errors.New("Unable to validate hash from server")
	}
	// fmt.Printf( "% x", h.Sum(nil) )
	return nil
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Println("We are at : " + dir)
	// os.Chdir(dir);
	GSBASEDIR = dir
	gatesentryf.SetBaseDir(dir + "/")
	// InitTasks();
	_ = devnull.Writer
	// log.SetOutput(devnull.Writer)
	overseer.SanityCheck()
	baseendpoint := "http://gatesentryreflector.abdullahirfan.com/api"
	baseendpointforupdates := "http://gatesentryupdates.abdullahirfan.com/api"
	// Baseendpointv2 :=
	_ = baseendpointforupdates
	updaterInterval := time.Second * 60 * 30
	gatesentryf.SetGSVer(1.8)
	gatesentryf.SetAPIBaseEndpoint(baseendpoint)
	// macaddr:=getMac();
	// APPLicense := BuildInstallationIDFromMac(macaddr)
	APPLicense := "NONEXISTENT"
	gatesentryf.SetInstallationID(APPLicense)

	serviceflag := ""

	flag.StringVar(&serviceflag, "service", "", "")
	flag.Parse()
	// serviceflag = "stop"
	// fmt.Println( len(serviceflag) )
	// return;
	if serviceflag == "start" || serviceflag == "stop" || serviceflag == "uninstall" || serviceflag == "restart" || serviceflag == "install" {
		RunGateSentryServiceRunner(serviceflag)
	} else {
		url := gatesentryf.GetUpdateBinaryURLOld(baseendpointforupdates)
		_ = updaterInterval
		_ = url
		RunGateSentryServiceRunner("")
		// fmt.Println(url);
		/*overseer.Run(overseer.Config{
			Program: prog,
			Address: ":37453",
			Fetcher: &fetcher.HTTP{
				URL:     url,
				// URL: "http://localhost/updates/bin/updater.bin2",
				Interval: updaterInterval,
			},
			PreUpgrade:preupgradeCheck,
		})*/
	}

}

// prog(state) runs in a child process
func prog(state overseer.State) {
	RunGateSentryServiceRunner("")

	// log.Printf("app (%s) listening...", state.ID)
	// go func(){
	// 	return;
	// 	t := time.NewTicker( time.Second * 5 )
	// 	for {
	// 		fmt.Println("I'm a runner from the first class");
	// 	// }
	// 		<-t.C
	// 	}
	// }();
	// ();
}

// Service setup.
//
//	Define service config.
//	Create the service.
//	Setup the logger.
//	Handle service controls (optional).
//	Run the service.
func RunGateSentryServiceRunner(svcFlag string) {

	// var port string
	// svcFlag := flag.String("service", "", "Control the system service.")
	// flag.StringVar(&port, "port", "10413", "port to run on")
	// port = GSPROXYPORT
	// flag.Parse()

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

	// fmt.Println("Flag = " + svcFlag)
	// return;
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

func RunGateSentry() {

	// log.Println("Your installation id = "+ APPLicense)

	// Disable Logging
	// log.SetOutput(devnull.Writer)

	R := gatesentryf.Start()
	gatesentryf.StartBonjour()
	gatesentryproxy.InitProxy()
	ngp := gatesentryproxy.NewGSProxy()

	// fmt.Println("Making a comm channel for dns")
	go gatesentryf.DNSServerThread(gatesentryf.GetBaseDir(), R.Logger, R.DNSServerChannel, R.GSSettings)

	// go func() {
	// 	for {
	// 		select {
	// 		case msg := <-R.DNSServerChannel:
	// 			fmt.Println("[DEBUG] Received message:", msg)
	// 		}
	// 	}
	// 	fmt.Println("[DEBUG] Done receive message:")
	// }()

	// ffp := gatesentryalpha.NewFPProxy();
	// ffp.StartProxy();

	// flag.StringVar(&port, "port", "10413", "port to run on")
	// flag.Parse()
	addr := "0.0.0.0:"
	addr += GSPROXYPORT

	// ggport := strconv.Itoa( GSPROXYPORT )
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

	CannedResponsesAuthError := []byte(gatesentry2responder.BuildGeneralResponsePage([]string{"Your access has been disabled."}, -1))
	CannedResponseAccessNotActiveError := []byte(gatesentry2responder.BuildGeneralResponsePage([]string{"Your access has been disabled by the administrator of this network."}, -1))

	// CONTENT FILTER
	ngp.RegisterHandler("proxyerror", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		clienterror := string(*s)
		msg := "Proxy Error. Unable to fulfill your request. <br/><strong>" + clienterror + "</strong>."
		switch clienterror {
		case "EOF":
			msg = "Proxy Error. Unable to fulfill your request at this time. Please try again in a few seconds."
			break
		default:
			break
		}
		*s = []byte(gatesentry2responder.BuildGeneralResponsePage([]string{msg}, -1))
	})

	// Should the Proxy MITM this traffic or not
	ngp.RegisterHandler("mitm", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		log.Println("Running MITM handler")
		// log.Println("GPT = ", gpt)
		host := string(*s)
		enable_filtering := R.GSSettings.Get("enable_https_filtering")
		log.Println("MITM Handler - enable_https_filtering = " + enable_filtering)
		if enable_filtering != "true" {
			log.Println("MITM Handler - Filtering disabled")
			gpt.DontTouch = true
			rs.Changed = true
			return
		}
		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
		gatesentryf.RunFilter("url/https_dontbump", host, responder)
		if responder.Blocked {
			gpt.DontTouch = true
			rs.Changed = true
			return
		}
		// gatesentry2.RunFilter( "url/all_exception_urls", host, responder )
		// if ( responder.Blocked ){
		// 	log.Println("Found URL in exception list ", host)
		// 	gpt.DontTouch = true;
		// 	rs.Changed = true;
		// 	return
		// }
	})

	ngp.RegisterHandler("except_urls", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		host := string(*s)
		log.Println("Running exception handler for = ", host)
		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
		gatesentryf.RunFilter("url/all_exception_urls", host, responder)
		if responder.Blocked {
			gpt.DontTouch = true
			log.Println("URL found in exception = ", host)
			// *s = []byte(gatesentry2responder.BuildGeneralResponsePage( []string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1));
			rs.Changed = true
		}
	})
	// CONTENT FILTER
	ngp.RegisterHandler("content", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		log.Println("Running content handler")
		// log.Println("GPT = ", gpt)
		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
		if !gpt.DontTouch {
			gatesentryf.RunFilter("text/html", string(*s), responder)
			if responder.Blocked {
				*s = []byte(gatesentry2responder.BuildResponsePage(responder.Reasons, responder.Score))
			}
			rs.Changed = responder.Blocked
		}

	})

	// URL CHECKER
	ngp.RegisterHandler("url", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		host := string(*s)
		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
		gatesentryf.RunFilter("url/all_blocked_urls", host, responder)
		if responder.Blocked {
			*s = []byte(gatesentry2responder.BuildGeneralResponsePage([]string{"Unable to fulfill your request because it contains a <strong>blocked URL</strong>."}, -1))
			rs.Changed = true
		}
	})

	ngp.RegisterHandler("contentlength", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		length := string(*s)
		go func() {
			i, err := strconv.ParseUint(length, 10, 64)
			if err == nil {
				R.UpdateUserData(gpt.User, i)
			}
			x, err := strconv.ParseInt(length, 10, 64)
			if err == nil {
				R.UpdateConsumption(x)
			}
		}()
	})

	ngp.RegisterHandler("blockinternet", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		if gpt.User == "admin" {
			rs.Changed = false
		}
	})

	ngp.RegisterHandler("isauthuser", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		base64string := string(*s)
		rs.Changed = false
		if R.IsUserValid(base64string) {
			rs.Changed = true
		}
	})

	ngp.RegisterHandler("isaccessactive", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		*s = CannedResponseAccessNotActiveError
		rs.Changed = false
		if R.IsUserActive(gpt.User) {
			rs.Changed = true
		}
	})

	ngp.RegisterHandler("authenabled", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		*s = CannedResponsesAuthError
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

	ngp.RegisterHandler("timeallowed", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		// url := string(*s)
		rs.Changed = false
		blockedtimes := R.GSSettings.Get("blocktimes")
		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
		timezone := R.GSSettings.Get("timezone")
		gatesentry2filters.RunTimeFilter(responder, blockedtimes, timezone)
		// user := gpt.User
		if responder.Blocked {
			rs.Changed = true
			*s = []byte(gatesentry2responder.BuildGeneralResponsePage([]string{"Internet access on this network has been disabled because the current time has been specified as a blocked time period in GateSentry's settings."}, -1))
		}
		// gatesentry2.RunFilter( "url/all_exception_urls", host, responder )

	})

	ngp.RegisterHandler("prerequest", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		rs.Changed = false
		// return;
		// bactive, message := R.IsBlackoutModeActive()
		// if (bactive){
		// log.Println("Blackout mode activated.");
		// rs.Changed = true;
		// *s = []byte(gatesentry2responder.BuildGeneralResponsePage( []string{message}, -1));
		// }
	})

	ngp.RegisterHandler("contenttypeblocked", func(s *[]byte, rs *gatesentryproxy.GSResponder, gpt *gatesentryproxy.GSProxyPassthru) {
		contentType := string(*s)
		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}

		gatesentryf.RunFilter("url/all_blocked_mimes", contentType, responder)
		if responder.Blocked {
			rs.Changed = true
			message := "This content type has been blocked on this network."
			if contentType == "image/png" || contentType == "image/jpeg" || contentType == "image/jpg" {
				dat, _ := gatesentry2filters.Asset("app/transparent.png")
				*s = dat
			} else {
				*s = []byte(gatesentry2responder.BuildGeneralResponsePage([]string{message}, -1))
			}
			// return
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
	// apifile := GSBASEDIR+"/" +".api"

	// if _, er := os.Stat(apifile); os.IsNotExist(er) {
	//   // path/to/whatever does not exist

	// 	reader := bufio.NewReader(os.Stdin)
	// 	fmt.Println("Unable to read API file")
	// 	fmt.Println("Please register for an API key on https://www.gatesentryfilter.com/register")
	// 	fmt.Println("Then enter your API key here : ")
	// 	text, _ := reader.ReadString('\n')
	// 	dat:= []byte(text);
	// 	fmt.Println("Saving : "+ text)
	// 	fmt.Println("API saved in = " + apifile)
	// 	erro := ioutil.WriteFile(apifile, dat , 0777)
	// 	if erro != nil {
	// 		log.Fatal(erro)
	// 		//os.Exit(1)
	// 	}
	// }else{
	// 	filebytes,_ :=ioutil.ReadFile(apifile)
	// 	filestring := string(filebytes)
	// 	fmt.Println("API: " + filestring)
	// 	VerifyKey(filestring)
	// 	//os.Exit(1)
	// }

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
