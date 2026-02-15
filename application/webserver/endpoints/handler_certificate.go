package gatesentryWebserverEndpoints

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	"bitbucket.org/abdullah_irfan/gatesentryproxy"
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

// GenerateCACertificate creates a new self-signed CA certificate and private key.
// Returns PEM-encoded cert and key strings.
func GenerateCACertificate(commonName string, validYears int) (certPEM string, keyPEM string, err error) {
	// Generate RSA 4096-bit private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"GateSentry"},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(validYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// PEM-encode the certificate
	certBuf := &pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM = string(pem.EncodeToMemory(certBuf))

	// PEM-encode the private key (PKCS8 format)
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyBuf := &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}
	keyPEM = string(pem.EncodeToMemory(keyBuf))

	return certPEM, keyPEM, nil
}

// GSApiCertificateGenerate generates a new CA cert/key, saves to settings,
// reloads the proxy, and returns the new cert info.
func GSApiCertificateGenerate(settings *gatesentry2storage.MapStore) interface{} {
	type GenerateResult struct {
		Success bool     `json:"success"`
		Info    CertInfo `json:"info"`
		Error   string   `json:"error"`
	}

	certPEM, keyPEM, err := GenerateCACertificate("GateSentry CA", 10)
	if err != nil {
		log.Println("[Certificate] Error generating CA:", err)
		return GenerateResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	// Save to settings
	settings.Update(CERTIFICATE_KEY, certPEM)
	settings.Update("keypem", keyPEM)

	// Reload the proxy certificate
	ReloadProxyCertificate(settings)

	// Return the new cert info
	name, expiry, err := getCertInfo(certPEM)
	if err != nil {
		return GenerateResult{
			Success: true,
			Info: CertInfo{
				Error: "Certificate generated but could not parse info: " + err.Error(),
			},
		}
	}

	log.Printf("[Certificate] Generated new CA: CN=%s, expires %s", name, expiry)

	return GenerateResult{
		Success: true,
		Info: CertInfo{
			Name:   name,
			Expiry: expiry,
		},
	}
}

// ReloadProxyCertificate reads the current cert/key from settings and
// reinitializes the proxy TLS with them.
func ReloadProxyCertificate(settings *gatesentry2storage.MapStore) {
	capem := []byte(settings.Get(CERTIFICATE_KEY))
	keypem := []byte(settings.Get("keypem"))
	gatesentryproxy.InitWithDataCerts(capem, keypem)
	log.Println("[Certificate] Proxy certificate reloaded")
}

// EnsureCACertificate checks if a CA certificate exists in settings.
// If not, it generates a unique one for this installation.
// Returns true if a new cert was generated.
func EnsureCACertificate(settings *gatesentry2storage.MapStore) bool {
	existing := settings.Get(CERTIFICATE_KEY)
	if existing != "" {
		return false
	}

	log.Println("[Certificate] No CA certificate found, generating a new one...")
	certPEM, keyPEM, err := GenerateCACertificate("GateSentry CA", 10)
	if err != nil {
		log.Println("[Certificate] ERROR: Failed to generate CA certificate:", err)
		return false
	}

	settings.Update(CERTIFICATE_KEY, certPEM)
	settings.Update("keypem", keyPEM)
	log.Println("[Certificate] New CA certificate generated and saved")
	return true
}
