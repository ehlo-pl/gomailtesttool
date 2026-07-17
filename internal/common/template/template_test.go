//go:build !integration

package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVars_Valid(t *testing.T) {
	tests := []struct {
		name  string
		pairs []string
		want  map[string]string
	}{
		{"empty", nil, map[string]string{}},
		{"single", []string{"Name=World"}, map[string]string{"Name": "World"}},
		{"multiple", []string{"a=1", "b=2"}, map[string]string{"a": "1", "b": "2"}},
		{"empty value", []string{"key="}, map[string]string{"key": ""}},
		{"value with equals", []string{"url=https://x/?a=b"}, map[string]string{"url": "https://x/?a=b"}},
		{"duplicate last wins", []string{"k=1", "k=2"}, map[string]string{"k": "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVars(tt.pairs)
			if err != nil {
				t.Fatalf("ParseVars(%v) unexpected error: %v", tt.pairs, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseVars(%v) = %v, want %v", tt.pairs, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("ParseVars(%v)[%q] = %q, want %q", tt.pairs, k, got[k], v)
				}
			}
		})
	}
}

func TestParseVars_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		pairs    []string
		errorMsg string
	}{
		{"no equals", []string{"NameWorld"}, "expected \"key=value\""},
		{"empty key", []string{"=value"}, "expected \"key=value\""},
		{"empty entry", []string{""}, "expected \"key=value\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseVars(tt.pairs)
			if err == nil {
				t.Fatalf("ParseVars(%v) expected error, got nil", tt.pairs)
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("ParseVars(%v) error = %q, want substring %q", tt.pairs, err.Error(), tt.errorMsg)
			}
		})
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestRender(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		vars     map[string]string
		want     string
		errorMsg string
	}{
		{
			name:    "substitution",
			content: "Hello {{.Name}}!",
			vars:    map[string]string{"Name": "World"},
			want:    "Hello World!",
		},
		{
			name:    "no variables",
			content: "<p>static</p>",
			vars:    map[string]string{},
			want:    "<p>static</p>",
		},
		{
			name:     "missing variable errors",
			content:  "Hello {{.Missing}}!",
			vars:     map[string]string{"Name": "World"},
			errorMsg: "failed to render",
		},
		{
			name:     "malformed template",
			content:  "Hello {{.Name",
			vars:     map[string]string{"Name": "World"},
			errorMsg: "failed to parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, "tpl.html", tt.content)
			got, err := Render(path, tt.vars)
			if tt.errorMsg != "" {
				if err == nil {
					t.Fatalf("Render() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Render() error = %q, want substring %q", err.Error(), tt.errorMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender_FileNotFound(t *testing.T) {
	_, err := Render(filepath.Join(t.TempDir(), "missing.eml"), nil)
	if err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("Render() error = %v, want read failure", err)
	}
}

func TestIsEML(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"message.eml", true},
		{"MESSAGE.EML", true},
		{"dir/msg.Eml", true},
		{"body.html", false},
		{"body.htm", false},
		{"template.tmpl", false},
		{"noextension", false},
	}
	for _, tt := range tests {
		if got := IsEML(tt.path); got != tt.want {
			t.Errorf("IsEML(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

const simpleEML = "From: Alice <alice@example.com>\r\n" +
	"To: bob@example.com, Carol <carol@example.com>\r\n" +
	"Cc: dave@example.com\r\n" +
	"Bcc: eve@example.com\r\n" +
	"Subject: Test message\r\n" +
	"Message-ID: <abc123@example.com>\r\n" +
	"X-Custom: something\r\n" +
	"\r\n" +
	"Plain body line.\r\n"

func TestParseEML_Simple(t *testing.T) {
	parsed, err := ParseEML(simpleEML)
	if err != nil {
		t.Fatalf("ParseEML() unexpected error: %v", err)
	}
	if parsed.From != "alice@example.com" {
		t.Errorf("From = %q, want alice@example.com", parsed.From)
	}
	if len(parsed.To) != 2 || parsed.To[0] != "bob@example.com" || parsed.To[1] != "carol@example.com" {
		t.Errorf("To = %v, want [bob@example.com carol@example.com]", parsed.To)
	}
	if len(parsed.Cc) != 1 || parsed.Cc[0] != "dave@example.com" {
		t.Errorf("Cc = %v, want [dave@example.com]", parsed.Cc)
	}
	if len(parsed.Bcc) != 1 || parsed.Bcc[0] != "eve@example.com" {
		t.Errorf("Bcc = %v, want [eve@example.com]", parsed.Bcc)
	}
	if parsed.Subject != "Test message" {
		t.Errorf("Subject = %q, want %q", parsed.Subject, "Test message")
	}
	if parsed.MessageID != "abc123@example.com" {
		t.Errorf("MessageID = %q, want abc123@example.com", parsed.MessageID)
	}
	if !strings.Contains(parsed.TextBody, "Plain body line.") {
		t.Errorf("TextBody = %q, want to contain %q", parsed.TextBody, "Plain body line.")
	}
	if parsed.HTMLBody != "" {
		t.Errorf("HTMLBody = %q, want empty", parsed.HTMLBody)
	}
	if len(parsed.UnmappedHeaders) != 1 || parsed.UnmappedHeaders[0] != "X-Custom" {
		t.Errorf("UnmappedHeaders = %v, want [X-Custom]", parsed.UnmappedHeaders)
	}
}

const multipartEML = "From: alice@example.com\r\n" +
	"To: bob@example.com\r\n" +
	"Subject: =?utf-8?q?Encoded_subject?=\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/alternative; boundary=\"BOUNDARY\"\r\n" +
	"\r\n" +
	"--BOUNDARY\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"Hello=20plain\r\n" +
	"--BOUNDARY\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"PGI+SGVsbG88L2I+\r\n" +
	"--BOUNDARY--\r\n"

func TestParseEML_MultipartAlternative(t *testing.T) {
	parsed, err := ParseEML(multipartEML)
	if err != nil {
		t.Fatalf("ParseEML() unexpected error: %v", err)
	}
	if parsed.Subject != "Encoded subject" {
		t.Errorf("Subject = %q, want %q", parsed.Subject, "Encoded subject")
	}
	if got := strings.TrimRight(parsed.TextBody, "\r\n"); got != "Hello plain" {
		t.Errorf("TextBody = %q, want %q", got, "Hello plain")
	}
	if parsed.HTMLBody != "<b>Hello</b>" {
		t.Errorf("HTMLBody = %q, want %q", parsed.HTMLBody, "<b>Hello</b>")
	}
	if len(parsed.UnmappedHeaders) != 0 {
		t.Errorf("UnmappedHeaders = %v, want none", parsed.UnmappedHeaders)
	}
}

func TestParseEML_HTMLOnly(t *testing.T) {
	eml := "From: a@b.c\r\nTo: d@e.f\r\nSubject: s\r\nContent-Type: text/html\r\n\r\n<p>hi</p>\r\n"
	parsed, err := ParseEML(eml)
	if err != nil {
		t.Fatalf("ParseEML() unexpected error: %v", err)
	}
	if !strings.Contains(parsed.HTMLBody, "<p>hi</p>") {
		t.Errorf("HTMLBody = %q, want to contain <p>hi</p>", parsed.HTMLBody)
	}
	if parsed.TextBody != "" {
		t.Errorf("TextBody = %q, want empty", parsed.TextBody)
	}
}

func TestParseEML_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		errorMsg string
	}{
		{"not a message", "no headers here", "failed to parse EML"},
		{"bad address", "From: <<<\r\nTo: a@b.c\r\n\r\nbody\r\n", "failed to parse From header"},
		{"multipart without boundary", "From: a@b.c\r\nContent-Type: multipart/mixed\r\n\r\nbody\r\n", "missing a boundary"},
		{"unsupported encoding", "From: a@b.c\r\nContent-Transfer-Encoding: uuencode\r\n\r\nbody\r\n", "unsupported Content-Transfer-Encoding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEML(tt.raw)
			if err == nil {
				t.Fatalf("ParseEML() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("ParseEML() error = %q, want substring %q", err.Error(), tt.errorMsg)
			}
		})
	}
}

func TestRenderAndParseEML_RoundTrip(t *testing.T) {
	tpl := "From: {{.From}}\r\n" +
		"To: {{.To}}\r\n" +
		"Subject: Hello {{.Name}}\r\n" +
		"\r\n" +
		"Greetings, {{.Name}}!\r\n"
	path := writeTempFile(t, "msg.eml", tpl)

	rendered, err := Render(path, map[string]string{
		"From": "sender@example.com",
		"To":   "rcpt@example.com",
		"Name": "World",
	})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}

	parsed, err := ParseEML(rendered)
	if err != nil {
		t.Fatalf("ParseEML() unexpected error: %v", err)
	}
	if parsed.From != "sender@example.com" {
		t.Errorf("From = %q, want sender@example.com", parsed.From)
	}
	if len(parsed.To) != 1 || parsed.To[0] != "rcpt@example.com" {
		t.Errorf("To = %v, want [rcpt@example.com]", parsed.To)
	}
	if parsed.Subject != "Hello World" {
		t.Errorf("Subject = %q, want %q", parsed.Subject, "Hello World")
	}
	if !strings.Contains(parsed.TextBody, "Greetings, World!") {
		t.Errorf("TextBody = %q, want to contain %q", parsed.TextBody, "Greetings, World!")
	}
}
