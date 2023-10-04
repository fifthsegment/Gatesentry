package gatesentry2securetransport

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"

	// "../../gatesentry2/storage"
	"net"
	"strconv"
	"strings"
	"time"
	// "fmt"
)

// var baseEndpoint = "https://gatesentry.fifthsegment.com/api"
var baseEndpoint = "http://gatesentry.fifthsegment.com/api"
var PinnedServerCert = `MIIGhzCCBW+gAwIBAgIRALoJhaWynZO3uQPTHuCBawwwDQYJKoZIhvcNAQELBQAw
    gY8xCzAJBgNVBAYTAkdCMRswGQYDVQQIExJHcmVhdGVyIE1hbmNoZXN0ZXIxEDAO
    BgNVBAcTB1NhbGZvcmQxGDAWBgNVBAoTD1NlY3RpZ28gTGltaXRlZDE3MDUGA1UE
    AxMuU2VjdGlnbyBSU0EgRG9tYWluIFZhbGlkYXRpb24gU2VjdXJlIFNlcnZlciBD
    QTAeFw0xOTA2MjEwMDAwMDBaFw0yMTA2MjAyMzU5NTlaMFgxITAfBgNVBAsTGERv
    bWFpbiBDb250cm9sIFZhbGlkYXRlZDEUMBIGA1UECxMLUG9zaXRpdmVTU0wxHTAb
    BgNVBAMTFGdhdGVzZW50cnlmaWx0ZXIuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOC
    AQ8AMIIBCgKCAQEA1Y8c+mGxQ/cokAo/8xcU1QJQcDH4TbRbphcpYGsAUxqGx6gc
    TXSn4O+49Y5SJ7wCv/Q+BuSWEzKXui9S8rGDa7b3uSk7KzW2mlleQuLvxQLph2oZ
    Y9e8jvKFEkc2jTmA8zwFqD8g+20ewlrQrRIznp5oWxRKTvCnEkpqWdrQlkyi32tZ
    i6j6z4QiOCa9/OvGwmPhnM+0OHgL9DuPB9i8C1zaZxeVn8DxUfUTD5tBDyAY80e9
    62iA1FPbg7Wif2X2+SrInyp2cpg2/pJkuUzYgCsHPl4EILR8SafGBK1WRFHYVrOd
    692bzn/mtQ7H3omv95yprJKLvM+l2YRbhk7QlwIDAQABo4IDEjCCAw4wHwYDVR0j
    BBgwFoAUjYxexFStiuF36Zv5mwXhuAGNYeEwHQYDVR0OBBYEFH4LySekFnazzPai
    UEAA4q7PVRnVMA4GA1UdDwEB/wQEAwIFoDAMBgNVHRMBAf8EAjAAMB0GA1UdJQQW
    MBQGCCsGAQUFBwMBBggrBgEFBQcDAjBJBgNVHSAEQjBAMDQGCysGAQQBsjEBAgIH
    MCUwIwYIKwYBBQUHAgEWF2h0dHBzOi8vc2VjdGlnby5jb20vQ1BTMAgGBmeBDAEC
    ATCBhAYIKwYBBQUHAQEEeDB2ME8GCCsGAQUFBzAChkNodHRwOi8vY3J0LnNlY3Rp
    Z28uY29tL1NlY3RpZ29SU0FEb21haW5WYWxpZGF0aW9uU2VjdXJlU2VydmVyQ0Eu
    Y3J0MCMGCCsGAQUFBzABhhdodHRwOi8vb2NzcC5zZWN0aWdvLmNvbTA5BgNVHREE
    MjAwghRnYXRlc2VudHJ5ZmlsdGVyLmNvbYIYd3d3LmdhdGVzZW50cnlmaWx0ZXIu
    Y29tMIIBgAYKKwYBBAHWeQIEAgSCAXAEggFsAWoAdwC72d+8H4pxtZOUI5eqkntH
    OFeVCqtS6BqQlmQ2jh7RhQAAAWt4fkWRAAAEAwBIMEYCIQD0J1HdsSCTl+rV5U/U
    XFWdYrqgRE16oqYffEEJNbEkHwIhAKCmBk6Rr0HBl4THeN0ZskCAw5CwAAQeIow8
    h3ggCQofAHYARJRlLrDuzq/EQAfYqP4owNrmgr7YyzG1P9MzlrW2gagAAAFreH5G
    YwAABAMARzBFAiEA0e/puuDUJ0W33/76uKoaOYjskVcav7q7bHE7nB+qBcwCIAPc
    mFeYoZEtxhSvTiubbSY0HvH7S4oxlDBhTkpLmIxOAHcAb1N2rDHwMRnYmQCkURX/
    dxUcEdkCwQApBo2yCJo32RMAAAFreH5FsQAABAMASDBGAiEAzapKDbKDDCeQ29wR
    97NPsBnlKH8IEAhi1TMZeZ3/Y9ICIQDiBLVgpshkwFMap4mcMvdTl+N/e0jgy0BJ
    cJfLbhxmujANBgkqhkiG9w0BAQsFAAOCAQEAbbx52hGBLdd/yDObzmV7eirdhaCP
    j5o663SZYoLlaMTJM7nliRKIjZJMTr+XSYsFbmsGCxAUeOfFCWEmXsrP9rw8c4Ec
    iKbOWeTwGa8LTKCERNfx4txpnVKy29dxzCZ7znx7uNNn66E8LlFQgL6j1HYSFlxi
    0Oc5INGyumPZ8sNtj5aR32PBcps8GXbGkXF9EzD1LNuEEvjxb7FKGeqCCT1s36aX
    y9PaUncVLMwa02gM14O3d5DdFupey+AsVy7Pzs1HD+sy9NaJyq1gWYwVbQvMV/yd
    5SbDvFJfz5tM6OLC9cBVFdLkKY0DFnOqzysgXegktViaCHIkpqngQ15AeA==`

