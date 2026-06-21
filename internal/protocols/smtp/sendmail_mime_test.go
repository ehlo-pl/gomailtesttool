//go:build !integration
// +build !integration

package smtp

import (
	"bytes"
	"encoding/base64"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestConfig() *Config {
	cfg := NewConfig()
	cfg.From = "sender@example.com"
	cfg.To = []string{"recipient@example.com"}
	cfg.Subject = "Test Subject"
	cfg.Body = "Plain text body"
	return cfg
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// parseMessage parses a built message into its headers and, if multipart,
// returns the parsed multipart reader.
func parseMessage(t *testing.T, data []byte) (*mail.Message, *multipart.Reader) {
	t.Helper()

	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("mail.ReadMessage() error = %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("mime.ParseMediaType() error = %v", err)
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return msg, nil
	}

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return msg, multipart.NewReader(bytes.NewReader(body), params["boundary"])
}

func TestBuildMIMEMessage_NoExtrasFallsBackToPlainText(t *testing.T) {
	cfg := newTestConfig()

	got, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	want := buildEmailMessage(cfg.From, cfg.To, cfg.Cc, cfg.Subject, cfg.Body, cfg.Priority)

	// Message-IDs are generated independently and will differ; strip them
	// before comparing the remaining structure.
	stripMessageID := func(s string) string {
		lines := strings.Split(s, "\r\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "Message-ID:") {
				lines[i] = "Message-ID: <redacted>"
			}
			if strings.HasPrefix(line, "Date:") {
				lines[i] = "Date: <redacted>"
			}
		}
		return strings.Join(lines, "\r\n")
	}

	if stripMessageID(string(got)) != stripMessageID(string(want)) {
		t.Errorf("buildMIMEMessage() with no extras = %q, want %q (buildEmailMessage output)", got, want)
	}
}

func TestBuildMIMEMessage_CcHeaderPresent_BccHeaderAbsent(t *testing.T) {
	cfg := newTestConfig()
	cfg.Cc = []string{"cc@example.com"}
	cfg.Bcc = []string{"bcc@example.com"}

	got, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	messageStr := string(got)
	if !strings.Contains(messageStr, "Cc: cc@example.com\r\n") {
		t.Errorf("buildMIMEMessage() missing Cc header, got:\n%s", messageStr)
	}
	if strings.Contains(messageStr, "bcc@example.com") || strings.Contains(messageStr, "Bcc:") {
		t.Errorf("buildMIMEMessage() must not include Bcc in headers, got:\n%s", messageStr)
	}
}

// TestBuildMIMEMessage_CcHeaderPresent_ExtrasPath covers the Cc: header
// written in buildMIMEMessage's main (non-fallback) path, exercised when
// BodyHTML (or another "extra") is set alongside Cc.
func TestBuildMIMEMessage_CcHeaderPresent_ExtrasPath(t *testing.T) {
	cfg := newTestConfig()
	cfg.Cc = []string{"cc@example.com"}
	cfg.Bcc = []string{"bcc@example.com"}
	cfg.BodyHTML = "<p>Hello</p>"

	got, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	messageStr := string(got)
	if !strings.Contains(messageStr, "Cc: cc@example.com\r\n") {
		t.Errorf("buildMIMEMessage() (extras path) missing Cc header, got:\n%s", messageStr)
	}
	if strings.Contains(messageStr, "bcc@example.com") || strings.Contains(messageStr, "Bcc:") {
		t.Errorf("buildMIMEMessage() (extras path) must not include Bcc in headers, got:\n%s", messageStr)
	}
}

func TestPriorityHeaderLines(t *testing.T) {
	tests := []struct {
		priority string
		want     []string
	}{
		{"normal", nil},
		{"", nil},
		{"unknown", nil},
		{"high", []string{"X-Priority: 1 (Highest)", "Importance: High", "Priority: urgent"}},
		{"low", []string{"X-Priority: 5 (Lowest)", "Importance: Low", "Priority: non-urgent"}},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got := priorityHeaderLines(tt.priority)
			if len(got) != len(tt.want) {
				t.Fatalf("priorityHeaderLines(%q) = %v, want %v", tt.priority, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("priorityHeaderLines(%q)[%d] = %q, want %q", tt.priority, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildMIMEMessage_PriorityHeaders(t *testing.T) {
	for _, priority := range []string{"high", "low"} {
		t.Run(priority, func(t *testing.T) {
			cfg := newTestConfig()
			cfg.Priority = priority
			cfg.BodyHTML = "<p>Hello</p>" // exercise the "extras" path

			got, err := buildMIMEMessage(cfg, discardLogger())
			if err != nil {
				t.Fatalf("buildMIMEMessage() error = %v", err)
			}

			messageStr := string(got)
			for _, line := range priorityHeaderLines(priority) {
				if !strings.Contains(messageStr, line+"\r\n") {
					t.Errorf("buildMIMEMessage() (priority=%s) missing header %q, got:\n%s", priority, line, messageStr)
				}
			}
		})
	}
}

func TestBuildMIMEMessage_NormalPriorityAddsNoHeaders(t *testing.T) {
	cfg := newTestConfig()
	cfg.Priority = "normal"
	cfg.BodyHTML = "<p>Hello</p>"

	got, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	messageStr := string(got)
	for _, name := range []string{"X-Priority", "Importance", "Priority:"} {
		if strings.Contains(messageStr, name) {
			t.Errorf("buildMIMEMessage() (priority=normal) unexpectedly contains %q, got:\n%s", name, messageStr)
		}
	}
}

func TestBuildEmailMessage_CcHeaderPresent_BccHeaderAbsent(t *testing.T) {
	message := buildEmailMessage(
		"sender@example.com",
		[]string{"recipient@example.com"},
		[]string{"cc@example.com"},
		"Test Subject",
		"Test Body",
		"normal",
	)

	messageStr := string(message)
	if !strings.Contains(messageStr, "Cc: cc@example.com\r\n") {
		t.Errorf("buildEmailMessage() missing Cc header, got:\n%s", messageStr)
	}
	if strings.Contains(messageStr, "Bcc:") {
		t.Errorf("buildEmailMessage() must never write a Bcc header, got:\n%s", messageStr)
	}
}

func TestBuildMIMEMessage_HTMLOnly(t *testing.T) {
	cfg := newTestConfig()
	cfg.Body = ""
	cfg.BodyHTML = "<p>Hello <b>world</b></p>"

	data, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	msg, mr := parseMessage(t, data)
	if mr != nil {
		t.Fatal("expected a single text/html part, got multipart")
	}

	ct := msg.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body, _ := io.ReadAll(msg.Body)
	if string(body) != cfg.BodyHTML {
		t.Errorf("body = %q, want %q", body, cfg.BodyHTML)
	}
}

func TestBuildMIMEMessage_TextAndHTMLAlternative(t *testing.T) {
	cfg := newTestConfig()
	cfg.BodyHTML = "<p>HTML body</p>"

	data, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	msg, mr := parseMessage(t, data)
	if mr == nil {
		t.Fatal("expected multipart/alternative, got single part")
	}
	if !strings.HasPrefix(msg.Header.Get("Content-Type"), "multipart/alternative") {
		t.Errorf("Content-Type = %q, want multipart/alternative", msg.Header.Get("Content-Type"))
	}

	var sawText, sawHTML bool
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		body, _ := io.ReadAll(part)
		switch {
		case strings.HasPrefix(part.Header.Get("Content-Type"), "text/plain"):
			sawText = true
			if string(body) != cfg.Body {
				t.Errorf("text part body = %q, want %q", body, cfg.Body)
			}
		case strings.HasPrefix(part.Header.Get("Content-Type"), "text/html"):
			sawHTML = true
			if string(body) != cfg.BodyHTML {
				t.Errorf("html part body = %q, want %q", body, cfg.BodyHTML)
			}
		}
	}

	if !sawText || !sawHTML {
		t.Errorf("expected both text and html parts, sawText=%v sawHTML=%v", sawText, sawHTML)
	}
}

func TestBuildMIMEMessage_WithAttachment(t *testing.T) {
	dir := t.TempDir()
	attachData := []byte("attachment-content")
	attachPath := writeTempFileForTest(t, dir, "report.txt", attachData)

	cfg := newTestConfig()
	cfg.Attachments = []string{attachPath}

	data, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	msg, mr := parseMessage(t, data)
	if mr == nil {
		t.Fatal("expected multipart/mixed, got single part")
	}
	if !strings.HasPrefix(msg.Header.Get("Content-Type"), "multipart/mixed") {
		t.Errorf("Content-Type = %q, want multipart/mixed", msg.Header.Get("Content-Type"))
	}

	var sawBody, sawAttachment bool
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		body, _ := io.ReadAll(part)

		disposition := part.Header.Get("Content-Disposition")
		switch {
		case strings.HasPrefix(disposition, "attachment"):
			sawAttachment = true
			if !strings.Contains(disposition, `filename="report.txt"`) {
				t.Errorf("attachment Content-Disposition = %q, missing filename", disposition)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
			if err != nil {
				t.Fatalf("base64 decode attachment: %v", err)
			}
			if string(decoded) != string(attachData) {
				t.Errorf("attachment content = %q, want %q", decoded, attachData)
			}
		default:
			sawBody = true
			if string(body) != cfg.Body {
				t.Errorf("body part = %q, want %q", body, cfg.Body)
			}
		}
	}

	if !sawBody || !sawAttachment {
		t.Errorf("expected body and attachment parts, sawBody=%v sawAttachment=%v", sawBody, sawAttachment)
	}
}

func TestBuildMIMEMessage_WithInlineAttachment(t *testing.T) {
	dir := t.TempDir()
	imgData := []byte{0x89, 0x50, 0x4e, 0x47}
	imgPath := writeTempFileForTest(t, dir, "logo.png", imgData)

	cfg := newTestConfig()
	cfg.Body = ""
	cfg.BodyHTML = `<p>Image: <img src="cid:logo.png"></p>`
	cfg.InlineAttachments = []string{imgPath}

	data, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	msg, mr := parseMessage(t, data)
	if mr == nil {
		t.Fatal("expected multipart/related, got single part")
	}
	if !strings.HasPrefix(msg.Header.Get("Content-Type"), "multipart/related") {
		t.Errorf("Content-Type = %q, want multipart/related", msg.Header.Get("Content-Type"))
	}

	var sawHTML, sawInline bool
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		body, _ := io.ReadAll(part)

		if cid := part.Header.Get("Content-ID"); cid != "" {
			sawInline = true
			if cid != "<logo.png>" {
				t.Errorf("Content-ID = %q, want <logo.png>", cid)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
			if err != nil {
				t.Fatalf("base64 decode inline attachment: %v", err)
			}
			if string(decoded) != string(imgData) {
				t.Errorf("inline attachment content = %q, want %q", decoded, imgData)
			}
		} else if strings.HasPrefix(part.Header.Get("Content-Type"), "text/html") {
			sawHTML = true
			if string(body) != cfg.BodyHTML {
				t.Errorf("html part = %q, want %q", body, cfg.BodyHTML)
			}
		}
	}

	if !sawHTML || !sawInline {
		t.Errorf("expected html and inline attachment parts, sawHTML=%v sawInline=%v", sawHTML, sawInline)
	}
}

func TestBuildMIMEMessage_CustomHeaders(t *testing.T) {
	cfg := newTestConfig()
	cfg.Headers = []string{"X-Custom-Header: custom-value", "X-Test: value\r\nBcc: attacker@evil.com"}

	data, err := buildMIMEMessage(cfg, discardLogger())
	if err != nil {
		t.Fatalf("buildMIMEMessage() error = %v", err)
	}

	msg, _ := parseMessage(t, data)
	if got := msg.Header.Get("X-Custom-Header"); got != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want custom-value", got)
	}
	if got := msg.Header.Get("X-Test"); got != "valueBcc: attacker@evil.com" {
		t.Errorf("X-Test = %q, want CRLF-stripped value", got)
	}
	if msg.Header.Get("Bcc") != "" {
		t.Error("CRLF injection via custom header introduced a Bcc header")
	}
}

func TestBuildMIMEMessage_RejectsProtectedHeader(t *testing.T) {
	cfg := newTestConfig()
	cfg.Headers = []string{"Subject: hijacked"}

	if _, err := buildMIMEMessage(cfg, discardLogger()); err == nil {
		t.Error("buildMIMEMessage() error = nil, want error for protected header override")
	}
}

func writeTempFileForTest(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
