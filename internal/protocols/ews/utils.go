package ews

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
)

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
