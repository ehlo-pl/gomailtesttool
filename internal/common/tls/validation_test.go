//go:build !integration
// +build !integration

package tls

import (
	"crypto/tls"
	"strings"
	"testing"
)

// TestParseTLSVersion covers the consolidated string->constant parser that all
// protocol clients (SMTP, IMAP, POP3, EWS) now share.
func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  uint16
	}{
		{"TLS 1.0", "1.0", tls.VersionTLS10},
		{"TLS 1.1", "1.1", tls.VersionTLS11},
		{"TLS 1.2", "1.2", tls.VersionTLS12},
		{"TLS 1.3", "1.3", tls.VersionTLS13},
		{"Whitespace trimmed", "  1.3  ", tls.VersionTLS13},
		{"Empty defaults to 1.2", "", tls.VersionTLS12},
		{"Unknown defaults to 1.2", "9.9", tls.VersionTLS12},
		{"Garbage defaults to 1.2", "tls1.2", tls.VersionTLS12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseTLSVersion(tt.input); got != tt.want {
				t.Errorf("ParseTLSVersion(%q) = 0x%04X, want 0x%04X", tt.input, got, tt.want)
			}
		})
	}
}

// TestTLSVersionString covers the consolidated constant->display formatter.
func TestTLSVersionString(t *testing.T) {
	tests := []struct {
		name  string
		input uint16
		want  string
	}{
		{"TLS 1.0", tls.VersionTLS10, "TLS 1.0"},
		{"TLS 1.1", tls.VersionTLS11, "TLS 1.1"},
		{"TLS 1.2", tls.VersionTLS12, "TLS 1.2"},
		{"TLS 1.3", tls.VersionTLS13, "TLS 1.3"},
		{"SSL 3.0", 0x0300, "SSL 3.0"},
		{"Unknown", 0x9999, "Unknown (0x9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TLSVersionString(tt.input); got != tt.want {
				t.Errorf("TLSVersionString(0x%04X) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseTLSVersionRoundTrip ensures parse and format agree for the
// supported versions.
func TestParseTLSVersionRoundTrip(t *testing.T) {
	for _, v := range []string{"1.0", "1.1", "1.2", "1.3"} {
		if got := TLSVersionString(ParseTLSVersion(v)); got != "TLS "+v {
			t.Errorf("round trip %q = %q, want %q", v, got, "TLS "+v)
		}
	}
}

func TestAnalyzeCipherStrength(t *testing.T) {
	tests := []struct {
		name   string
		cipher uint16
		want   string
	}{
		{"AES-GCM is strong", tls.TLS_AES_128_GCM_SHA256, "strong"},
		{"ChaCha20-Poly1305 is strong", tls.TLS_CHACHA20_POLY1305_SHA256, "strong"},
		{"ECDHE-GCM is strong", tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, "strong"},
		{"CBC w/o SHA256 is weak", tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, "weak"},
		{"RC4 is deprecated", tls.TLS_RSA_WITH_RC4_128_SHA, "deprecated"},
		{"3DES is deprecated", tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA, "deprecated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnalyzeCipherStrength(tt.cipher); got != tt.want {
				t.Errorf("AnalyzeCipherStrength(%s) = %q, want %q",
					tls.CipherSuiteName(tt.cipher), got, tt.want)
			}
		})
	}
}

func TestCheckTLSWarnings(t *testing.T) {
	t.Run("deprecated version and weak cipher warn", func(t *testing.T) {
		info := &TLSInfo{Version: "TLS 1.0", CipherSuiteStrength: "weak", CipherSuite: "X"}
		warnings := CheckTLSWarnings(info, nil, false)
		if !containsSubstr(warnings, "Deprecated TLS version") {
			t.Errorf("expected deprecated TLS version warning, got %v", warnings)
		}
		if !containsSubstr(warnings, "Weak cipher suite") {
			t.Errorf("expected weak cipher warning, got %v", warnings)
		}
	})

	t.Run("SSL 3.0 warns", func(t *testing.T) {
		info := &TLSInfo{Version: "SSL 3.0", CipherSuiteStrength: "strong"}
		if !containsSubstr(CheckTLSWarnings(info, nil, false), "SSL 3.0") {
			t.Error("expected SSL 3.0 warning")
		}
	})

	t.Run("skipverify warns", func(t *testing.T) {
		info := &TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}
		if !containsSubstr(CheckTLSWarnings(info, nil, true), "verification disabled") {
			t.Error("expected skipverify warning")
		}
	})

	t.Run("clean config has no warnings", func(t *testing.T) {
		info := &TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}
		if w := CheckTLSWarnings(info, nil, false); len(w) != 0 {
			t.Errorf("expected no warnings, got %v", w)
		}
	})
}

func TestGetTLSRecommendations(t *testing.T) {
	t.Run("old version and weak cipher recommend upgrades", func(t *testing.T) {
		info := &TLSInfo{Version: "TLS 1.0", CipherSuiteStrength: "weak"}
		recs := GetTLSRecommendations(info)
		if !containsSubstr(recs, "Upgrade to TLS 1.2 or 1.3") {
			t.Errorf("expected version upgrade recommendation, got %v", recs)
		}
		if !containsSubstr(recs, "AEAD cipher") {
			t.Errorf("expected cipher recommendation, got %v", recs)
		}
	})

	t.Run("modern config has no recommendations", func(t *testing.T) {
		info := &TLSInfo{Version: "TLS 1.3", CipherSuiteStrength: "strong"}
		if r := GetTLSRecommendations(info); len(r) != 0 {
			t.Errorf("expected no recommendations, got %v", r)
		}
	})
}

func TestAnalyzeTLSConnection(t *testing.T) {
	state := &tls.ConnectionState{
		Version:            tls.VersionTLS13,
		CipherSuite:        tls.TLS_AES_128_GCM_SHA256,
		ServerName:         "mail.example.com",
		NegotiatedProtocol: "h2",
	}

	info := AnalyzeTLSConnection(state)
	if info.Version != "TLS 1.3" {
		t.Errorf("Version = %q, want TLS 1.3", info.Version)
	}
	if info.CipherSuiteStrength != "strong" {
		t.Errorf("CipherSuiteStrength = %q, want strong", info.CipherSuiteStrength)
	}
	if info.ServerName != "mail.example.com" {
		t.Errorf("ServerName = %q, want mail.example.com", info.ServerName)
	}
	if info.NegotiatedProtocol != "h2" {
		t.Errorf("NegotiatedProtocol = %q, want h2", info.NegotiatedProtocol)
	}
}

func containsSubstr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
