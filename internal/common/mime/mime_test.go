//go:build !integration
// +build !integration

package mime

import (
	"bytes"
	"encoding/base64"
	"io"
	stdmime "mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
)

func baseMessage() Message {
	return Message{
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		Subject:  "Test Subject",
		TextBody: "Plain text body",
	}
}

// parseMessage parses a built message into its headers and, if multipart,
// returns the parsed multipart reader.
func parseMessage(t *testing.T, data []byte) (*mail.Message, *multipart.Reader) {
	t.Helper()

	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("mail.ReadMessage() error = %v", err)
	}

	mediaType, params, err := stdmime.ParseMediaType(msg.Header.Get("Content-Type"))
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

func TestBuild_PlainText(t *testing.T) {
	data, err := Build(baseMessage())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	msg, mr := parseMessage(t, data)
	if mr != nil {
		t.Fatal("expected a single text/plain part, got multipart")
	}
	if ct := msg.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	body, _ := io.ReadAll(msg.Body)
	if strings.TrimRight(string(body), "\r\n") != "Plain text body" {
		t.Errorf("body = %q, want %q", body, "Plain text body")
	}
	// Required RFC 5322 / MIME headers.
	for _, h := range []string{"Message-ID", "Date", "From", "To", "Subject", "Mime-Version"} {
		if msg.Header.Get(h) == "" {
			t.Errorf("missing %s header", h)
		}
	}
}

func TestBuild_HTMLOnly(t *testing.T) {
	m := baseMessage()
	m.TextBody = ""
	m.HTMLBody = "<p>Hello <b>world</b></p>"

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	msg, mr := parseMessage(t, data)
	if mr != nil {
		t.Fatal("expected a single text/html part, got multipart")
	}
	if ct := msg.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(msg.Body)
	if string(body) != m.HTMLBody {
		t.Errorf("body = %q, want %q", body, m.HTMLBody)
	}
}

func TestBuild_TextAndHTMLAlternative(t *testing.T) {
	m := baseMessage()
	m.HTMLBody = "<p>HTML body</p>"

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
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
			if string(body) != m.TextBody {
				t.Errorf("text part = %q, want %q", body, m.TextBody)
			}
		case strings.HasPrefix(part.Header.Get("Content-Type"), "text/html"):
			sawHTML = true
			if string(body) != m.HTMLBody {
				t.Errorf("html part = %q, want %q", body, m.HTMLBody)
			}
		}
	}
	if !sawText || !sawHTML {
		t.Errorf("expected both text and html parts, sawText=%v sawHTML=%v", sawText, sawHTML)
	}
}

func TestBuild_WithInlineAttachment(t *testing.T) {
	m := baseMessage()
	m.TextBody = ""
	m.HTMLBody = `<p><img src="cid:logo.png"></p>`
	imgData := []byte{0x89, 0x50, 0x4e, 0x47}
	m.Inline = []email.Attachment{{
		Name:        "logo.png",
		ContentType: "image/png",
		Data:        imgData,
		ContentID:   "logo.png",
		Inline:      true,
	}}

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
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
				t.Fatalf("base64 decode inline: %v", err)
			}
			if !bytes.Equal(decoded, imgData) {
				t.Errorf("inline content = %v, want %v", decoded, imgData)
			}
		} else if strings.HasPrefix(part.Header.Get("Content-Type"), "text/html") {
			sawHTML = true
		}
	}
	if !sawHTML || !sawInline {
		t.Errorf("expected html and inline parts, sawHTML=%v sawInline=%v", sawHTML, sawInline)
	}
}

func TestBuild_WithAttachment(t *testing.T) {
	m := baseMessage()
	attachData := []byte("attachment-content")
	m.Attachments = []email.Attachment{{
		Name:        "report.txt",
		ContentType: "text/plain",
		Data:        attachData,
	}}

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
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
		if strings.HasPrefix(part.Header.Get("Content-Disposition"), "attachment") {
			sawAttachment = true
			if !strings.Contains(part.Header.Get("Content-Disposition"), `filename="report.txt"`) {
				t.Errorf("attachment disposition missing filename: %q", part.Header.Get("Content-Disposition"))
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
			if err != nil {
				t.Fatalf("base64 decode attachment: %v", err)
			}
			if !bytes.Equal(decoded, attachData) {
				t.Errorf("attachment content = %q, want %q", decoded, attachData)
			}
		} else {
			sawBody = true
		}
	}
	if !sawBody || !sawAttachment {
		t.Errorf("expected body and attachment parts, sawBody=%v sawAttachment=%v", sawBody, sawAttachment)
	}
}

