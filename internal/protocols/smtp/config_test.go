//go:build !integration
// +build !integration

package smtp

import (
	"strings"
	"testing"
)

// TestValidateConfiguration_SMTPSAndSTARTTLS tests mutual exclusion of SMTPS and STARTTLS flags
func TestValidateConfiguration_SMTPSAndSTARTTLS(t *testing.T) {
	tests := []struct {
		name      string
		smtps     bool
		starttls  bool
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Neither SMTPS nor STARTTLS",
			smtps:     false,
			starttls:  false,
			wantError: false,
		},
		{
			name:      "SMTPS only",
			smtps:     true,
			starttls:  false,
			wantError: false,
		},
		{
			name:      "STARTTLS only",
			smtps:     false,
			starttls:  true,
			wantError: false,
		},
		{
			name:      "Both SMTPS and STARTTLS - should error",
			smtps:     true,
			starttls:  true,
			wantError: true,
			errorMsg:  "cannot use both -smtps and -starttls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = ActionTestConnect
			config.Host = "smtp.example.com"
			config.SMTPS = tt.smtps
			config.StartTLS = tt.starttls

			err := validateConfiguration(config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConfiguration() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateConfiguration() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateConfiguration() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateConfiguration_NoTLSFlags tests --no-smtps/--no-starttls mutual exclusion
// with --smtps/--starttls, and the teststarttls/--no-starttls restriction.
func TestValidateConfiguration_NoTLSFlags(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		smtps      bool
		starttls   bool
		noSMTPS    bool
		noStartTLS bool
		wantError  bool
		errorMsg   string
	}{
		{
			name:       "no-starttls alone is fine",
			action:     ActionTestConnect,
			noStartTLS: true,
			wantError:  false,
		},
		{
			name:      "no-smtps alone is fine",
			action:    ActionTestConnect,
			noSMTPS:   true,
			wantError: false,
		},
		{
			name:      "smtps and no-smtps conflict",
			action:    ActionTestConnect,
			smtps:     true,
			noSMTPS:   true,
			wantError: true,
			errorMsg:  "cannot use both -smtps and -no-smtps",
		},
		{
			name:       "starttls and no-starttls conflict",
			action:     ActionTestConnect,
			starttls:   true,
			noStartTLS: true,
			wantError:  true,
			errorMsg:   "cannot use both -starttls and -no-starttls",
		},
		{
			name:       "teststarttls with no-starttls and no smtps errors",
			action:     ActionTestStartTLS,
			noStartTLS: true,
			wantError:  true,
			errorMsg:   "teststarttls requires STARTTLS or -smtps",
		},
		{
			name:       "teststarttls with no-starttls and smtps and no-smtps errors via mutual exclusion",
			action:     ActionTestStartTLS,
			smtps:      true,
			noSMTPS:    true,
			noStartTLS: true,
			wantError:  true,
			errorMsg:   "cannot use both -smtps and -no-smtps",
		},
		{
			name:      "teststarttls without no-starttls is fine",
			action:    ActionTestStartTLS,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = tt.action
			config.Host = "smtp.example.com"
			config.SMTPS = tt.smtps
			config.StartTLS = tt.starttls
			config.NoSMTPS = tt.noSMTPS
			config.NoStartTLS = tt.noStartTLS

			err := validateConfiguration(config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConfiguration() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateConfiguration() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateConfiguration() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateConfiguration_SMTPSPortDefault tests smart port defaulting for SMTPS
func TestValidateConfiguration_SMTPSPortDefault(t *testing.T) {
	tests := []struct {
		name         string
		smtps        bool
		initialPort  int
		expectedPort int
	}{
		{
			name:         "SMTPS with default port 25 changes to 465",
			smtps:        true,
			initialPort:  25,
			expectedPort: 465,
		},
		{
			name:         "SMTPS with explicit port 587 stays 587",
			smtps:        true,
			initialPort:  587,
			expectedPort: 587,
		},
		{
			name:         "No SMTPS with port 25 stays 25",
			smtps:        false,
			initialPort:  25,
			expectedPort: 25,
		},
		{
			name:         "SMTPS with explicit port 465 stays 465",
			smtps:        true,
			initialPort:  465,
			expectedPort: 465,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = ActionTestConnect
			config.Host = "smtp.example.com"
			config.SMTPS = tt.smtps
			config.Port = tt.initialPort

			err := validateConfiguration(config)
			if err != nil {
				t.Fatalf("validateConfiguration() unexpected error = %v", err)
			}

			if config.Port != tt.expectedPort {
				t.Errorf("validateConfiguration() port = %d, want %d", config.Port, tt.expectedPort)
			}
		})
	}
}

// TestValidateConfiguration_XOAUTH2 tests XOAUTH2 authentication validation
func TestValidateConfiguration_XOAUTH2(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		username    string
		password    string
		accessToken string
		authMethod  string
		wantError   bool
		errorMsg    string
	}{
		// testauth action
		{
			name:        "testauth with password only",
			action:      ActionTestAuth,
			username:    "user@example.com",
			password:    "secret",
			accessToken: "",
			authMethod:  "auto",
			wantError:   false,
		},
		{
			name:        "testauth with accesstoken only",
			action:      ActionTestAuth,
			username:    "user@example.com",
			password:    "",
			accessToken: "ya29.token",
			authMethod:  "auto",
			wantError:   false,
		},
		{
			name:        "testauth with both password and accesstoken",
			action:      ActionTestAuth,
			username:    "user@example.com",
			password:    "secret",
			accessToken: "ya29.token",
			authMethod:  "auto",
			wantError:   false,
		},
		{
			name:        "testauth with XOAUTH2 method but no accesstoken - error",
			action:      ActionTestAuth,
			username:    "user@example.com",
			password:    "secret",
			accessToken: "",
			authMethod:  "XOAUTH2",
			wantError:   true,
			errorMsg:    "XOAUTH2 authentication requires -accesstoken",
		},
		{
			name:        "testauth with no password and no accesstoken - error",
			action:      ActionTestAuth,
			username:    "user@example.com",
			password:    "",
			accessToken: "",
			authMethod:  "auto",
			wantError:   true,
			errorMsg:    "requires -password",
		},
		{
			name:        "testauth with no username - error",
			action:      ActionTestAuth,
			username:    "",
			password:    "secret",
			accessToken: "",
			authMethod:  "auto",
			wantError:   true,
			errorMsg:    "requires -username",
		},

		// testconnect action (no auth required)
		{
			name:        "testconnect with no credentials - OK",
			action:      ActionTestConnect,
			username:    "",
			password:    "",
			accessToken: "",
			authMethod:  "auto",
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = tt.action
			config.Host = "smtp.example.com"
			config.Username = tt.username
			config.Password = tt.password
			config.AccessToken = tt.accessToken
			config.AuthMethod = tt.authMethod

			err := validateConfiguration(config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConfiguration() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateConfiguration() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateConfiguration() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestNewConfig tests default configuration values
func TestNewConfig(t *testing.T) {
	config := NewConfig()

	// Verify defaults
	if config.Port != 25 {
		t.Errorf("NewConfig() Port = %d, want 25", config.Port)
	}
	if config.AuthMethod != "auto" {
		t.Errorf("NewConfig() AuthMethod = %q, want 'auto'", config.AuthMethod)
	}
	if config.TLSVersion != "1.2" {
		t.Errorf("NewConfig() TLSVersion = %q, want '1.2'", config.TLSVersion)
	}
	if config.SMTPS != false {
		t.Errorf("NewConfig() SMTPS = %v, want false", config.SMTPS)
	}
	if config.StartTLS != false {
		t.Errorf("NewConfig() StartTLS = %v, want false", config.StartTLS)
	}
	if config.VerboseMode != false {
		t.Errorf("NewConfig() VerboseMode = %v, want false", config.VerboseMode)
	}
}

// TestValidateConfiguration_Actions tests action validation
func TestValidateConfiguration_Actions(t *testing.T) {
	validActions := []string{
		ActionTestConnect,
		ActionTestStartTLS,
		ActionTestAuth,
		ActionSendMail,
	}

	for _, action := range validActions {
		t.Run("Valid action: "+action, func(t *testing.T) {
			config := NewConfig()
			config.Action = action
			config.Host = "smtp.example.com"

			// Add required fields for specific actions
			if action == ActionTestAuth {
				config.Username = "user@example.com"
				config.Password = "secret"
			}
			if action == ActionSendMail {
				config.Username = "user@example.com"
				config.Password = "secret"
				config.From = "sender@example.com"
				config.To = []string{"recipient@example.com"}
			}

			err := validateConfiguration(config)
			if err != nil {
				t.Errorf("validateConfiguration() unexpected error for action %s: %v", action, err)
			}
		})
	}

	t.Run("Invalid action", func(t *testing.T) {
		config := NewConfig()
		config.Action = "invalidaction"
		config.Host = "smtp.example.com"

		err := validateConfiguration(config)
		if err == nil {
			t.Error("validateConfiguration() expected error for invalid action, got nil")
		}
		if !strings.Contains(err.Error(), "invalid action") {
			t.Errorf("validateConfiguration() error = %v, want error containing 'invalid action'", err)
		}
	})
}

// TestParseBoolEnv tests boolean environment variable parsing
func TestParseBoolEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Truthy values
		{"true lowercase", "true", true},
		{"true uppercase", "TRUE", true},
		{"true mixed case", "True", true},
		{"1", "1", true},
		{"yes lowercase", "yes", true},
		{"yes uppercase", "YES", true},
		{"yes mixed case", "Yes", true},
		{"on lowercase", "on", true},
		{"on uppercase", "ON", true},
		{"on mixed case", "On", true},

		// Falsy values
		{"false lowercase", "false", false},
		{"false uppercase", "FALSE", false},
		{"0", "0", false},
		{"no", "no", false},
		{"off", "off", false},
		{"empty string", "", false},
		{"random string", "random", false},
		{"whitespace", "  ", false},
		{"true with spaces", " true ", false}, // strict matching, no trim
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBoolEnv(tt.input)
			if result != tt.expected {
				t.Errorf("parseBoolEnv(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
