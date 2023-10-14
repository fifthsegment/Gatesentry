package gatesentryWebserver

import (
	"net/http"

	"github.com/gorilla/mux"
)

type GsWeb struct {
	router *mux.Router
}

type HttpHandlerFunc func(http.ResponseWriter, *http.Request)

func NewGsWeb() *GsWeb {
	return &GsWeb{
		router: mux.NewRouter(),
	}
}

func (g *GsWeb) Get(path string, handlerOrMiddleware interface{}, optionalHandler ...HttpHandlerFunc) {
	switch h := handlerOrMiddleware.(type) {
	case HttpHandlerFunc:
		g.router.Handle(path, http.HandlerFunc(h)).Methods("GET")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.router.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("GET")
		} else {
			panic("middleware provided but no handler function")
		}
	default:
		panic("unsupported type provided to GET method")
	}
}

func (g *GsWeb) Post(path string, handlerOrMiddleware interface{}, optionalHandler ...HttpHandlerFunc) {
	switch h := handlerOrMiddleware.(type) {
	case HttpHandlerFunc:
		g.router.Handle(path, http.HandlerFunc(h)).Methods("POST")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.router.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("POST")
		} else {
			panic("middleware provided but no handler function")
		}
	default:
		panic("unsupported type provided to POST method")
	}
}

func (g *GsWeb) Put(path string, handlerOrMiddleware interface{}, optionalHandler ...HttpHandlerFunc) {
	switch h := handlerOrMiddleware.(type) {
	case HttpHandlerFunc:
		g.router.Handle(path, http.HandlerFunc(h)).Methods("PUT")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.router.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("PUT")
		} else {
			panic("middleware provided but no handler function")
		}
	default:
		panic("unsupported type provided to PUT method")
	}
}

func (g *GsWeb) Delete(path string, handlerOrMiddleware interface{}, optionalHandler ...HttpHandlerFunc) {
	switch h := handlerOrMiddleware.(type) {
	case HttpHandlerFunc:
		g.router.Handle(path, http.HandlerFunc(h)).Methods("DELETE")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.router.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("DELETE")
		} else {
			panic("middleware provided but no handler function")
		}
	default:
		panic("unsupported type provided to DELETE method")
	}
}

func (g *GsWeb) ListenAndServe(port string) error {
	return http.ListenAndServe(port, g.router)
}
