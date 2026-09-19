package gatesentryWebserverEndpoints

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

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

func GetCertificateBytes(settings *gatesentry2storage.MapStore) ([]byte, error) {
	cert, err := settings.GetE(CERTIFICATE_KEY)
	return []byte(cert), err
}

func GetCertificateInfo(settings *gatesentry2storage.MapStore) (interface{}, error) {
	cert, err := settings.GetE(CERTIFICATE_KEY)
	if err != nil {
		return nil, err
	}
	name, expiry, err := getCertInfo(cert)

	if err != nil {
		return CertInfo{
			Name:   "",
			Expiry: "",
			Error:  err.Error(),
		}, nil
	}

	return CertInfo{
		Name:   name,
		Expiry: expiry,
		Error:  "",
	}, nil
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

// CertDetail is the parsed certificate metadata used by the HTTPS inspection
// status endpoint and the diagnostics check. It carries no private-key
// material and no secrets.
type CertDetail struct {
	Name      string `json:"name"`
	NotBefore string `json:"not_before"`
	NotAfter  string `json:"not_after"`
	Expired   bool   `json:"expired"`
}

// ErrCertUnavailable reports that no CA certificate is configured. Callers
// use it to tell "not configured" apart from "configured but unparseable".
var ErrCertUnavailable = errors.New("certificate is not available")

// ParseCertDetail decodes and parses the PEM-encoded CA certificate stored
// under CERTIFICATE_KEY. It returns ErrCertUnavailable when the setting is
// absent or empty, so callers can distinguish "not configured" from
// "present but invalid".
func ParseCertDetail(settings *gatesentry2storage.MapStore) (CertDetail, error) {
	cert, err := settings.GetE(CERTIFICATE_KEY)
	if err != nil || cert == "" {
		return CertDetail{}, ErrCertUnavailable
	}
	block, _ := pem.Decode([]byte(cert))
	if block == nil {
		return CertDetail{}, errors.New("failed to decode PEM block")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return CertDetail{}, err
	}
	now := time.Now().UTC()
	return CertDetail{
		Name:      parsed.Subject.CommonName,
		NotBefore: parsed.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:  parsed.NotAfter.UTC().Format(time.RFC3339),
		Expired:   !now.Before(parsed.NotAfter),
	}, nil
}
