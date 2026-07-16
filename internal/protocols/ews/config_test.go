package ews

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestResolveAuthMethod(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   string
	}{
		{
			name: "bearer when access token exists",
			config: &Config{
				AccessToken: "token",
			},
			want: "Bearer",
		},
		{
			name: "ntlm when username has domain prefix",
			config: &Config{
				Username: `CORP\user`,
			},
			want: "NTLM",
		},
		{
			name: "ntlm when domain is explicitly set",
			config: &Config{
				Username: "user",
				Domain:   "CORP",
			},
			want: "NTLM",
		},
		{
			name: "basic fallback",
			config: &Config{
				Username: "user@example.com",
			},
			want: "Basic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAuthMethod(tt.config)
			if got != tt.want {
				t.Fatalf("resolveAuthMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigFromViper(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		v := viper.New()

		cfg := ConfigFromViper(v)
		if cfg.Port != 443 {
			t.Fatalf("Port = %d, want 443", cfg.Port)
		}
		if cfg.Timeout != 30*time.Second {
			t.Fatalf("Timeout = %v, want 30s", cfg.Timeout)
		}
		if cfg.AuthMethod != "auto" {
			t.Fatalf("AuthMethod = %q, want auto", cfg.AuthMethod)
		}
		if cfg.TLSVersion != "1.2" {
			t.Fatalf("TLSVersion = %q, want 1.2", cfg.TLSVersion)
		}
		if cfg.EWSPath != "/EWS/Exchange.asmx" {
			t.Fatalf("EWSPath = %q", cfg.EWSPath)
		}
		if cfg.AutodiscoverPath != "/autodiscover/autodiscover.svc" {
			t.Fatalf("AutodiscoverPath = %q", cfg.AutodiscoverPath)
		}
	})

	t.Run("overrides and normalization", func(t *testing.T) {
		v := viper.New()
		v.Set("host", "mail.example.com")
		v.Set("port", 8443)
		v.Set("timeout", 45)
		v.Set("authmethod", "NTLM")
		v.Set("tlsversion", "1.3")
		v.Set("ewspath", "/custom/ews")
		v.Set("autodiscoverpath", "/custom/autodiscover")
		v.Set("username", "CORP\\user")
		v.Set("password", "pass")
		v.Set("accesstoken", "token")
		v.Set("domain", "CORP")
		v.Set("mailbox", "target@example.com")
		v.Set("skipverify", true)
		v.Set("proxy", "http://proxy.example.com:8080")
		v.Set("verbose", true)
		v.Set("loglevel", "debug")
		v.Set("logformat", "JSON")

		cfg := ConfigFromViper(v)
		if cfg.Host != "mail.example.com" || cfg.Port != 8443 {
			t.Fatalf("unexpected host/port: %s/%d", cfg.Host, cfg.Port)
		}
		if cfg.Timeout != 45*time.Second {
			t.Fatalf("Timeout = %v, want 45s", cfg.Timeout)
		}
		if cfg.AuthMethod != "NTLM" || cfg.TLSVersion != "1.3" {
			t.Fatalf("unexpected auth/tls: %s/%s", cfg.AuthMethod, cfg.TLSVersion)
		}
		if cfg.EWSPath != "/custom/ews" || cfg.AutodiscoverPath != "/custom/autodiscover" {
			t.Fatalf("unexpected paths: %s / %s", cfg.EWSPath, cfg.AutodiscoverPath)
		}
		if cfg.LogFormat != "json" {
			t.Fatalf("LogFormat = %q, want json", cfg.LogFormat)
		}
	})
}

func TestValidateConfiguration(t *testing.T) {
	base := NewConfig()
	base.Action = ActionTestAuth
	base.Host = "mail.example.com"
	base.Username = "CORP\\user"
	base.Password = "secret"

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{
			name: "valid testauth",
			mutate: func(c *Config) {
				c.Action = ActionTestAuth
			},
			wantErr: false,
		},
		{
			name: "invalid action",
			mutate: func(c *Config) {
				c.Action = "bad"
			},
			wantErr: true,
		},
		{
			name: "missing host",
			mutate: func(c *Config) {
				c.Host = ""
			},
			wantErr: true,
		},
		{
			name: "autodiscover requires email username",
			mutate: func(c *Config) {
				c.Action = ActionAutodiscover
				c.Username = "CORP\\user"
			},
			wantErr: true,
		},
		{
			name: "bearer auth requires access token",
			mutate: func(c *Config) {
				c.Action = ActionTestAuth
				c.AuthMethod = "Bearer"
				c.AccessToken = ""
				c.Password = ""
			},
			wantErr: true,
		},
		{
			name: "auto auth resolves to bearer",
			mutate: func(c *Config) {
				c.Action = ActionTestAuth
				c.AuthMethod = "auto"
				c.Username = "user@example.com"
				c.AccessToken = "token"
				c.Password = ""
			},
			wantErr: false,
		},
		{
			name: "getevents with defaults",
			mutate: func(c *Config) {
				c.Action = ActionGetEvents
			},
			wantErr: false,
		},
		{
			name: "getevents with invalid start time",
			mutate: func(c *Config) {
				c.Action = ActionGetEvents
				c.StartTime = "yesterday"
			},
			wantErr: true,
		},
		{
			name: "sendinvite requires to/start/end",
			mutate: func(c *Config) {
				c.Action = ActionSendInvite
			},
			wantErr: true,
		},
		{
			name: "sendinvite with to/start/end",
			mutate: func(c *Config) {
				c.Action = ActionSendInvite
				c.To = []string{"attendee@example.com"}
				c.StartTime = "2026-08-01T10:00:00Z"
				c.EndTime = "2026-08-01T11:00:00Z"
			},
			wantErr: false,
		},
		{
			name: "getschedule requires to",
			mutate: func(c *Config) {
				c.Action = ActionGetSchedule
			},
			wantErr: true,
		},
		{
			name: "getschedule rejects multiple recipients",
			mutate: func(c *Config) {
				c.Action = ActionGetSchedule
				c.To = []string{"a@example.com", "b@example.com"}
			},
			wantErr: true,
		},
		{
			name: "getschedule with single recipient",
			mutate: func(c *Config) {
				c.Action = ActionGetSchedule
				c.To = []string{"a@example.com"}
			},
			wantErr: false,
		},
		{
			name: "findtimeslot requires to",
			mutate: func(c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 30
			},
			wantErr: true,
		},
		{
			name: "findtimeslot rejects multiple recipients",
			mutate: func(c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 30
				c.To = []string{"a@example.com", "b@example.com"}
			},
			wantErr: true,
		},
		{
			name: "findtimeslot rejects duration out of range",
			mutate: func(c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 4
				c.To = []string{"a@example.com"}
			},
			wantErr: true,
		},
		{
			name: "findtimeslot with single recipient and valid duration",
			mutate: func(c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 30
				c.To = []string{"a@example.com"}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := *base
			tt.mutate(&cfg)

			err := validateConfiguration(&cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseFlexibleTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"RFC3339", "2026-08-01T10:00:00Z", false},
		{"RFC3339 with offset", "2026-08-01T10:00:00+02:00", false},
		{"PowerShell sortable", "2026-08-01T10:00:00", false},
		{"empty", "", true},
		{"garbage", "next tuesday", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFlexibleTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFlexibleTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestBuildAttendeesXML(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		if got := buildAttendeesXML(nil); got != "" {
			t.Errorf("buildAttendeesXML(nil) = %q, want empty", got)
		}
	})

	t.Run("escapes XML special characters", func(t *testing.T) {
		got := buildAttendeesXML([]string{`a&b@example.com`})
		if !strings.Contains(got, "a&amp;b@example.com") {
			t.Errorf("buildAttendeesXML() = %q, want escaped ampersand", got)
		}
		if !strings.Contains(got, "<t:Attendee><t:Mailbox>") {
			t.Errorf("buildAttendeesXML() = %q, missing Attendee/Mailbox nesting", got)
		}
	})
}

func TestBuildTLSConfig(t *testing.T) {
	t.Run("tls 1.2 default", func(t *testing.T) {
		cfg, err := buildTLSConfig(&Config{TLSVersion: "1.2", SkipVerify: false})
		if err != nil {
			t.Fatalf("buildTLSConfig() error = %v", err)
		}
		if cfg.MinVersion != tls.VersionTLS12 {
			t.Fatalf("MinVersion = %d, want TLS1.2", cfg.MinVersion)
		}
		if cfg.InsecureSkipVerify {
			t.Fatalf("InsecureSkipVerify = true, want false")
		}
	})

	t.Run("tls 1.3 and skip verify", func(t *testing.T) {
		cfg, err := buildTLSConfig(&Config{TLSVersion: "1.3", SkipVerify: true})
		if err != nil {
			t.Fatalf("buildTLSConfig() error = %v", err)
		}
		if cfg.MinVersion != tls.VersionTLS13 {
			t.Fatalf("MinVersion = %d, want TLS1.3", cfg.MinVersion)
		}
		if !cfg.InsecureSkipVerify {
			t.Fatalf("InsecureSkipVerify = false, want true")
		}
	})
}

func TestCertCSVFields(t *testing.T) {
	t.Run("empty cert slice", func(t *testing.T) {
		subject, issuer, sans, validFrom, validTo := certCSVFields(nil)
		if subject != "" || issuer != "" || sans != "" || validFrom != "" || validTo != "" {
			t.Fatalf("expected all empty strings for nil cert slice")
		}
	})

	t.Run("extracts first cert values", func(t *testing.T) {
		now := time.Now()
		cert := &x509.Certificate{
			Subject:   pkix.Name{CommonName: "mail.example.com"},
			Issuer:    pkix.Name{CommonName: "Example CA"},
			DNSNames:  []string{"mail.example.com", "autodiscover.example.com"},
			NotBefore: now.Add(-time.Hour),
			NotAfter:  now.Add(24 * time.Hour),
		}

		subject, issuer, sans, validFrom, validTo := certCSVFields([]*x509.Certificate{cert})
		if subject != "mail.example.com" {
			t.Fatalf("subject = %q", subject)
		}
		if issuer != "Example CA" {
			t.Fatalf("issuer = %q", issuer)
		}
		if sans != "mail.example.com;autodiscover.example.com" {
			t.Fatalf("sans = %q", sans)
		}
		if validFrom == "" || validTo == "" {
			t.Fatalf("expected non-empty validity dates")
		}
	})
}
