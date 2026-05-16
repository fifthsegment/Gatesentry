package gatesentryproxy

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"time"
)

// createBlockPageTLSConfig generates a TLS config with a certificate for the
// given hostname, signed by the GateSentry CA. This ensures browsers that
// trust the CA will accept the block page without certificate errors.
// Falls back to the raw CA cert if the CA is not loaded.
func createBlockPageTLSConfig(host string) (*tls.Config, error) {
	// Strip port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// If the CA cert isn't loaded, fall back to using TLSCert directly
	if ParsedTLSCert == nil {
		return &tls.Config{
			Certificates: []tls.Certificate{TLSCert},
		}, nil
	}

	// Generate a certificate for this specific host, signed by the GateSentry CA
	template := &x509.Certificate{
		SerialNumber: big.NewInt(0).SetBytes([]byte(host)),
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"GateSentry Block Page"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Set SAN: IP or DNS name
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
		// Also add wildcard if it's a simple hostname
		if strings.Count(host, ".") >= 1 {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	// Sign with the GateSentry CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, ParsedTLSCert, ParsedTLSCert.PublicKey, TLSCert.PrivateKey)
	if err != nil {
		// Fall back to raw CA cert on error
		return &tls.Config{
			Certificates: []tls.Certificate{TLSCert},
		}, nil
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  TLSCert.PrivateKey,
	}
	// Append CA cert to chain
	cert.Certificate = append(cert.Certificate, TLSCert.Certificate...)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}
