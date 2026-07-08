//go:build !integration
// +build !integration

package msgraph

import (
	"strings"
	"testing"
)

// validConfigForPriorityTest returns a Config that passes every check in
// validateConfiguration() except (possibly) the priority validation, so the
// priority validation can be tested in isolation.
func validConfigForPriorityTest(priority string) *Config {
	cfg := NewConfig()
	cfg.TenantID = "00000000-0000-0000-0000-000000000001"
	cfg.ClientID = "00000000-0000-0000-0000-000000000002"
	cfg.Mailbox = "user@example.com"
	cfg.BearerToken = "token"
	cfg.Priority = priority
	return cfg
}

func TestValidateConfiguration_Priority(t *testing.T) {
	tests := []struct {
		name      string
		priority  string
		wantError bool
	}{
		{name: "high is valid", priority: "high"},
		{name: "normal is valid", priority: "normal"},
		{name: "low is valid", priority: "low"},
		{name: "invalid value", priority: "urgent", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfiguration(validConfigForPriorityTest(tt.priority))

			if tt.wantError {
				if err == nil {
					t.Fatal("validateConfiguration() expected error, got nil")
				}
				if !strings.Contains(err.Error(), "-priority") {
					t.Errorf("validateConfiguration() error = %v, want error mentioning -priority", err)
				}
			} else if err != nil {
				t.Errorf("validateConfiguration() unexpected error = %v", err)
			}
		})
	}
}

func validConfigForDelegatedTest() *Config {
	cfg := NewConfig()
	cfg.TenantID = "00000000-0000-0000-0000-000000000001"
	cfg.ClientID = "00000000-0000-0000-0000-000000000002"
	cfg.Mailbox = "user@example.com"
	cfg.Delegated = true
	cfg.AuthFlow = "devicecode"
	return cfg
}

func TestValidateConfiguration_Delegated(t *testing.T) {
	t.Run("devicecode without bearer token is valid", func(t *testing.T) {
		cfg := validConfigForDelegatedTest()
		if err := validateConfiguration(cfg); err != nil {
			t.Fatalf("validateConfiguration() unexpected error = %v", err)
		}
	})

	t.Run("browser flow requires redirect URL", func(t *testing.T) {
		cfg := validConfigForDelegatedTest()
		cfg.AuthFlow = "browser"

		err := validateConfiguration(cfg)
		if err == nil || !strings.Contains(err.Error(), "--redirecturl") {
			t.Fatalf("expected redirect URL validation error, got: %v", err)
		}
	})

	t.Run("browser flow with redirect URL is valid", func(t *testing.T) {
		cfg := validConfigForDelegatedTest()
		cfg.AuthFlow = "browser"
		cfg.RedirectURL = "http://localhost:8400/callback"

		if err := validateConfiguration(cfg); err != nil {
			t.Fatalf("validateConfiguration() unexpected error = %v", err)
		}
	})

	t.Run("delegated without bearer token rejects app credentials", func(t *testing.T) {
		cfg := validConfigForDelegatedTest()
		cfg.Secret = "app-secret"

		err := validateConfiguration(cfg)
		if err == nil || !strings.Contains(err.Error(), "delegated mode") {
			t.Fatalf("expected delegated mode validation error, got: %v", err)
		}
	})
func TestValidateExportBearerTokenConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError string
	}{
		{
			name: "accepts pre-obtained bearer token without tenant and client IDs",
			config: &Config{
				BearerToken:  "token",
				OutputFormat: "text",
			},
		},
		{
			name: "accepts client secret flow with tenant and client IDs",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Secret:       "secret",
				OutputFormat: "json",
			},
		},
		{
			name: "requires tenant id when not using pre-obtained token",
			config: &Config{
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Secret:       "secret",
				OutputFormat: "text",
			},
			wantError: "Tenant ID",
		},
		{
			name: "requires exactly one auth method",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				OutputFormat: "text",
			},
			wantError: "missing authentication",
		},
		{
			name: "validates output format",
			config: &Config{
				BearerToken:  "token",
				OutputFormat: "yaml",
			},
			wantError: "invalid output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExportBearerTokenConfiguration(tt.config)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateExportBearerTokenConfiguration() unexpected error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateExportBearerTokenConfiguration() expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validateExportBearerTokenConfiguration() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}
