package gatesentryf

// import (
// 	"fmt"
// 	"github.com/abourget/goproxy"
// 	"gatesentry2/proxy"
// 	"gatesentry2/responder"
// 	"gatesentry2/proxy/ext"
// 	// "net/http"
// 	// "io"
// 	// "os"
// 	// "log"

// )

// func RegisterProxyHandlers(){
// 	fmt.Println("Registering proxy handlers")

// 	R.Proxy.HandleRequestFunc(func(ctx *goproxy.ProxyCtx) goproxy.Next {
// 		R.Logger.Log(ctx);
// 		if (ctx.UserObjects["gssession"]==nil ){
// 			ctx.UserObjects["gssession"] = R.Proxy.InitGSession();
// 		}
// 		// ctx.UserObjects["gssession"] = R.Proxy.InitGSession();
// 		// sess := gproxy.GetGSession(ctx.Session);
// 		// sess.A = "Hello world" + strconv.FormatInt(ctx.Session,10);
// 		return goproxy.NEXT
// 	});

// 	R.Proxy.HandleConnectFunc(func(ctx *goproxy.ProxyCtx ) goproxy.Next{
// 		host:=ctx.SNIHost()
// 		responder := &gatesentry2responder.GSFilterResponder{Blocked: false}
// 		RunFilter( "url/https_dontbump", host, responder );
// 		var sess *gatesentry2proxy.GSPassThru;
// 		if ( ctx.UserObjects["gssession"] == nil ){

// 			// fmt.Println(ctx.UserObjects["gssession"])
// 			// fmt.Println("GS Session is empty, creating one")
// 			ctx.UserObjects["gssession"] = R.Proxy.InitGSession();
// 			// fmt.Println(ctx.UserObjects["gssession"])
// 		}else{
// 			// sess = ctx.UserObjects["gssession"].(*gatesentry2proxy.GSPassThru);
// 		}
// 		sess = ctx.UserObjects["gssession"].(*gatesentry2proxy.GSPassThru);
// 		if ( responder.Blocked ){

// 			sess.DONT_TOUCH = true;
// 			// A blocked here means the url is present in the list
// 			// The list in this case happens to be the exception site list
// 			// So a url on the list means to not MITM it.
// 			return goproxy.FORWARD
// 		}
// 		responder = &gatesentry2responder.GSFilterResponder{Blocked: false}
// 		url := ctx.Req.URL.String();
// 		RunFilter( "url/all_exception_urls", url , responder )
// 		if ( responder.Blocked ){
// 			fmt.Println("Setting non touch on " + url)
// 			sess.DONT_TOUCH = true;
// 			ctx.UserObjects["gssession"] = sess;
// 			// A blocked here means the url is present in the list
// 			// The list in this case happens to be the exception site list
// 			// So a url on the list means to not to block it.

// 		}

// 		responder = &gatesentry2responder.GSFilterResponder{Blocked: false}
// 		RunFilter( "url/all_blocked_urls", url , responder )
// 		if ( responder.Blocked ){
// 			// ctx.SetDestinationHost("127.0.0.1")
// 			// so that Bing receives the right `Host:` header
// 			// ctx.Req.Host = "127.0.0.1"
// 			return goproxy.REJECT
// 		// 	return r, goproxy.NewResponse(r,goproxy.ContentTypeHtml, http.StatusForbidden,gatesentry2responder.BuildResponsePage( []string{"URL Blocked"}, -1) );
// 		}
// 		return goproxy.MITM
// 	})

// 	contentHandler := goproxy.HandlerFunc(func(ctx *goproxy.ProxyCtx) goproxy.Next {
// 		fmt.Println("Im a content handler")
// 		// fmt.Println(string(ctx.Resp.Body) )
// 		// _, err := io.Copy(os.Stdout, ctx.Resp.Body)
//         // if err != nil {
//         //         log.Fatal(err)
//         // }
// 		f := gatesentry2ext_html.HandleString(
// 			func(s string, ctx *goproxy.ProxyCtx) string {
// 				fmt.Println("I'm here a content handler")
// 				sess := ctx.UserObjects["gssession"].(*gatesentry2proxy.GSPassThru);
// 				fmt.Println( sess );
// 				// return s;
// 				// R.Logger.Log(ctx);
// 				if ( !sess.DONT_TOUCH ){
// 					return Handle_Html_Response(s)
// 				}
// 				return s;
// 		})
// 		f.Handle(ctx.Resp, ctx );
// 	 	return goproxy.NEXT;
// 	})
// 	R.Proxy.HandleResponse(goproxy.RespContentTypeIs("text/html", "text/json", "text/xml")(contentHandler));
// }
