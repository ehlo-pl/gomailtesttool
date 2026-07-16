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
}

func TestValidateConfiguration_ExportMessages(t *testing.T) {
	base := func() *Config {
		cfg := NewConfig()
		cfg.TenantID = "00000000-0000-0000-0000-000000000001"
		cfg.ClientID = "00000000-0000-0000-0000-000000000002"
		cfg.Secret = "app-secret"
		cfg.Mailbox = "user@example.com"
		cfg.Action = ActionExportMessages
		cfg.Subject = ""
		return cfg
	}

	t.Run("no criteria fails", func(t *testing.T) {
		err := validateConfiguration(base())
		if err == nil || !strings.Contains(err.Error(), "--folder") {
			t.Fatalf("expected criteria validation error mentioning --folder, got: %v", err)
		}
	})

	t.Run("folder only is valid", func(t *testing.T) {
		cfg := base()
		cfg.Folder = "inbox"
		if err := validateConfiguration(cfg); err != nil {
			t.Fatalf("validateConfiguration() unexpected error = %v", err)
		}
	})

	t.Run("messageid only is still valid", func(t *testing.T) {
		cfg := base()
		cfg.MessageID = "<abc@example.com>"
		if err := validateConfiguration(cfg); err != nil {
			t.Fatalf("validateConfiguration() unexpected error = %v", err)
		}
	})
}

func TestValidateConfiguration_FindTimeSlot(t *testing.T) {
	base := func() *Config {
		cfg := NewConfig()
		cfg.TenantID = "00000000-0000-0000-0000-000000000001"
		cfg.ClientID = "00000000-0000-0000-0000-000000000002"
		cfg.Secret = "app-secret"
		cfg.Mailbox = "user@example.com"
		cfg.Action = ActionFindTimeSlot
		cfg.Duration = 30
		return cfg
	}

	t.Run("missing to fails", func(t *testing.T) {
		err := validateConfiguration(base())
		if err == nil || !strings.Contains(err.Error(), "--to") {
			t.Fatalf("expected --to validation error, got: %v", err)
		}
	})

	t.Run("multiple recipients fail", func(t *testing.T) {
		cfg := base()
		cfg.To = stringSlice{"a@example.com", "b@example.com"}
		err := validateConfiguration(cfg)
		if err == nil || !strings.Contains(err.Error(), "one recipient") {
			t.Fatalf("expected single-recipient validation error, got: %v", err)
		}
	})

	t.Run("duration out of range fails", func(t *testing.T) {
		cfg := base()
		cfg.To = stringSlice{"a@example.com"}
		cfg.Duration = 481
		err := validateConfiguration(cfg)
		if err == nil || !strings.Contains(err.Error(), "--duration") {
			t.Fatalf("expected duration validation error, got: %v", err)
		}
	})

	t.Run("single recipient with valid duration passes", func(t *testing.T) {
		cfg := base()
		cfg.To = stringSlice{"a@example.com"}
		if err := validateConfiguration(cfg); err != nil {
			t.Fatalf("validateConfiguration() unexpected error = %v", err)
		}
	})
}

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
		{
			name: "accepts delegated devicecode flow without app credentials",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Delegated:    true,
				AuthFlow:     "devicecode",
				OutputFormat: "text",
			},
		},
		{
			name: "accepts delegated browser flow with redirect URL",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Delegated:    true,
				AuthFlow:     "browser",
				RedirectURL:  "http://localhost:8400/callback",
				OutputFormat: "text",
			},
		},
		{
			name: "delegated browser flow requires redirect URL",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Delegated:    true,
				AuthFlow:     "browser",
				OutputFormat: "text",
			},
			wantError: "--redirecturl",
		},
		{
			name: "delegated rejects invalid auth flow",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Delegated:    true,
				AuthFlow:     "bogus",
				OutputFormat: "text",
			},
			wantError: "invalid -authflow",
		},
		{
			name: "delegated without bearer token rejects app credentials",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Delegated:    true,
				Secret:       "app-secret",
				OutputFormat: "text",
			},
			wantError: "delegated mode",
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

func TestValidateTestConnectConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError string
	}{
		{
			name:   "passes with no credentials, mailbox, or IDs",
			config: &Config{OutputFormat: "text"},
		},
		{
			name:   "passes with json output",
			config: &Config{OutputFormat: "json"},
		},
		{
			name:      "rejects invalid output format",
			config:    &Config{OutputFormat: "yaml"},
			wantError: "invalid output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTestConnectConfiguration(tt.config)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateTestConnectConfiguration() unexpected error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateTestConnectConfiguration() expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validateTestConnectConfiguration() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestValidateTestAuthConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError string
	}{
		{
			name:   "accepts bearer token without tenant/client IDs and no mailbox",
			config: &Config{BearerToken: "token", OutputFormat: "text"},
		},
		{
			name: "accepts client secret with tenant and client IDs",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Secret:       "secret",
				OutputFormat: "text",
			},
		},
		{
			name: "accepts an optional valid mailbox",
			config: &Config{
				BearerToken:  "token",
				Mailbox:      "user@example.com",
				OutputFormat: "text",
			},
		},
		{
			name: "rejects missing authentication",
			config: &Config{
				TenantID:     "00000000-0000-0000-0000-000000000001",
				ClientID:     "00000000-0000-0000-0000-000000000002",
				OutputFormat: "text",
			},
			wantError: "missing authentication",
		},
		{
			name: "requires tenant id when not using bearer token",
			config: &Config{
				ClientID:     "00000000-0000-0000-0000-000000000002",
				Secret:       "secret",
				OutputFormat: "text",
			},
			wantError: "Tenant ID",
		},
		{
			name: "rejects an invalid mailbox when provided",
			config: &Config{
				BearerToken:  "token",
				Mailbox:      "not-an-email",
				OutputFormat: "text",
			},
			wantError: "invalid mailbox",
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
			err := validateTestAuthConfiguration(tt.config)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateTestAuthConfiguration() unexpected error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateTestAuthConfiguration() expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validateTestAuthConfiguration() error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}
