package tls

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// TLSInfo holds information about a TLS connection.
type TLSInfo struct {
	Version            string // TLS version (e.g., "TLS 1.3")
	CipherSuite        string // Cipher suite name
	CipherSuiteStrength string // "strong", "weak", or "deprecated"
	ServerName         string // SNI server name
	NegotiatedProtocol string // ALPN negotiated protocol (if any)
}

// AnalyzeTLSConnection extracts and analyzes TLS connection details.
func AnalyzeTLSConnection(state *tls.ConnectionState) *TLSInfo {
	return &TLSInfo{
		Version:            TLSVersionString(state.Version),
		CipherSuite:        tls.CipherSuiteName(state.CipherSuite),
		CipherSuiteStrength: AnalyzeCipherStrength(state.CipherSuite),
		ServerName:         state.ServerName,
		NegotiatedProtocol: state.NegotiatedProtocol,
	}
}

// TLSVersionString converts a TLS version constant to a human-readable string.
func TLSVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	case 0x0300: // SSL 3.0 - use raw value instead of deprecated tls.VersionSSL30
		return "SSL 3.0"
	default:
		return fmt.Sprintf("Unknown (0x%04X)", version)
	}
}

// ParseTLSVersion converts a version string (e.g., "1.2", "1.3") to a tls constant.
func ParseTLSVersion(versionStr string) uint16 {
	switch strings.TrimSpace(versionStr) {
	case "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12 // Default to TLS 1.2
	}
}

// AnalyzeCipherStrength categorizes a cipher suite by security strength.
// Returns "strong", "weak", or "deprecated".
func AnalyzeCipherStrength(cipherSuite uint16) string {
	cipherName := tls.CipherSuiteName(cipherSuite)
	cipherLower := strings.ToLower(cipherName)

	// Deprecated/weak indicators
	if strings.Contains(cipherLower, "rc4") ||
		strings.Contains(cipherLower, "3des") ||
		strings.Contains(cipherLower, "des") ||
		strings.Contains(cipherLower, "export") ||
		strings.Contains(cipherLower, "null") ||
		strings.Contains(cipherLower, "anon") {
		return "deprecated"
	}

	// Weak indicators
	if strings.Contains(cipherLower, "cbc") &&
		!strings.Contains(cipherLower, "sha256") &&
		!strings.Contains(cipherLower, "sha384") {
		return "weak"
	}

	// Strong modern ciphers
	if strings.Contains(cipherLower, "gcm") ||
		strings.Contains(cipherLower, "chacha20") ||
		strings.Contains(cipherLower, "poly1305") ||
		strings.Contains(cipherLower, "ccm") {
		return "strong"
	}

	// Default to weak if we don't recognize it
	return "weak"
}

// CheckTLSWarnings generates a list of warnings based on TLS info and certificate info.
func CheckTLSWarnings(tlsInfo *TLSInfo, certInfo *CertificateInfo, skipVerify bool) []string {
	var warnings []string

	// TLS version warnings
	if tlsInfo.Version == "TLS 1.0" || tlsInfo.Version == "TLS 1.1" {
		warnings = append(warnings, fmt.Sprintf("Deprecated TLS version: %s (upgrade to TLS 1.2+ recommended)", tlsInfo.Version))
	}
	if tlsInfo.Version == "SSL 3.0" {
		warnings = append(warnings, "SSL 3.0 is critically insecure and should not be used")
	}

	// Cipher suite warnings
	if tlsInfo.CipherSuiteStrength == "deprecated" {
		warnings = append(warnings, fmt.Sprintf("Deprecated cipher suite: %s", tlsInfo.CipherSuite))
	} else if tlsInfo.CipherSuiteStrength == "weak" {
		warnings = append(warnings, fmt.Sprintf("Weak cipher suite: %s (consider upgrading server configuration)", tlsInfo.CipherSuite))
	}

	// Certificate warnings
	if certInfo != nil {
		// Expiration warnings
		if certInfo.IsExpired {
			warnings = append(warnings, fmt.Sprintf("Certificate expired on %s", certInfo.ValidTo.Format("2006-01-02")))
		} else if certInfo.DaysUntilExpiry < 30 && certInfo.DaysUntilExpiry >= 0 {
			warnings = append(warnings, fmt.Sprintf("Certificate expires soon (%d days remaining on %s)",
				certInfo.DaysUntilExpiry, certInfo.ValidTo.Format("2006-01-02")))
		}

		// Hostname mismatch
		if certInfo.VerificationStatus == "hostname_mismatch" {
			warnings = append(warnings, "Certificate hostname does not match server hostname")
		}

		// Self-signed
		if certInfo.IsSelfSigned {
			warnings = append(warnings, "Self-signed certificate (not trusted by default)")
		}

		// Weak public key
		if certInfo.PublicKeySize < 2048 && certInfo.PublicKeySize > 0 {
			warnings = append(warnings, fmt.Sprintf("Weak public key size: %d bits (2048+ recommended)", certInfo.PublicKeySize))
		}

		// Short validity period (potentially suspicious)
		validityDays := int(certInfo.ValidTo.Sub(certInfo.ValidFrom).Hours() / 24)
		if validityDays > 398 {
			warnings = append(warnings, fmt.Sprintf("Certificate validity period exceeds 398 days (%d days) - may not be trusted by modern browsers", validityDays))
		}
	}

	// Skip verify warning
	if skipVerify {
		warnings = append(warnings, "Certificate verification disabled (-skipverify flag) - connection is not secure")
	}

	return warnings
}

// GetTLSRecommendations generates recommendations based on the TLS configuration.
func GetTLSRecommendations(tlsInfo *TLSInfo) []string {
	var recommendations []string

	// Version recommendations
	if tlsInfo.Version != "TLS 1.3" && tlsInfo.Version != "TLS 1.2" {
		recommendations = append(recommendations, "Upgrade to TLS 1.2 or 1.3 for better security")
	}

	// Cipher recommendations
	if tlsInfo.CipherSuiteStrength != "strong" {
		recommendations = append(recommendations, "Configure server to prefer modern AEAD cipher suites (GCM, ChaCha20-Poly1305)")
	}

	return recommendations
}
