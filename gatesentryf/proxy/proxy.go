package gatesentry2proxy;

// import (
// 	"flag"
// 	"log"
// 	"net/http"
// 	// "github.com/abourget/goproxy"
// 	"fmt"
// 	// "strconv"
// 	// "gatesentry2"
// )

// func (proxy *GSProxy) Listen(){
// 	verbose := flag.Bool("v", false, "should every proxy request be logged to stdout")
// 	addr := flag.String("addr", ":8092", "proxy listen address")
// 	flag.Parse()
// 	proxy.Verbose = *verbose
// 	fmt.Println("Proxy is now listening for connections")
// 	// go func(){log.Fatal(http.ListenAndServe(*addr, proxy))}();
// 	log.Fatal(http.ListenAndServe(*addr, proxy));
// }

// func StartProxy() *GSProxy{
// 	proxy := goproxy.NewProxyHttpServer()
// 	gproxy := InitGSProxy(proxy);

// 	con, err := goproxy.LoadCAConfig(CA_CERT, CA_KEY)

// 	if ( err != nil ){
// 		panic(err)
// 	}

// 	gproxy.SetMITMCertConfig(con)

// 	// gproxy.HandleConnect(goproxy.AlwaysMitm)







	
// 	// fmt.Println("Starting proxy")
	
// 	fmt.Println("Handing back control");
// 	return gproxy;
// }