// var baseEndpoint = "a";

type GSSecureResponse struct {
	Data string
	Sign string
	Time string
}

type GSSecureRequest struct {
	Data string
}

type GSPreSecureRequest struct {
	InstallationId string
	Data           string
}

func SetbaseEndpoint(ep string) {
	baseEndpoint = ep
	log.Println("Setting API base to = " + baseEndpoint)
}

func doRequestGET(endpoint string) ([]byte, error) {
	log.Println("Doing a GET to = " + baseEndpoint + endpoint)
	client := GETHttpClient()

	resp, err := client.Get(baseEndpoint + endpoint)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return body, err
}

func doRequestPOST(endpoint string, buf []byte) ([]byte, error) {
	log.Println("Doing a POST to = " + baseEndpoint + endpoint)
	client := GETHttpClient()
	data := bytes.NewBuffer(buf)

	resp, err := client.Post(baseEndpoint+endpoint, "application/json", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return body, err
}

func istimeCheckValid(senton string) bool {
	t := time.Now()

	i, err := strconv.ParseInt(senton, 10, 64)
	if err != nil {
		return false
	}
	s := time.Unix(i, 0)
	dur := t.Sub(s)

	if dur.Seconds() > 60 {
		return false
	}
	// _=t;
	return true
}

// func verifyAndDecodeResponse(resp []byte)(string, error){
// 	var jresp GSSecureResponse;
// 	json.Unmarshal(resp, &jresp);
// 	d, err := gatesentryweb.VerifySig(jresp.Data, jresp.Sign)
// 	if ( err != nil || len(d) == 0 ){
// 		return "", err
// 	}

// 	var g GSSecureResponse
// 	json.Unmarshal([]byte(jresp.Data), &g);
// 	if (!istimeCheckValid(g.Time)){
// 		return "", errors.New("Response has already expired")
// 	}
// 	dec, err := base64.StdEncoding.DecodeString(g.Data)
// 	// fmt.Println( dec )
//     if err != nil {
//         return "", err
//     }
//     return string(dec),nil;
// }

/**
* After Version this will eventually be phased out
*
 */
func doRequestSecure(endpoint string, rtype string, datatosend []byte) (string, error) {
	log.Println("Doing a secure request to " + endpoint)
	switch rtype {
	case "get", "GET":
		// case "GET":
		resp, err := doRequestGET(endpoint)
		if err != nil {
			log.Println(err)
			return "", err
		}
		return string(resp), nil
		// return verifyAndDecodeResponse(resp)
		break
	case "post", "POST":
		if datatosend == nil {
			return "", errors.New("You havent sent any data with this POST request")
		}
		// case "POST":
		/*encd, err := gatesentryweb.EncTest(string(datatosend))
		if err != nil {
			return "", err
		}*/
		encd := "s"
		gss := GSSecureRequest{Data: encd}
		gssj, err := json.Marshal(gss)
		if err != nil {
			return "", err
		}
		resp, err := doRequestPOST(endpoint, gssj)
		if err != nil {
			log.Println(err)
			return "", err
		}
		return string(resp), nil
		// return verifyAndDecodeResponse(resp)
		break

	}
	return "", errors.New("Incorrect Request Type, please use a GET OR POST")
}

/**
* 1.73 : No Longer encrypts data
*
 */
func SendEncryptedData(endpoint string, data []byte, key string) (string, error) {
	// var data []byte;
	// data = []byte("{\"X\":\"Client Hello\"}");
	// key := ENCRYPTIONKEY;
	log.Println("Sending data to /" + endpoint)
	// crypted , err := gatesentry2storage.Encrypt(data,[]byte(key));
	// if ( err != nil ){
	// 	// panic(err)
	// 	return "", err ;
	// }
	sendthisData := GSPreSecureRequest{InstallationId: key, Data: string(data)}
	sendthisJson, err := json.Marshal(sendthisData)
	if err != nil {
		return "", err
	}
	return doRequestSecure(endpoint, "post", sendthisJson)
}

func certRemoveWhiteSpace(certString string) string {
	certString = strings.Replace(certString, "\n", "", -1)
	certString = strings.Replace(certString, "\t", "", -1)
	certString = strings.Replace(certString, "    ", "", -1)
	return certString
}

func PNDGet(url string) (string, error) {
	certString := PinnedServerCert
	certString = certRemoveWhiteSpace(certString)
	client := &http.Client{}
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	client.Transport = &http.Transport{
		DialTLS: func(network, addr string) (net.Conn, error) {
			conn, err := tls.Dial(network, addr, tlsConfig)
			if err != nil {
				return conn, err
			}
			if len(conn.ConnectionState().PeerCertificates) == 0 {
				return nil, errors.New("No certificates found in chain")
			}
			sEnc := base64.StdEncoding.EncodeToString(conn.ConnectionState().PeerCertificates[0].Raw)
			if sEnc != certString {
				return nil, errors.New("Unable to validate certificate")
			}
			return conn, nil
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
		// os.Exit(1)
	}
	return string(contents), nil
}

func GETHttpClient() *http.Client {
	certString := PinnedServerCert
	certString = certRemoveWhiteSpace(certString)
	client := &http.Client{}
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	client.Transport = &http.Transport{
		DialTLS: func(network, addr string) (net.Conn, error) {
			conn, err := tls.Dial(network, addr, tlsConfig)
			if err != nil {
				return conn, err
			}
			if len(conn.ConnectionState().PeerCertificates) == 0 {
				return nil, errors.New("No certificates found in chain")
			}
			sEnc := base64.StdEncoding.EncodeToString(conn.ConnectionState().PeerCertificates[0].Raw)
			if sEnc != certString {
				return nil, errors.New("Unable to validate certificate")
			}
			return conn, nil
		},
	}
	return client
}

func PNDPost(url string) (string, error) {
	client := GETHttpClient()
	resp, err := client.Post(url, "application/json", nil)
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
		// os.Exit(1)
	}
	return string(contents), nil
}
