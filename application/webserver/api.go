package gatesentryWebserver

import (
	"net/http"

	"github.com/gorilla/mux"
)

type GsWeb struct {
	router   *mux.Router // root router (handles redirect, serves subrouter)
	sub      *mux.Router // subrouter mounted at basePath — all routes go here
	basePath string
}

type HttpHandlerFunc func(http.ResponseWriter, *http.Request)

func NewGsWeb(basePath string) *GsWeb {
	root := mux.NewRouter()

	var sub *mux.Router
	if basePath == "/" {
		sub = root
	} else {
		sub = root.PathPrefix(basePath).Subrouter()
		// Redirect bare root to the base path
		root.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, basePath+"/", http.StatusFound)
		})
	}

	return &GsWeb{
		router:   root,
		sub:      sub,
		basePath: basePath,
	}
}

func (g *GsWeb) Get(path string, handlerOrMiddleware interface{}, optionalHandler ...HttpHandlerFunc) {
	switch h := handlerOrMiddleware.(type) {
	case HttpHandlerFunc:
		g.sub.Handle(path, http.HandlerFunc(h)).Methods("GET")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.sub.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("GET")
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
		g.sub.Handle(path, http.HandlerFunc(h)).Methods("POST")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.sub.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("POST")
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
		g.sub.Handle(path, http.HandlerFunc(h)).Methods("PUT")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.sub.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("PUT")
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
		g.sub.Handle(path, http.HandlerFunc(h)).Methods("DELETE")
	case mux.MiddlewareFunc:
		if len(optionalHandler) > 0 {
			g.sub.Handle(path, h(http.HandlerFunc(optionalHandler[0]))).Methods("DELETE")
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
