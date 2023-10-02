package gatesentryWebserverEndpoints

import (
	"crypto/tls"
	"crypto/x509"
	"log"

	"github.com/kataras/iris/v12"
)

func ApiVerifyCert(ctx iris.Context) {
	type Datareceiver struct {
		Key   string `json:key`
		Value string `json:value`
	}
	var temp Datareceiver
	err := ctx.ReadJSON(&temp)
	_ = err
	if err != nil {
		return
	}
	keyPEMBlock := []byte(temp.Key)
	certPEMBlock := []byte(temp.Value)

	if len(certPEMBlock) != 0 && len(keyPEMBlock) != 0 {
		log.Println(string(keyPEMBlock))
		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			log.Println("Error loading TLS certificate:", err)
			ctx.JSON(struct {
				Value  string
				Status int
			}{Value: "Error loading TLS certificate: " + err.Error(), Status: 2})
			return
		}
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			log.Println("Error parsing X509 certificate:", err)
			ctx.JSON(struct {
				Value  string
				Status int
			}{Value: "Error parsing X509 certificate: " + err.Error(), Status: 2})
			return
		}
		_ = parsed
	}
	ctx.JSON(struct {
		Value  string
		Status int
	}{Value: "Certificate loaded succesfully", Status: 1})
}
