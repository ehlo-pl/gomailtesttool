package ews

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
)

// tlsVersionString converts a TLS version constant to a display string.
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		if version == 0 {
			return ""
		}
		return fmt.Sprintf("TLS 0x%04x", version)
	}
}

// tlsCipherName returns the cipher suite name, or hex if unknown.
func tlsCipherName(id uint16) string {
	name := tls.CipherSuiteName(id)
	if name == "" {
		return fmt.Sprintf("0x%04x", id)
	}
	return name
}

// certCSVFields extracts cert fields for CSV from the first cert in a chain.
// Returns subject, issuer, SANs, validFrom, validTo.
func certCSVFields(certs []*x509.Certificate) (subject, issuer, sans, validFrom, validTo string) {
	if len(certs) == 0 {
		return "", "", "", "", ""
	}
	cert := certs[0]
	return cert.Subject.CommonName,
		cert.Issuer.CommonName,
		strings.Join(cert.DNSNames, ";"),
		cert.NotBefore.Format("2006-01-02"),
		cert.NotAfter.Format("2006-01-02")
}
