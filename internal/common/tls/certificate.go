package tls

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"strings"
	"time"
)

// CertificateInfo holds detailed information about a certificate.
type CertificateInfo struct {
	Subject             string    // Certificate subject (CN, O, OU, etc.)
	Issuer              string    // Certificate issuer
	SerialNumber        string    // Serial number (hex format)
	ValidFrom           time.Time // Not valid before date
	ValidTo             time.Time // Not valid after date
	SANs                []string  // Subject Alternative Names
	KeyUsage            []string  // Key usage extensions
	ExtKeyUsage         []string  // Extended key usage
	IsCA                bool      // Is this a CA certificate?
	SignatureAlgorithm  string    // Signature algorithm
	PublicKeyAlgorithm  string    // Public key algorithm
	PublicKeySize       int       // Public key size in bits
	VerificationStatus  string    // Verification result
	ChainLength         int       // Length of certificate chain
	DaysUntilExpiry     int       // Days until expiration (negative if expired)
	IsExpired           bool      // Certificate has expired
	IsSelfSigned        bool      // Certificate is self-signed
}

// AnalyzeCertificateChain analyzes an entire certificate chain and returns detailed information.
// The first certificate in the chain should be the leaf (server) certificate.
// Returns information about the leaf certificate and chain details.
func AnalyzeCertificateChain(certs []*x509.Certificate, hostname string) *CertificateInfo {
	if len(certs) == 0 {
		return &CertificateInfo{
			VerificationStatus: "no_certificates",
		}
	}

	// Analyze the leaf certificate (first in chain)
	leafCert := certs[0]

	// Extract Subject Alternative Names
	sans := extractSANs(leafCert)

	// Extract key usage
	keyUsage := extractKeyUsage(leafCert)
	extKeyUsage := extractExtKeyUsage(leafCert)

	// Check if self-signed
	isSelfSigned := leafCert.Subject.String() == leafCert.Issuer.String()

	// Verify hostname
	verificationStatus := verifyHostname(leafCert, hostname)

	// Check expiration
	now := time.Now()
	isExpired := now.After(leafCert.NotAfter)
	daysUntilExpiry := int(time.Until(leafCert.NotAfter).Hours() / 24)

	// If already verified hostname, check expiration
	if verificationStatus == "valid" && isExpired {
		verificationStatus = "expired"
	}

	// If self-signed, note it
	if verificationStatus == "valid" && isSelfSigned {
		verificationStatus = "self_signed"
	}

	// Determine public key size
	publicKeySize := getPublicKeySize(leafCert)

	return &CertificateInfo{
		Subject:             leafCert.Subject.String(),
		Issuer:              leafCert.Issuer.String(),
		SerialNumber:        fmt.Sprintf("%X", leafCert.SerialNumber),
		ValidFrom:           leafCert.NotBefore,
		ValidTo:             leafCert.NotAfter,
		SANs:                sans,
		KeyUsage:            keyUsage,
		ExtKeyUsage:         extKeyUsage,
		IsCA:                leafCert.IsCA,
		SignatureAlgorithm:  leafCert.SignatureAlgorithm.String(),
		PublicKeyAlgorithm:  leafCert.PublicKeyAlgorithm.String(),
		PublicKeySize:       publicKeySize,
		VerificationStatus:  verificationStatus,
		ChainLength:         len(certs),
		DaysUntilExpiry:     daysUntilExpiry,
		IsExpired:           isExpired,
		IsSelfSigned:        isSelfSigned,
	}
}

// extractSANs extracts Subject Alternative Names from a certificate.
func extractSANs(cert *x509.Certificate) []string {
	var sans []string

	// DNS names
	sans = append(sans, cert.DNSNames...)

	// IP addresses
	for _, ip := range cert.IPAddresses {
		sans = append(sans, ip.String())
	}

	// Email addresses
	sans = append(sans, cert.EmailAddresses...)

	// URIs
	for _, uri := range cert.URIs {
		sans = append(sans, uri.String())
	}

	return sans
}

