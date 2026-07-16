//go:build !integration
// +build !integration

package imap

import (
	"strings"
	"testing"
)

// TestValidateConfiguration_IMAPSAndSTARTTLS tests mutual exclusion of IMAPS and STARTTLS flags
func TestValidateConfiguration_IMAPSAndSTARTTLS(t *testing.T) {
	tests := []struct {
		name      string
		imaps     bool
		starttls  bool
		wantError bool
		errorMsg  string
	}{
		{name: "Neither IMAPS nor STARTTLS", imaps: false, starttls: false, wantError: false},
		{name: "IMAPS only", imaps: true, starttls: false, wantError: false},
		{name: "STARTTLS only", imaps: false, starttls: true, wantError: false},
		{
			name:      "Both IMAPS and STARTTLS - should error",
			imaps:     true,
			starttls:  true,
			wantError: true,
			errorMsg:  "cannot use both --imaps and --starttls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = ActionTestConnect
			config.Host = "imap.example.com"
			config.IMAPS = tt.imaps
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
		wantError bool
		errorMsg  string
	}{
		{name: "testconnect needs no creds", action: ActionTestConnect, wantError: false},
		{name: "teststarttls needs no creds", action: ActionTestStartTLS, wantError: false},
		{name: "listmail with creds", action: ActionListMail, username: "user", password: "pass", wantError: false},
		{name: "listmail without username", action: ActionListMail, wantError: true, errorMsg: "requires --username"},
		{name: "listmail without password", action: ActionListMail, username: "user", wantError: true, errorMsg: "requires --password"},
		{name: "invalid action", action: "bogus", wantError: true, errorMsg: "invalid action"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.Action = tt.action
			config.Host = "imap.example.com"
			config.Username = tt.username
			config.Password = tt.password

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

// TestValidateConfiguration_NoTLSFlags tests --no-imaps/--no-starttls mutual exclusion
// with --imaps/--starttls.
func TestValidateConfiguration_NoTLSFlags(t *testing.T) {
	tests := []struct {
		name       string
		imaps      bool
		starttls   bool
		noIMAPS    bool
		noStartTLS bool
		wantError  bool
		errorMsg   string
	}{
		{name: "no-starttls alone is fine", noStartTLS: true, wantError: false},
		{name: "no-imaps alone is fine", noIMAPS: true, wantError: false},
		{
			name:      "imaps and no-imaps conflict",
			imaps:     true,
			noIMAPS:   true,
			wantError: true,
			errorMsg:  "cannot use both --imaps and --no-imaps",
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
			config.Host = "imap.example.com"
			config.IMAPS = tt.imaps
			config.StartTLS = tt.starttls
			config.NoIMAPS = tt.noIMAPS
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
