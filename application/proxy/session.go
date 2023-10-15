package gatesentry2proxy

import (
	"gopkg.in/elazarl/goproxy.v1"
)

type GSPassThru struct {
	DONT_TOUCH bool
}

func InitGSession() *GSPassThru {
	g := GSPassThru{DONT_TOUCH: false}
	return &g
}

func SetSession(ctx *goproxy.ProxyCtx) {
	if ctx.UserData == nil {
		sess := InitGSession()
		ctx.UserData = sess
	}
}

func SetSessionData(ctx *goproxy.ProxyCtx, key string, value interface{}) {
	SetSession(ctx)
	sess := ctx.UserData.(*GSPassThru)
	switch key {
	case "DONT_TOUCH":
		// ctx.UserData.DONT_TOUCH = value.(bool);
		sess.DONT_TOUCH = value.(bool)
		break
	}
}

func GetSessionData(ctx *goproxy.ProxyCtx, key string) interface{} {
	SetSession(ctx)
	sess := ctx.UserData.(*GSPassThru)
	switch key {
	case "DONT_TOUCH":
		// ctx.UserData.DONT_TOUCH = value.(bool);
		return sess.DONT_TOUCH
		break
	}
	return nil
}
