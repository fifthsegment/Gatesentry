package gatesentryproxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/h2non/filetype"
)

type ContentScannerInput struct {
	Content     []byte
	ContentType string
	Url         string
}

func ScanMedia(dataToScan []byte, contentType string,
	r *http.Request,
	w http.ResponseWriter,
	resp *http.Response,
	buf bytes.Buffer,
	passthru *GSProxyPassthru) (bool, ProxyAction) {
	var httpResponseSent bool = false
	var proxyActionPerformed ProxyAction = ProxyActionFilterNone
	if filetype.IsAudio(dataToScan) || filetype.IsVideo(dataToScan) || filetype.IsImage(dataToScan) || isImage(contentType) || isVideo(contentType) {
		destwithcounter := &DataPassThru{
			Writer:      w,
			Contenttype: contentType,
			Passthru:    passthru,
		}
		copyResponseHeader(w, resp)

		// var contentScannerInput = ContentScannerInput{
		// 	Content:     dataToScan,
		// 	ContentType: contentType,
		// 	Url:         r.URL.String(),
		// }
		// contentScannerInputBytes, err := json.Marshal(contentScannerInput)
		// if err != nil {
		// 	log.Println(err)
		// 	showBlockPage(w, r, nil, PROXY_ERROR_UNABLE_TO_MARSHALL_DATA_FOR_SCANNING)
		// 	httpResponseSent = true
		// 	return httpResponseSent, ProxyActionFilterError
		// }

		// isBlocked, reasonForBlock := IProxy.RunHandler("contentscannerMedia", &contentScannerInputBytes, passthru)
		contentFilterData := GSContentFilterData{
			Url:         r.URL.String(),
			ContentType: contentType,
			Content:     dataToScan,
		}
		IProxy.ContentHandler(&contentFilterData)
		if contentFilterData.FilterResponseAction == ProxyActionBlockedMediaContent {
			proxyActionPerformed = ProxyActionBlockedMediaContent
			var reasonForBlockArray []string
			err := json.Unmarshal(contentFilterData.FilterResponse, &reasonForBlockArray)
			if err != nil {
				emptyImage, _ := createEmptyImage(500, 500, "jpeg", []string{"", "Error", err.Error()})

				emptyReader := bytes.NewReader(emptyImage)
				io.Copy(destwithcounter, emptyReader)
			} else {
				reasonForBlockArray = append([]string{"", "Image blocked by Gatesentry", "Reason(s) for blocking"}, reasonForBlockArray...)
				emptyImage, _ := createEmptyImage(500, 500, "jpeg", reasonForBlockArray)
				emptyReader := bytes.NewReader(emptyImage)
				io.Copy(destwithcounter, emptyReader)
			}

		} else {
			// newBuf, _ := createTextOverlayOnImage(buf.Bytes(), []string{"", "Gatesentry filtered", "No reason(s) for blocking"})
			io.Copy(destwithcounter, &buf)
			httpResponseSent = true
		}
		if DebugLogging {
			log.Println("IO Copy done for url = ", r.URL.String())
		}
	}
	return httpResponseSent, proxyActionPerformed
}

func ScanText(dataToScan []byte,
	contentType string,
	r *http.Request,
	w http.ResponseWriter,
	resp *http.Response,
	buf bytes.Buffer,
	passthru *GSProxyPassthru) (bool, ProxyAction) {
	var httpResponseSent bool = false
	var proxyActionPerformed ProxyAction = ProxyActionFilterNone
	if DebugLogging {
		log.Println("ScanText called for url = " + r.URL.String() + " content type = " + contentType)
	}
	if strings.Contains(contentType, "html") || len(contentType) == 0 {
		contentFilterData := GSContentFilterData{
			Url:         r.URL.String(),
			ContentType: contentType,
			Content:     (dataToScan),
		}
		IProxy.ContentHandler(&contentFilterData)
		// isBlocked, _ := IProxy.RunHandler("content", contentType, (&dataToScan), passthru)
		if contentFilterData.FilterResponseAction == ProxyActionBlockedTextContent {
			proxyActionPerformed = ProxyActionBlockedTextContent
			httpResponseSent = true
			//dataToScan gets modified to contain the blocked page
			sendBlockMessageBytes(w, r, nil, contentFilterData.FilterResponse, nil)
			resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		}

	}
	return httpResponseSent, proxyActionPerformed
}
