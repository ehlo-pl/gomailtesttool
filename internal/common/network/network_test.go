package network

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestValidateIPVersionFlags(t *testing.T) {
	if err := ValidateIPVersionFlags(false, false); err != nil {
		t.Errorf("neither flag set: unexpected error: %v", err)
	}
	if err := ValidateIPVersionFlags(true, false); err != nil {
		t.Errorf("--ipv4 only: unexpected error: %v", err)
	}
	if err := ValidateIPVersionFlags(false, true); err != nil {
		t.Errorf("--ipv6 only: unexpected error: %v", err)
	}
	if err := ValidateIPVersionFlags(true, true); err == nil {
		t.Error("both flags set: expected error, got nil")
	}
}

func TestResolveForDial_NoFlags(t *testing.T) {
	host, err := ResolveForDial(context.Background(), "example.com", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "example.com" {
		t.Errorf("host = %q, want unchanged %q", host, "example.com")
	}
}

func TestResolveForDial_LiteralIP(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		ipv4    bool
		ipv6    bool
		want    string
		wantErr bool
	}{
		{"IPv4 literal with --ipv4", "192.0.2.1", true, false, "192.0.2.1", false},
		{"IPv6 literal with --ipv6", "2001:db8::1", false, true, "2001:db8::1", false},
		{"IPv4 literal with --ipv6 mismatches", "192.0.2.1", false, true, "", true},
		{"IPv6 literal with --ipv4 mismatches", "2001:db8::1", true, false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveForDial(context.Background(), tt.host, tt.ipv4, tt.ipv6)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveForDial_Hostname(t *testing.T) {
	// "localhost" resolves to both 127.0.0.1 and ::1 in most environments.
	v4, err := ResolveForDial(context.Background(), "localhost", true, false)
	if err != nil {
		t.Fatalf("--ipv4 resolve: unexpected error: %v", err)
	}
	if ip := net.ParseIP(v4); ip == nil || ip.To4() == nil {
		t.Errorf("--ipv4 resolved to non-IPv4 address: %q", v4)
	}

	v6, err := ResolveForDial(context.Background(), "localhost", false, true)
	if err != nil {
		t.Fatalf("--ipv6 resolve: unexpected error: %v", err)
	}
	if ip := net.ParseIP(v6); ip == nil || ip.To4() != nil {
		t.Errorf("--ipv6 resolved to non-IPv6 address: %q", v6)
	}
}

func TestResolveForDial_UnknownHost(t *testing.T) {
	_, err := ResolveForDial(context.Background(), "this-host-does-not-exist.invalid", true, false)
	if err == nil {
		t.Fatal("expected error for unresolvable host, got nil")
	}
	if !strings.Contains(err.Error(), "IPv4") {
		t.Errorf("error should mention IPv4: %v", err)
	}
}

func TestLookupMX_KnownDomain(t *testing.T) {
	host, err := LookupMX(context.Background(), "gmail.com")
	if err != nil {
		t.Fatalf("LookupMX() error = %v", err)
	}
	if host == "" {
		t.Fatal("LookupMX() returned empty host")
	}
	if strings.HasSuffix(host, ".") {
		t.Errorf("LookupMX() should strip trailing dot, got %q", host)
	}
	if !strings.Contains(host, "google.com") && !strings.Contains(host, "gmail") {
		t.Errorf("LookupMX(gmail.com) = %q, expected a Google mail server", host)
	}
}

func TestLookupMX_NoRecords(t *testing.T) {
	_, err := LookupMX(context.Background(), "this-domain-does-not-exist.invalid")
	if err == nil {
		t.Fatal("expected error for domain with no MX records, got nil")
	}
}
