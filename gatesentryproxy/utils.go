package gatesentryproxy

import (
	"mime"
	"net"
	"strings"
)

func isLanAddress(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch ip4[0] {
		case 10, 127:
			return true
		case 172:
			return ip4[1]&0xf0 == 16
		case 192:
			return ip4[1] == 168
		}
		return false
	}

	// IPv6
	switch {
	case ip[0]&0xfe == 0xfc:
		return true
	case ip[0] == 0xfe && (ip[1]&0xfc) == 0x80:
		return true
	case ip.Equal(ip6Loopback):
		return true
	}

	return false
}

func isAVIF(data []byte) bool {
	// Check for 'ftyp' box and 'avif' major brand
	return len(data) > 12 &&
		string(data[4:8]) == "ftyp" &&
		string(data[8:12]) == "avif"
}

func isVideo(contentType string) bool {
	return (contentType == "video/webm" ||
		contentType == "video/mp4" ||
		contentType == "video/x-ms-wmv" ||
		contentType == "audio/mpeg" ||
		contentType == "video/x-msvideo" ||
		contentType == "video/jpeg")
}

func isImage(contentType string) bool {
	return (contentType == "image/png" ||
		contentType == "image/avif" ||
		contentType == "image/gif" ||
		contentType == "image/jpeg" ||
		contentType == "image/jpg" ||
		contentType == "image/webp" ||
		contentType == "image/svg+xml" ||
		contentType == "image/bmp" ||
		contentType == "image/x-icon")

	// 	cContentType == "image/x-icon" ||
	// 	cContentType == "text/css" ||
	// 	cContentType == "font/woff2" ||
	// 	cContentType == "application/x-font-woff" ||
	// 	cContentType == "application/zip" ||
	// 	cContentType == "application/x-msdownload" ||
	// 	cContentType == "application/octet-stream" ||
	// 	cContentType == "application/x-javascript" ||
	// 	cContentType == "application/javascript" {
	// 	log.Println("Not filtering, sending directly to client")

	// }
}

func getFileExtensionFromUrl(urlString string) string {
	if strings.Contains(urlString, "?") {
		urlString = strings.Split(urlString, "?")[0]
	}
	return urlString[strings.LastIndex(urlString, ".")+1:]
}

func getMimeByExtension(extension string) string {
	mimeType := mime.TypeByExtension("." + extension)
	if strings.Contains(mimeType, ";") {
		mimeType = strings.Split(mimeType, ";")[0]
	}
	return mimeType
}

func isUrlContainingImage(urlString string) bool {
	// remove query string
	if strings.Contains(urlString, "?") {
		urlString = strings.Split(urlString, "?")[0]
	}
	if strings.HasSuffix(urlString, ".jpeg") ||
		strings.HasSuffix(urlString, ".jpg") ||
		strings.HasSuffix(urlString, ".png") ||
		strings.HasSuffix(urlString, ".gif") ||
		strings.HasSuffix(urlString, ".bmp") ||
		strings.HasSuffix(urlString, ".ico") ||
		strings.HasSuffix(urlString, ".svg") {
		return true
	}
	return false
}
