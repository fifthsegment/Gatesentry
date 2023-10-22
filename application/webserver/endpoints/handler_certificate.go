package gatesentryWebserverEndpoints

import (
	"crypto/x509"
	"encoding/pem"
	"errors"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// func ApiVerifyCert(ctx iris.Context) {
// 	type Datareceiver struct {
// 		Key   string `json:key`
// 		Value string `json:value`
// 	}
// 	var temp Datareceiver
// 	err := ctx.ReadJSON(&temp)
// 	_ = err
// 	if err != nil {
// 		return
// 	}
// 	keyPEMBlock := []byte(temp.Key)
// 	certPEMBlock := []byte(temp.Value)

// 	if len(certPEMBlock) != 0 && len(keyPEMBlock) != 0 {
// 		log.Println(string(keyPEMBlock))
// 		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
// 		if err != nil {
// 			log.Println("Error loading TLS certificate:", err)
// 			ctx.JSON(struct {
// 				Value  string
// 				Status int
// 			}{Value: "Error loading TLS certificate: " + err.Error(), Status: 2})
// 			return
// 		}
// 		parsed, err := x509.ParseCertificate(cert.Certificate[0])
// 		if err != nil {
// 			log.Println("Error parsing X509 certificate:", err)
// 			ctx.JSON(struct {
// 				Value  string
// 				Status int
// 			}{Value: "Error parsing X509 certificate: " + err.Error(), Status: 2})
// 			return
// 		}
// 		_ = parsed
// 	}
// 	ctx.JSON(struct {
// 		Value  string
// 		Status int
// 	}{Value: "Certificate loaded succesfully", Status: 1})
// }

const CERTIFICATE_KEY = "capem"

type CertInfo struct {
	Name   string `json:"name"`
	Expiry string `json:"expiry"`
	Error  string `json:"error"`
}

func GetCertificateBytes(settings *gatesentry2storage.MapStore) []byte {
	cert := settings.Get(CERTIFICATE_KEY)

	return []byte(cert)
}

func GetCertificateInfo(settings *gatesentry2storage.MapStore) interface{} {
	cert := settings.Get(CERTIFICATE_KEY)
	name, expiry, err := getCertInfo(cert)

	if err != nil {
		return CertInfo{
			Name:   "",
			Expiry: "",
			Error:  err.Error(),
		}
	}

	return CertInfo{
		Name:   name,
		Expiry: expiry,
		Error:  "",
	}
}

func getCertInfo(certPEM string) (string, string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", "", errors.New("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", err
	}

	name := cert.Subject.CommonName
	expiry := cert.NotAfter.Format("2006-01-02 15:04:05")

	return name, expiry, nil
}