// extractKeyUsage extracts key usage flags as human-readable strings.
func extractKeyUsage(cert *x509.Certificate) []string {
	var usage []string

	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		usage = append(usage, "DigitalSignature")
	}
	if cert.KeyUsage&x509.KeyUsageContentCommitment != 0 {
		usage = append(usage, "ContentCommitment")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		usage = append(usage, "KeyEncipherment")
	}
	if cert.KeyUsage&x509.KeyUsageDataEncipherment != 0 {
		usage = append(usage, "DataEncipherment")
	}
	if cert.KeyUsage&x509.KeyUsageKeyAgreement != 0 {
		usage = append(usage, "KeyAgreement")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		usage = append(usage, "CertSign")
	}
	if cert.KeyUsage&x509.KeyUsageCRLSign != 0 {
		usage = append(usage, "CRLSign")
	}
	if cert.KeyUsage&x509.KeyUsageEncipherOnly != 0 {
		usage = append(usage, "EncipherOnly")
	}
	if cert.KeyUsage&x509.KeyUsageDecipherOnly != 0 {
		usage = append(usage, "DecipherOnly")
	}

	return usage
}

// extractExtKeyUsage extracts extended key usage as human-readable strings.
func extractExtKeyUsage(cert *x509.Certificate) []string {
	var usage []string

	for _, eku := range cert.ExtKeyUsage {
		switch eku {
		case x509.ExtKeyUsageAny:
			usage = append(usage, "Any")
		case x509.ExtKeyUsageServerAuth:
			usage = append(usage, "ServerAuth")
		case x509.ExtKeyUsageClientAuth:
			usage = append(usage, "ClientAuth")
		case x509.ExtKeyUsageCodeSigning:
			usage = append(usage, "CodeSigning")
		case x509.ExtKeyUsageEmailProtection:
			usage = append(usage, "EmailProtection")
		case x509.ExtKeyUsageIPSECEndSystem:
			usage = append(usage, "IPSECEndSystem")
		case x509.ExtKeyUsageIPSECTunnel:
			usage = append(usage, "IPSECTunnel")
		case x509.ExtKeyUsageIPSECUser:
			usage = append(usage, "IPSECUser")
		case x509.ExtKeyUsageTimeStamping:
			usage = append(usage, "TimeStamping")
		case x509.ExtKeyUsageOCSPSigning:
			usage = append(usage, "OCSPSigning")
		case x509.ExtKeyUsageMicrosoftServerGatedCrypto:
			usage = append(usage, "MicrosoftServerGatedCrypto")
		case x509.ExtKeyUsageNetscapeServerGatedCrypto:
			usage = append(usage, "NetscapeServerGatedCrypto")
		case x509.ExtKeyUsageMicrosoftCommercialCodeSigning:
			usage = append(usage, "MicrosoftCommercialCodeSigning")
		case x509.ExtKeyUsageMicrosoftKernelCodeSigning:
			usage = append(usage, "MicrosoftKernelCodeSigning")
		}
	}

	return usage
}

// verifyHostname checks if the certificate is valid for the given hostname.
func verifyHostname(cert *x509.Certificate, hostname string) string {
	// Try to verify hostname
	err := cert.VerifyHostname(hostname)
	if err != nil {
		// Check if it's a hostname mismatch
		if strings.Contains(err.Error(), "doesn't match") ||
		   strings.Contains(err.Error(), "certificate is valid for") {
			return "hostname_mismatch"
		}
		return "invalid"
	}

	return "valid"
}

// getPublicKeySize determines the size of the public key in bits.
func getPublicKeySize(cert *x509.Certificate) int {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return pub.N.BitLen()
	case *ecdsa.PublicKey:
		return pub.Curve.Params().BitSize
	case *ed25519.PublicKey:
		return 256 // Ed25519 is always 256 bits
	default:
		return 0
	}
}
