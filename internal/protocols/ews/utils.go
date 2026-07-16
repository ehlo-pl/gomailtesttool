package ews

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"
)

// parseFlexibleTime parses RFC3339 or the PowerShell sortable timestamp format
// (assumed UTC when no timezone is given).
func parseFlexibleTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("time string is empty")
	}

	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}

	t, err = time.Parse("2006-01-02T15:04:05", timeStr)
	if err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339 like '2026-01-15T14:00:00Z' or PowerShell sortable like '2026-01-15T14:00:00')")
}

// ewsDateTime formats a time as the xs:dateTime UTC form EWS expects.
func ewsDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
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
