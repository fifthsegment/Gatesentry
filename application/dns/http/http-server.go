package dnsHttpServer

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"

	dnsCerts "bitbucket.org/abdullah_irfan/gatesentryf/dns/cert"
	gatesentryDnsUtils "bitbucket.org/abdullah_irfan/gatesentryf/dns/utils"
)

var (
	blockPage = []byte(`
						<!DOCTYPE html>
						<html>
						<head>
							<title>Gatesentry DNS</title>
						</head>
						<body>
							<p>Gatesentry DNS Server home.</p>
							<p></p>
						</body>
						</html>
					`)
	localIp, _    = gatesentryDnsUtils.GetLocalIP()
	serverSecure  *http.Server
	server        *http.Server
	serverRunning bool = false
)

func StartHTTPServer() {
	serverRunning = true
	// http.HandleFunc("/", handleServerPages)
	go func() {
		fmt.Println("HTTP server listening on :80")

		server = &http.Server{
			Addr:    ":80",
			Handler: http.HandlerFunc(handleServerPages),
		}
		err := server.ListenAndServe()
		if err != nil {
			fmt.Println("Error starting HTTP server:", err)
		}
	}()

	// HTTPS server on port 443
	go func() {
		// ca := &x509.Certificate{
		// 	SerialNumber: big.NewInt(2019),
		// 	Subject: pkix.Name{
		// 		Organization:  []string{"Company, INC."},
		// 		Country:       []string{"US"},
		// 		Province:      []string{""},
		// 		Locality:      []string{"San Francisco"},
		// 		StreetAddress: []string{"Golden Gate Bridge"},
		// 		PostalCode:    []string{"94016"},
		// 	},
		// 	NotBefore:             time.Now(),
		// 	NotAfter:              time.Now().AddDate(10, 0, 0),
		// 	IsCA:                  true,
		// 	ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		// 	KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		// 	BasicConstraintsValid: true,
		// }

		// caPrivKey, errKey := rsa.GenerateKey(rand.Reader, 4096)
		// if errKey != nil {
		// 	fmt.Println("Error generating private key:", errKey)
		// 	return
		// }

		// caBytes, errBytes := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
		// if errBytes != nil {
		// 	fmt.Println("Error creating certificate:", errBytes)
		// 	return
		// }

		// caPEM := new(bytes.Buffer)
		// pem.Encode(caPEM, &pem.Block{
		// 	Type:  "CERTIFICATE",
		// 	Bytes: caBytes,
		// })

		// caPrivKeyPEM := new(bytes.Buffer)
		// pem.Encode(caPrivKeyPEM, &pem.Block{
		// 	Type:  "RSA PRIVATE KEY",
		// 	Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
		// })
		// cert, err := tls.X509KeyPair(caPEM.Bytes(), caPrivKeyPEM.Bytes())

		cert, err := tls.X509KeyPair(dnsCerts.CaPEM, dnsCerts.CaPrivKeyPEM)
		if err != nil {
			fmt.Println("Error loading certificate:", err)
			return
		}

		config := &tls.Config{Certificates: []tls.Certificate{cert}}

		serverSecure = &http.Server{
			Addr:      ":443",
			TLSConfig: config,
			Handler:   http.HandlerFunc(handleServerPages),
		}

		err = serverSecure.ListenAndServeTLS("", "")
		if err != nil {
			fmt.Println("Error starting server:", err)
		}
	}()
}

func StopHTTPServer() {
	serverRunning = false
	// serverSecure.Shutdown(context.Background())
	// serverSecure.Close()
	// server.Shutdown(context.Background())
	// server.Close()
	// serverSecure = nil
	// server = nil
}

func handleServerPages(w http.ResponseWriter, r *http.Request) {
	if serverRunning == false {
		log.Println("HTTP server is not running")
		w.Write([]byte("HTTP server is currently disabled"))
		return
	}
	if r.TLS == nil {
		// Serve different content for HTTP (port 80)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(blockPage)
	} else {
		http.Redirect(w, r, "http://"+localIp, http.StatusSeeOther)
	}
}
