//go:build !integration

package smtp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSendMailConfig returns a valid baseline sendmail config.
func newSendMailConfig() *Config {
	cfg := NewConfig()
	cfg.Action = ActionSendMail
	cfg.Host = "smtp.example.com"
	cfg.From = "sender@example.com"
	cfg.To = []string{"recipient@example.com"}
	return cfg
}

func writeTemplateFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write template file: %v", err)
	}
	return path
}

func TestValidateConfiguration_Template(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(c *Config, dir string)
		errorMsg string // empty means no error expected
	}{
		{
			name: "template-vars without template",
			mutate: func(c *Config, dir string) {
				c.TemplateVars = []string{"Name=World"}
			},
			errorMsg: "-template-vars requires -template",
		},
		{
			name: "template with bodyhtml",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "body.html")
				c.BodyHTML = "<p>x</p>"
			},
			errorMsg: "cannot use both -template and -bodyhtml",
		},
		{
			name: "template with body",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "body.html")
				c.Body = "custom body"
			},
			errorMsg: "cannot use both -template and -body",
		},
		{
			name: "invalid template-vars format",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "body.html")
				c.TemplateVars = []string{"noequals"}
			},
			errorMsg: "invalid -template-vars",
		},
		{
			name: "eml template with attachments",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "msg.eml")
				c.Attachments = []string{filepath.Join(dir, "a.txt")}
			},
			errorMsg: "complete message",
		},
		{
			name: "eml template with priority",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "msg.eml")
				c.Priority = "high"
			},
			errorMsg: "complete message",
		},
		{
			name: "eml template relaxes from/to/subject",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "msg.eml")
				c.From = ""
				c.To = nil
				c.Subject = ""
			},
		},
		{
			name: "html template still requires from",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "body.html")
				c.From = ""
			},
			errorMsg: "sendmail requires -from",
		},
		{
			name: "html template with vars is valid",
			mutate: func(c *Config, dir string) {
				c.Template = filepath.Join(dir, "body.html")
				c.TemplateVars = []string{"Name=World"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// The referenced template files must exist for path validation.
			for _, name := range []string{"body.html", "msg.eml", "a.txt"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
					t.Fatalf("failed to write %s: %v", name, err)
				}
			}

			config := newSendMailConfig()
			tt.mutate(config, dir)

			err := validateConfiguration(config)
			if tt.errorMsg == "" {
				if err != nil {
					t.Fatalf("validateConfiguration() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateConfiguration() expected error containing %q, got nil", tt.errorMsg)
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("validateConfiguration() error = %q, want substring %q", err.Error(), tt.errorMsg)
			}
		})
	}
}

func TestResolveTemplate_HTMLMode(t *testing.T) {
	path := writeTemplateFile(t, "body.html", "<p>Hello {{.Name}}!</p>")

	config := newSendMailConfig()
	config.Template = path
	config.TemplateVars = []string{"Name=World"}

	if err := resolveTemplate(config); err != nil {
		t.Fatalf("resolveTemplate() unexpected error: %v", err)
	}
	if config.BodyHTML != "<p>Hello World!</p>" {
		t.Errorf("BodyHTML = %q, want rendered template", config.BodyHTML)
	}
	if config.RawMessage != nil {
		t.Errorf("RawMessage should be nil in HTML mode")
	}
}

const testEMLTemplate = "From: Tpl Sender <tpl-sender@example.com>\r\n" +
	"To: tpl-rcpt@example.com\r\n" +
	"Cc: tpl-cc@example.com\r\n" +
	"Subject: Hello {{.Name}}\r\n" +
	"Message-ID: <tpl-id@example.com>\r\n" +
	"\r\n" +
	"Greetings, {{.Name}}!\r\n"

func TestResolveTemplate_EMLMode(t *testing.T) {
	path := writeTemplateFile(t, "msg.eml", testEMLTemplate)

	config := newSendMailConfig()
	config.Template = path
	config.TemplateVars = []string{"Name=World"}
	config.From = ""
	config.To = nil

	if err := resolveTemplate(config); err != nil {
		t.Fatalf("resolveTemplate() unexpected error: %v", err)
	}
	if config.RawMessage == nil {
		t.Fatal("RawMessage not set in EML mode")
	}
	if !strings.Contains(string(config.RawMessage), "Greetings, World!") {
		t.Errorf("RawMessage = %q, want rendered body", config.RawMessage)
	}
	if config.From != "tpl-sender@example.com" {
		t.Errorf("From = %q, want tpl-sender@example.com (from EML)", config.From)
	}
	if len(config.To) != 1 || config.To[0] != "tpl-rcpt@example.com" {
		t.Errorf("To = %v, want [tpl-rcpt@example.com] (from EML)", config.To)
	}
	if len(config.Cc) != 1 || config.Cc[0] != "tpl-cc@example.com" {
		t.Errorf("Cc = %v, want [tpl-cc@example.com] (from EML)", config.Cc)
	}
	if config.Subject != "Hello World" {
		t.Errorf("Subject = %q, want %q (from EML)", config.Subject, "Hello World")
	}
	if config.RawMessageID != "tpl-id@example.com" {
		t.Errorf("RawMessageID = %q, want tpl-id@example.com", config.RawMessageID)
	}
}

func TestResolveTemplate_EMLMode_FlagsWin(t *testing.T) {
	path := writeTemplateFile(t, "msg.eml", testEMLTemplate)

	config := newSendMailConfig()
	config.Template = path
	config.TemplateVars = []string{"Name=World"}
	// From/To set by newSendMailConfig simulate explicit flags.

	if err := resolveTemplate(config); err != nil {
		t.Fatalf("resolveTemplate() unexpected error: %v", err)
	}
	if config.From != "sender@example.com" {
		t.Errorf("From = %q, want flag value sender@example.com", config.From)
	}
	if len(config.To) != 1 || config.To[0] != "recipient@example.com" {
		t.Errorf("To = %v, want flag value [recipient@example.com]", config.To)
	}
}

func TestResolveTemplate_EMLMode_Errors(t *testing.T) {
	tests := []struct {
		name     string
		eml      string
		errorMsg string
	}{
		{
			name:     "no recipients",
			eml:      "From: a@b.c\r\nSubject: s\r\n\r\nbody\r\n",
			errorMsg: "no recipients",
		},
		{
			name:     "no from",
			eml:      "To: a@b.c\r\nSubject: s\r\n\r\nbody\r\n",
			errorMsg: "no From header",
		},
		{
			name:     "missing template variable",
			eml:      "From: a@b.c\r\nTo: d@e.f\r\nSubject: {{.Missing}}\r\n\r\nbody\r\n",
			errorMsg: "failed to render",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newSendMailConfig()
			config.Template = writeTemplateFile(t, "msg.eml", tt.eml)
			config.From = ""
			config.To = nil

			err := resolveTemplate(config)
			if err == nil {
				t.Fatalf("resolveTemplate() expected error containing %q, got nil", tt.errorMsg)
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("resolveTemplate() error = %q, want substring %q", err.Error(), tt.errorMsg)
			}
		})
	}
}
