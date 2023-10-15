package gatesentry2proxy

import "gopkg.in/elazarl/goproxy.v1"

// import (
// 	"gopkg.in/elazarl/goproxy.v1"
// )

type GSProxy struct {
	*goproxy.ProxyHttpServer
	// ProxySessions map[int64]*GSPassThru
	// Original *goproxy.ProxyHttpServer
}

func InitGSProxy(P *goproxy.ProxyHttpServer) *GSProxy {
	// var sessions map[int64]*GSPassThru
	// sessions := make(map[int64]*GSPassThru, 100000)
	proxy := GSProxy{P}
	return &proxy
}

// func (P *GSProxy) HandleRequestFunc(f func(ctx *goproxy.ProxyCtx) goproxy.Next) {
// 	// fmt.Println("Registering Request Function")
// 	P.ProxyHttpServer.HandleRequestFunc(f)
// }

// func (P *GSProxy) HandleConnectFunc(f func(ctx *goproxy.ProxyCtx) goproxy.Next) {
// 	// fmt.Println("I was called");
// 	(P.ProxyHttpServer.HandleConnectFunc(f))
// 	// .HandleConnectFunc(f);
// }

// func (P *GSProxy) HandleResponseFunc(f func(ctx *goproxy.ProxyCtx) goproxy.Next) {
// 	// fmt.Println("Registering response handler");
// 	P.ProxyHttpServer.HandleResponseFunc(f)
// }

// func (P *GSProxy) InitGSession() *GSPassThru {
// 	g := GSPassThru{DONT_TOUCH: false}
// 	return &g
// }
