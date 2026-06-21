//go:build !integration
// +build !integration

package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// makeSelfSignedCert builds a self-signed leaf certificate for testing the
// certificate analysis helpers without needing a live TLS handshake.
func makeSelfSignedCert(t *testing.T, cn string, dnsNames []string, notBefore, notAfter time.Time, keyBits int) *x509.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		Issuer:       pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		DNSNames:     dnsNames,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment |
			x509.KeyUsageCertSign | x509.KeyUsageDataEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageEmailProtection,
		},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func TestAnalyzeCertificateChain_Empty(t *testing.T) {
	info := AnalyzeCertificateChain(nil, "example.com")
	if info == nil {
		t.Fatal("expected non-nil info for empty chain")
	}
	if info.ChainLength != 0 {
		t.Errorf("ChainLength = %d, want 0", info.ChainLength)
	}
}

func TestAnalyzeCertificateChain_Valid(t *testing.T) {
	now := time.Now()
	cert := makeSelfSignedCert(t, "example.com", []string{"example.com", "www.example.com"},
		now.Add(-time.Hour), now.Add(365*24*time.Hour), 2048)

	info := AnalyzeCertificateChain([]*x509.Certificate{cert}, "example.com")

	if info.IsExpired {
		t.Error("certificate should not be expired")
	}
	if !info.IsSelfSigned {
		t.Error("certificate should be detected as self-signed")
	}
	if info.PublicKeySize != 2048 {
		t.Errorf("PublicKeySize = %d, want 2048", info.PublicKeySize)
	}
	if len(info.SANs) != 2 {
		t.Errorf("SANs = %v, want 2 entries", info.SANs)
	}
	if info.ChainLength != 1 {
		t.Errorf("ChainLength = %d, want 1", info.ChainLength)
	}
}

func TestAnalyzeCertificateChain_Expired(t *testing.T) {
	now := time.Now()
	cert := makeSelfSignedCert(t, "old.example.com", []string{"old.example.com"},
		now.Add(-48*time.Hour), now.Add(-24*time.Hour), 2048)

	info := AnalyzeCertificateChain([]*x509.Certificate{cert}, "old.example.com")
	if !info.IsExpired {
		t.Error("certificate should be detected as expired")
	}

	// Expired + self-signed should surface warnings via the shared helper.
	warnings := CheckTLSWarnings(&TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}, info, false)
	if !containsSubstr(warnings, "expired") {
		t.Errorf("expected expiry warning, got %v", warnings)
	}
	if !containsSubstr(warnings, "Self-signed") {
		t.Errorf("expected self-signed warning, got %v", warnings)
	}
}

func TestCheckTLSWarnings_ExpiringSoon(t *testing.T) {
	now := time.Now()
	cert := makeSelfSignedCert(t, "soon.example.com", []string{"soon.example.com"},
		now.Add(-time.Hour), now.Add(10*24*time.Hour), 2048)

	info := AnalyzeCertificateChain([]*x509.Certificate{cert}, "soon.example.com")
	warnings := CheckTLSWarnings(&TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}, info, false)
	if !containsSubstr(warnings, "expires soon") {
		t.Errorf("expected 'expires soon' warning, got %v", warnings)
	}
}

func TestCheckTLSWarnings_LongValidity(t *testing.T) {
	now := time.Now()
	cert := makeSelfSignedCert(t, "long.example.com", []string{"long.example.com"},
		now.Add(-time.Hour), now.Add(500*24*time.Hour), 2048)

	info := AnalyzeCertificateChain([]*x509.Certificate{cert}, "long.example.com")
	warnings := CheckTLSWarnings(&TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}, info, false)
	if !containsSubstr(warnings, "exceeds 398 days") {
		t.Errorf("expected long-validity warning, got %v", warnings)
	}
}

func TestAnalyzeCertificateChain_HostnameMismatch(t *testing.T) {
	now := time.Now()
	cert := makeSelfSignedCert(t, "example.com", []string{"example.com"},
		now.Add(-time.Hour), now.Add(24*time.Hour), 2048)

	info := AnalyzeCertificateChain([]*x509.Certificate{cert}, "wrong-host.com")
	if info.VerificationStatus != "hostname_mismatch" {
		t.Errorf("VerificationStatus = %q, want hostname_mismatch", info.VerificationStatus)
	}

	warnings := CheckTLSWarnings(&TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}, info, false)
	if !containsSubstr(warnings, "hostname does not match") {
		t.Errorf("expected hostname mismatch warning, got %v", warnings)
	}
}

func TestAnalyzeCertificateChain_WeakKey(t *testing.T) {
	now := time.Now()
	cert := makeSelfSignedCert(t, "weak.example.com", []string{"weak.example.com"},
		now.Add(-time.Hour), now.Add(24*time.Hour), 1024)

	info := AnalyzeCertificateChain([]*x509.Certificate{cert}, "weak.example.com")
	if info.PublicKeySize != 1024 {
		t.Errorf("PublicKeySize = %d, want 1024", info.PublicKeySize)
	}

	warnings := CheckTLSWarnings(&TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}, info, false)
	if !containsSubstr(warnings, "Weak public key") {
		t.Errorf("expected weak key warning, got %v", warnings)
	}
}
