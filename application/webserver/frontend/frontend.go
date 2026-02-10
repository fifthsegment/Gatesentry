package gatesentryWebserverFrontend

import (
	"embed"
	"encoding/json"
	"html"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed all:files
var build embed.FS

func GetBlockPageMaterialUIStylesheet() []byte {
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

// GetIndexHtmlWithBasePath returns index.html with the base path injected.
// Injects a <script> setting window.__GS_BASE_PATH__ and a <base href> tag
// so the Svelte SPA can resolve assets and API calls relative to the base path.
func GetIndexHtmlWithBasePath(basePath string) []byte {
	raw := GetIndexHtml()
	if raw == nil {
		return nil
	}

	// For root base path, no injection needed
	if basePath == "/" {
		return raw
	}

	htmlStr := string(raw)

	// Build injection tags — escape basePath for safe HTML/JS injection
	baseHref := html.EscapeString(basePath + "/")
	jsPath, _ := json.Marshal(basePath) // produces a safely-quoted JSON string
	injection := `<base href="` + baseHref + `">` + "\n" +
		`    <script>window.__GS_BASE_PATH__ = ` + string(jsPath) + `;</script>`

	// Inject after <head> or after first <meta> tag
	htmlStr = strings.Replace(htmlStr, "<head>", "<head>\n    "+injection, 1)

	return []byte(htmlStr)
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