func TestBuild_CustomHeaders(t *testing.T) {
	m := baseMessage()
	m.Headers = []email.Header{{Name: "X-Custom-Header", Value: "custom-value"}}

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	msg, _ := parseMessage(t, data)
	if got := msg.Header.Get("X-Custom-Header"); got != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want custom-value", got)
	}
}

func TestBuild_BccHeaderControlledByFlag(t *testing.T) {
	tests := []struct {
		name    string
		include bool
		wantBcc bool
	}{
		{"excluded (SMTP)", false, false},
		{"included (Gmail)", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := baseMessage()
			m.Cc = []string{"cc@example.com"}
			m.Bcc = []string{"bcc@example.com"}
			m.IncludeBccHeader = tt.include

			data, err := Build(m)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			s := string(data)
			if !strings.Contains(s, "Cc: cc@example.com\r\n") {
				t.Errorf("missing Cc header, got:\n%s", s)
			}
			hasBcc := strings.Contains(s, "Bcc: bcc@example.com\r\n")
			if hasBcc != tt.wantBcc {
				t.Errorf("Bcc header present = %v, want %v, got:\n%s", hasBcc, tt.wantBcc, s)
			}
		})
	}
}

func TestBuild_SanitizesHeaderInjection(t *testing.T) {
	m := baseMessage()
	m.From = "sender@example.com\r\nBcc: attacker@evil.com"
	m.Subject = "Hi\r\nX-Injected: 1"

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(string(data), "From: sender@example.comBcc: attacker@evil.com\r\n") {
		t.Errorf("From not sanitized, got:\n%s", data)
	}

	// The stripped CRLF must not create new header lines: the injected text
	// collapses into the Subject value, and no Bcc/X-Injected header appears.
	msg, _ := parseMessage(t, data)
	if got := msg.Header.Get("Subject"); got != "HiX-Injected: 1" {
		t.Errorf("Subject = %q, want %q", got, "HiX-Injected: 1")
	}
	if msg.Header.Get("X-Injected") != "" {
		t.Errorf("subject CRLF injection introduced an X-Injected header, got:\n%s", data)
	}
	if msg.Header.Get("Bcc") != "" {
		t.Errorf("from CRLF injection introduced a Bcc header, got:\n%s", data)
	}
}

func TestBuild_PriorityHeaders(t *testing.T) {
	for _, priority := range []string{"high", "low"} {
		t.Run(priority, func(t *testing.T) {
			m := baseMessage()
			m.Priority = priority

			data, err := Build(m)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			s := string(data)
			for _, line := range PriorityHeaderLines(priority) {
				if !strings.Contains(s, line+"\r\n") {
					t.Errorf("missing priority header %q, got:\n%s", line, s)
				}
			}
		})
	}
}

func TestBuild_UsesSuppliedMessageID(t *testing.T) {
	m := baseMessage()
	m.MessageID = "fixed-id@host"

	data, err := Build(m)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(string(data), "Message-ID: <fixed-id@host>\r\n") {
		t.Errorf("supplied Message-ID not used, got:\n%s", data)
	}
}

func TestGenerateMessageID(t *testing.T) {
	if got := GenerateMessageID("", "gomailtest"); !strings.Contains(got, "@gomailtest") || !strings.Contains(got, ".gomailtest") {
		t.Errorf("GenerateMessageID(\"\",...) = %q, want default host and tool", got)
	}
	if got := GenerateMessageID("mail.example.com", "gomailtest"); !strings.HasSuffix(got, "@mail.example.com") {
		t.Errorf("GenerateMessageID(host,...) = %q, want @mail.example.com suffix", got)
	}
}
