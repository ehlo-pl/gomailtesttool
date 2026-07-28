//go:build !integration
// +build !integration

package msgraph

import (
	"strings"
	"testing"
)

func TestValidateRespondMeeting(t *testing.T) {
	base := func() *Config {
		c := NewConfig()
		c.TenantID = "00000000-0000-0000-0000-000000000001"
		c.ClientID = "00000000-0000-0000-0000-000000000002"
		c.Mailbox = "user@example.com"
		c.Secret = "secret"
		c.Action = ActionRespondMeeting
		c.EventID = "AAMkAGI2THVSAAA="
		c.MeetingResponse = "accept"
		return c
	}

	tests := []struct {
		name     string
		mutate   func(*Config)
		errorMsg string
	}{
		{
			name:     "valid accept",
			mutate:   func(c *Config) {},
			errorMsg: "",
		},
		{
			name:     "valid decline",
			mutate:   func(c *Config) { c.MeetingResponse = "decline" },
			errorMsg: "",
		},
		{
			name:     "valid tentative",
			mutate:   func(c *Config) { c.MeetingResponse = "tentative" },
			errorMsg: "",
		},
		{
			name:     "missing event-id",
			mutate:   func(c *Config) { c.EventID = "" },
			errorMsg: "respondmeeting action requires --event-id",
		},
		{
			name:     "invalid response",
			mutate:   func(c *Config) { c.MeetingResponse = "maybe" },
			errorMsg: "respondmeeting --response must be one of",
		},
		{
			name:     "empty response",
			mutate:   func(c *Config) { c.MeetingResponse = "" },
			errorMsg: "respondmeeting --response must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := validateConfiguration(cfg)
			if tt.errorMsg == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errorMsg)
			}
			if got := err.Error(); !strings.Contains(got, tt.errorMsg) {
				t.Errorf("error %q does not contain %q", got, tt.errorMsg)
			}
		})
	}
}
