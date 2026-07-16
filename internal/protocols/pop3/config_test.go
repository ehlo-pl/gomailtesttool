//go:build !integration
// +build !integration

package pop3

import (
	"strings"
	"testing"
)

// TestValidateConfiguration_POP3SAndSTARTTLS tests mutual exclusion of POP3S and STARTTLS flags
func TestValidateConfiguration_POP3SAndSTARTTLS(t *testing.T) {
	tests := []struct {
		name      string
		pop3s     bool
		starttls  bool
		wantError bool
		errorMsg  string
	}{
		{name: "Neither POP3S nor STARTTLS", pop3s: false, starttls: false, wantError: false},
		{name: "POP3S only", pop3s: true, starttls: false, wantError: false},
		{name: "STARTTLS only", pop3s: false, starttls: true, wantError: false},
		{
			name:      "Both POP3S and STARTTLS - should error",
			pop3s:     true,
			starttls:  true,
			wantError: true,
			errorMsg:  "cannot use both --pop3s and --starttls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = ActionTestConnect
			config.Host = "pop3.example.com"
			config.POP3S = tt.pop3s
			config.StartTLS = tt.starttls

			err := validateConfiguration(config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConfiguration() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateConfiguration() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("validateConfiguration() unexpected error = %v", err)
			}
		})
	}
}

// TestValidateConfiguration_Actions tests per-action validation, in particular
// which actions require credentials.
func TestValidateConfiguration_Actions(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		username  string
		password  string
		pop3s     bool
		wantError bool
		errorMsg  string
	}{
		{name: "testconnect needs no creds", action: ActionTestConnect, wantError: false},
		{name: "teststarttls needs no creds", action: ActionTestStartTLS, wantError: false},
		{name: "teststarttls with pop3s", action: ActionTestStartTLS, pop3s: true, wantError: false},
		{name: "listmail with creds", action: ActionListMail, username: "user", password: "pass", wantError: false},
		{name: "invalid action", action: "bogus", wantError: true, errorMsg: "invalid action"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = tt.action
			config.Host = "pop3.example.com"
			config.Username = tt.username
			config.Password = tt.password
			config.POP3S = tt.pop3s

			err := validateConfiguration(config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConfiguration() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateConfiguration() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("validateConfiguration() unexpected error = %v", err)
			}
		})
	}
}

// TestValidateConfiguration_NoTLSFlags tests --no-pop3s/--no-starttls mutual exclusion
// with --pop3s/--starttls.
func TestValidateConfiguration_NoTLSFlags(t *testing.T) {
	tests := []struct {
		name       string
		pop3s      bool
		starttls   bool
		noPOP3S    bool
		noStartTLS bool
		wantError  bool
		errorMsg   string
	}{
		{name: "no-starttls alone is fine", noStartTLS: true, wantError: false},
		{name: "no-pop3s alone is fine", noPOP3S: true, wantError: false},
		{
			name:      "pop3s and no-pop3s conflict",
			pop3s:     true,
			noPOP3S:   true,
			wantError: true,
			errorMsg:  "cannot use both --pop3s and --no-pop3s",
		},
		{
			name:       "starttls and no-starttls conflict",
			starttls:   true,
			noStartTLS: true,
			wantError:  true,
			errorMsg:   "cannot use both --starttls and --no-starttls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = ActionTestConnect
			config.Host = "pop3.example.com"
			config.POP3S = tt.pop3s
			config.StartTLS = tt.starttls
			config.NoPOP3S = tt.noPOP3S
			config.NoStartTLS = tt.noStartTLS

			err := validateConfiguration(config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConfiguration() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateConfiguration() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("validateConfiguration() unexpected error = %v", err)
			}
		})
	}
}
