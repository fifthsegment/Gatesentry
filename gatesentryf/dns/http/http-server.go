package dnsHttpServer

import (
	"crypto/tls"
	"fmt"
	"net/http"

	dnsCerts "bitbucket.org/abdullah_irfan/gatesentryf/dns/cert"
	gatesentryDnsUtils "bitbucket.org/abdullah_irfan/gatesentryf/dns/utils"
)

var (
	blockPage = []byte(`
						<!DOCTYPE html>
						<html>
						<head>
							<title>Blocked Website</title>
						</head>
						<body>
							<h1>Website Blocked</h1>
							<p>This website is blocked.</p>
							<p></p>
						</body>
						</html>
					`)
	localIp, _ = gatesentryDnsUtils.GetLocalIP()
)

func StartHTTPServer() {
	http.HandleFunc("/", handleServerPages)
	fmt.Println("HTTP server listening on :80")
	go func() {
		err := http.ListenAndServe(":80", nil)
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

		server := &http.Server{
			Addr:      ":443",
			TLSConfig: config,
		}

		err = server.ListenAndServeTLS("", "")
		if err != nil {
			fmt.Println("Error starting server:", err)
		}
	}()
}

func handleServerPages(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		// Serve different content for HTTP (port 80)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(blockPage)
	} else {
		http.Redirect(w, r, "http://"+localIp, http.StatusSeeOther)
	}
}
