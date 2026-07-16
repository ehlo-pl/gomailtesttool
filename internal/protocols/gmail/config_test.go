//go:build !integration
// +build !integration

package gmail

import (
	"os"
	"path/filepath"
	"testing"
)

// tempFile creates a regular file so validation.ValidateFilePath (which requires
// the path to exist) succeeds for auth-file fields.
func tempFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

// newTestConfig returns a minimal valid service-account configuration.
func newTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg := NewConfig()
	cfg.CredentialsFile = tempFile(t, "creds.json")
	cfg.Mailbox = "user@example.com"
	cfg.Action = ActionTestAuth
	return cfg
}

func TestValidateConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, c *Config)
		wantErr bool
	}{
		{
			name:    "valid service account",
			mutate:  func(t *testing.T, c *Config) {},
			wantErr: false,
		},
		{
			name:    "no auth method",
			mutate:  func(t *testing.T, c *Config) { c.CredentialsFile = "" },
			wantErr: true,
		},
		{
			name: "multiple auth methods",
			mutate: func(t *testing.T, c *Config) {
				c.BearerToken = "ya29.token"
			},
			wantErr: true,
		},
		{
			name: "service account without mailbox",
			mutate: func(t *testing.T, c *Config) {
				c.Mailbox = ""
			},
			wantErr: true,
		},
		{
			name: "invalid mailbox",
			mutate: func(t *testing.T, c *Config) {
				c.Mailbox = "not-an-email"
			},
			wantErr: true,
		},
		{
			name: "bearer token without mailbox is allowed",
			mutate: func(t *testing.T, c *Config) {
				c.CredentialsFile = ""
				c.Mailbox = ""
				c.BearerToken = "ya29.token"
			},
			wantErr: false,
		},
		{
			name: "oauth without client-secret file",
			mutate: func(t *testing.T, c *Config) {
				c.CredentialsFile = ""
				c.OAuth = true
			},
			wantErr: true,
		},
		{
			name: "oauth with client-secret file",
			mutate: func(t *testing.T, c *Config) {
				c.CredentialsFile = ""
				c.OAuth = true
				c.OAuthCredentials = tempFile(t, "oauth.json")
			},
			wantErr: false,
		},
		{
			name: "invalid priority",
			mutate: func(t *testing.T, c *Config) {
				c.Priority = "urgent"
			},
			wantErr: true,
		},
		{
			name: "invalid recipient email",
			mutate: func(t *testing.T, c *Config) {
				c.To = stringSlice{"bad-address"}
			},
			wantErr: true,
		},
		{
			name: "exportmessages without criteria",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionExportMessages
				c.Subject = ""
				c.MessageID = ""
			},
			wantErr: true,
		},
		{
			name: "exportmessages with valid message id",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionExportMessages
				c.Subject = ""
				c.MessageID = "<abc123@example.com>"
			},
			wantErr: false,
		},
		{
			name: "exportmessages rejects message-id injection",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionExportMessages
				c.Subject = ""
				c.MessageID = "<a@b> OR rfc822msgid:<c@d>"
			},
			wantErr: true,
		},
		{
			name: "exportmessages with raw search query only",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionExportMessages
				c.Subject = ""
				c.MessageID = ""
				c.SearchQuery = "from:x@example.com newer_than:7d"
			},
			wantErr: false,
		},
		{
			name: "getschedule requires to",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionGetSchedule
			},
			wantErr: true,
		},
		{
			name: "getschedule rejects multiple recipients",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionGetSchedule
				c.To = stringSlice{"a@example.com", "b@example.com"}
			},
			wantErr: true,
		},
		{
			name: "getschedule with single recipient",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionGetSchedule
				c.To = stringSlice{"a@example.com"}
			},
			wantErr: false,
		},
		{
			name: "findtimeslot requires to",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 30
			},
			wantErr: true,
		},
		{
			name: "findtimeslot rejects multiple recipients",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 30
				c.To = stringSlice{"a@example.com", "b@example.com"}
			},
			wantErr: true,
		},
		{
			name: "findtimeslot rejects duration out of range",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 481
				c.To = stringSlice{"a@example.com"}
			},
			wantErr: true,
		},
		{
			name: "findtimeslot with single recipient and valid duration",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionFindTimeSlot
				c.Duration = 30
				c.To = stringSlice{"a@example.com"}
			},
			wantErr: false,
		},
		{
			name: "sendinvite requires start and end",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionSendInvite
				c.StartTime = ""
				c.EndTime = ""
			},
			wantErr: true,
		},
		{
			name: "sendinvite with start and end",
			mutate: func(t *testing.T, c *Config) {
				c.Action = ActionSendInvite
				c.StartTime = "2026-01-15T14:00:00Z"
				c.EndTime = "2026-01-15T15:00:00Z"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			tt.mutate(t, cfg)
			err := validateConfiguration(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMessageID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "<abc@example.com>", false},
		{"empty", "", true},
		{"no brackets", "abc@example.com", true},
		{"quote injection", "<a@b'>", true},
		{"whitespace/operator injection", "<a@b> OR <c@d>", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateMessageID(tt.id); (err != nil) != tt.wantErr {
				t.Errorf("validateMessageID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveScopes(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{ActionSendMail, scopeGmailSend},
		{ActionGetInbox, scopeGmailReadonly},
		{ActionExportMessages, scopeGmailReadonly},
		{ActionGetEvents, scopeCalendarReadonly},
		{ActionSendInvite, scopeCalendarEvents},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			cfg := NewConfig()
			cfg.Action = tt.action
			got := effectiveScopes(cfg)
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("effectiveScopes(%s) = %v, want [%s]", tt.action, got, tt.want)
			}
		})
	}

	t.Run("scope override", func(t *testing.T) {
		cfg := NewConfig()
		cfg.Action = ActionSendMail
		cfg.Scopes = stringSlice{"https://example.com/custom"}
		got := effectiveScopes(cfg)
		if len(got) != 1 || got[0] != "https://example.com/custom" {
			t.Errorf("effectiveScopes with override = %v", got)
		}
	})
}

func TestBuildSearchQuery(t *testing.T) {
	tests := []struct {
		name      string
		messageID string
		subject   string
		want      string
	}{
		{"message id only", "<abc@example.com>", "", "rfc822msgid:abc@example.com"},
		{"subject only", "", "hello world", "subject:(hello world)"},
		{"both", "<abc@example.com>", "hi", "rfc822msgid:abc@example.com subject:(hi)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildSearchQuery(tt.messageID, tt.subject); got != tt.want {
				t.Errorf("buildSearchQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostFromEmail(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"user@example.com", "example.com"},
		{"user@", "gmail.com"},
		{"noatsign", "gmail.com"},
		{"", "gmail.com"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := hostFromEmail(tt.in); got != tt.want {
				t.Errorf("hostFromEmail(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
