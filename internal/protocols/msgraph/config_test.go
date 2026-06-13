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
