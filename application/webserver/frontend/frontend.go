package gatesentryWebserverFrontend

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed files
var build embed.FS

func GetStyles() []byte {
	cssData, err := fs.ReadFile(build, "files/material.css")
	if err != nil {
		return nil
	}
	return cssData

}

func GetIndexHtml() []byte {
	indexData, err := fs.ReadFile(build, "files/index.html")
	if err != nil {
		return nil
	}
	return indexData

}

func GetFileSystem(dir string, fsys fs.FS) http.FileSystem {
	// if useOS {
	// 	log.Println("[Webserver] using live mode")
	// 	return http.FS(os.DirFS(dir))
	// }

	log.Print("[Webserver] using embed mode")
	fsys, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}

func GetFSHandler() http.FileSystem {
	log.Print("[Webserver] using embed mode")
	fsys, err := fs.Sub(build, "files")
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}
