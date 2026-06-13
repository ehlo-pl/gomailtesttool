//go:build !integration
// +build !integration

package smtp

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestSanitizeEmailHeader tests email header sanitization for CRLF injection prevention (CRITICAL SECURITY)
func TestSanitizeEmailHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		// Normal cases
		{"Normal email address", "user@example.com", "user@example.com"},
		{"Normal subject", "Test Email Subject", "Test Email Subject"},
		{"Empty header", "", ""},
		{"Header with spaces", "Test Subject Line", "Test Subject Line"},
		{"Header with special chars", "Re: Order #12345", "Re: Order #12345"},

		// Security: CRLF injection attempts (CRITICAL)
		{"Security: From CRLF injection", "sender@example.com\r\nBcc: attacker@evil.com", "sender@example.comBcc: attacker@evil.com"},
		{"Security: Subject injection", "Test\r\nBcc: attacker@evil.com", "TestBcc: attacker@evil.com"},
		{"Security: To header injection", "user@example.com\r\nCc: leak@evil.com", "user@example.comCc: leak@evil.com"},
		{"Security: Newline in Subject", "Subject\nwith newline", "Subjectwith newline"},
		{"Security: Carriage return only", "subject\rwith CR", "subjectwith CR"},
		{"Security: Multiple CRLF", "test\r\n\r\nmalicious", "testmalicious"},
		{"Security: CRLF at start", "\r\nBcc: evil@attacker.com", "Bcc: evil@attacker.com"},
		{"Security: CRLF at end", "Normal Subject\r\n", "Normal Subject"},
		{"Security: Mixed newlines", "Subject\r\nLine2\nLine3\rLine4", "SubjectLine2Line3Line4"},
		{"Security: Bcc injection via From", "user@example.com\r\nBcc: hidden@evil.com\r\n", "user@example.comBcc: hidden@evil.com"},
		{"Security: Reply-To injection", "sender@example.com\r\nReply-To: phishing@evil.com", "sender@example.comReply-To: phishing@evil.com"},
		{"Security: X-Header injection", "Test Subject\r\nX-Priority: 1", "Test SubjectX-Priority: 1"},
		{"Security: Content-Type injection", "user@test.com\r\nContent-Type: text/html", "user@test.comContent-Type: text/html"},
		{"Security: Embedded CRLF in middle", "user@example\r\n.com", "user@example.com"},
		{"Security: Multiple header injection", "from@test.com\r\nCc: leak1@evil.com\r\nBcc: leak2@evil.com", "from@test.comCc: leak1@evil.comBcc: leak2@evil.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeEmailHeader(tt.header)
			if result != tt.expected {
				t.Errorf("sanitizeEmailHeader(%q) = %q, want %q", tt.header, result, tt.expected)
			}
		})
	}
}

// TestBuildEmailMessage tests email message construction with header injection prevention
func TestBuildEmailMessage(t *testing.T) {
	tests := []struct {
		name         string
		from         string
		to           []string
		subject      string
		body         string
		wantFromLine string
		wantToLine   string
		wantSubjLine string
		bodyPreserve bool
	}{
		{
			name:         "Normal single recipient",
			from:         "sender@example.com",
			to:           []string{"recipient@example.com"},
			subject:      "Test Subject",
			body:         "Test message body",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: recipient@example.com",
			wantSubjLine: "Subject: Test Subject",
			bodyPreserve: false,
		},
		{
			name:         "Normal multiple recipients",
			from:         "sender@example.com",
			to:           []string{"user1@example.com", "user2@example.com"},
			subject:      "Multi-recipient Test",
			body:         "Test body",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: user1@example.com, user2@example.com",
			wantSubjLine: "Subject: Multi-recipient Test",
			bodyPreserve: false,
		},
		{
			name:         "Security: From CRLF injection",
			from:         "sender@example.com\r\nBcc: attacker@evil.com",
			to:           []string{"recipient@example.com"},
			subject:      "Test",
			body:         "Body",
			wantFromLine: "From: sender@example.comBcc: attacker@evil.com",
			wantToLine:   "To: recipient@example.com",
			wantSubjLine: "Subject: Test",
			bodyPreserve: false,
		},
		{
			name:         "Security: To header injection",
			from:         "sender@example.com",
			to:           []string{"user@example.com\r\nCc: leak@evil.com"},
			subject:      "Test",
			body:         "Body",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: user@example.comCc: leak@evil.com",
			wantSubjLine: "Subject: Test",
			bodyPreserve: false,
		},
		{
			name:         "Security: Subject injection",
			from:         "sender@example.com",
			to:           []string{"recipient@example.com"},
			subject:      "Test\r\nBcc: attacker@evil.com",
			body:         "Body",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: recipient@example.com",
			wantSubjLine: "Subject: TestBcc: attacker@evil.com",
			bodyPreserve: false,
		},
		{
			name:         "Security: Multiple recipients injection",
			from:         "sender@example.com",
			to:           []string{"user1@example.com\r\nBcc: leak@evil.com", "user2@example.com"},
			subject:      "Test",
			body:         "Body",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: user1@example.comBcc: leak@evil.com, user2@example.com",
			wantSubjLine: "Subject: Test",
			bodyPreserve: false,
		},
		{
			name:         "Body preserves newlines (NOT sanitized)",
			from:         "sender@example.com",
			to:           []string{"recipient@example.com"},
			subject:      "Test",
			body:         "Line 1\nLine 2\r\nLine 3",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: recipient@example.com",
			wantSubjLine: "Subject: Test",
			bodyPreserve: true,
		},
		{
			name:         "Security: Complex injection attempt",
			from:         "sender@example.com\r\nReply-To: phishing@evil.com\r\nX-Priority: 1",
			to:           []string{"victim@bank.com"},
			subject:      "Urgent\r\nBcc: attacker@evil.com\r\nX-Mailer: Evil",
			body:         "Legitimate body with\nnewlines preserved",
			wantFromLine: "From: sender@example.comReply-To: phishing@evil.comX-Priority: 1",
			wantToLine:   "To: victim@bank.com",
			wantSubjLine: "Subject: UrgentBcc: attacker@evil.comX-Mailer: Evil",
			bodyPreserve: true,
		},
		{
			name:         "Body preserves SMTP-like content",
			from:         "sender@example.com",
			to:           []string{"recipient@example.com"},
			subject:      "Test",
			body:         "Line 1\r\n.\r\nMAIL FROM:<attacker@evil.com>",
			wantFromLine: "From: sender@example.com",
			wantToLine:   "To: recipient@example.com",
			wantSubjLine: "Subject: Test",
			bodyPreserve: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := buildEmailMessage(tt.from, tt.to, nil, tt.subject, tt.body, "normal")
			messageStr := string(message)

			// Verify From header is sanitized
			if !strings.Contains(messageStr, tt.wantFromLine) {
				t.Errorf("buildEmailMessage() From header mismatch\ngot:  %s\nwant: %s", messageStr, tt.wantFromLine)
			}

			// Verify To header is sanitized
			if !strings.Contains(messageStr, tt.wantToLine) {
				t.Errorf("buildEmailMessage() To header mismatch\ngot:  %s\nwant: %s", messageStr, tt.wantToLine)
			}

			// Verify Subject header is sanitized
			if !strings.Contains(messageStr, tt.wantSubjLine) {
				t.Errorf("buildEmailMessage() Subject header mismatch\ngot:  %s\nwant: %s", messageStr, tt.wantSubjLine)
			}

			// Verify body is present (with or without newline preservation)
			if !strings.Contains(messageStr, tt.body) {
				if tt.bodyPreserve {
					t.Errorf("buildEmailMessage() body NOT preserved with newlines\ngot:  %s\nwant body: %q", messageStr, tt.body)
				} else if !strings.Contains(messageStr, strings.ReplaceAll(tt.body, "\n", "")) {
					t.Errorf("buildEmailMessage() body missing\ngot:  %s\nwant body: %q", messageStr, tt.body)
				}
			}

			// Verify RFC 5322 format elements
			if !strings.Contains(messageStr, "Message-ID: <") {
				t.Error("buildEmailMessage() missing Message-ID header")
			}
			if !strings.Contains(messageStr, "Date: ") {
				t.Error("buildEmailMessage() missing Date header")
			}
			if !strings.Contains(messageStr, "MIME-Version: 1.0") {
				t.Error("buildEmailMessage() missing MIME-Version header")
			}
			if !strings.Contains(messageStr, "Content-Type: text/plain; charset=UTF-8") {
				t.Error("buildEmailMessage() missing Content-Type header")
			}

			// Verify headers end with CRLF
			lines := strings.Split(messageStr, "\r\n")
			for i, line := range lines {
				if line == "" && i > 0 {
					// Empty line separates headers from body
					break
				}
				if strings.HasPrefix(line, "From:") || strings.HasPrefix(line, "To:") || strings.HasPrefix(line, "Subject:") {
					// Verify no CRLF within the header value (after sanitization)
					if strings.Contains(line, "\n") || strings.Count(messageStr, line) != 1 {
						t.Errorf("buildEmailMessage() header contains unsanitized newlines: %s", line)
					}
				}
			}
		})
	}
}

// TestGenerateMessageID tests message ID generation
func TestGenerateMessageID(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		wantHost string
	}{
		{"With custom host", "smtp.example.com", "@smtp.example.com"},
		{"With empty host (default)", "", "@smtptool"},
		{"With IP address", "192.168.1.1", "@192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgID := generateMessageID(tt.host)

			// Verify format: timestamp.smtptool@host
			if !strings.Contains(msgID, ".smtptool") {
				t.Errorf("generateMessageID() missing '.smtptool': %s", msgID)
			}
			if !strings.Contains(msgID, tt.wantHost) {
				t.Errorf("generateMessageID() host mismatch, got %s, want to contain %s", msgID, tt.wantHost)
			}

			// Verify timestamp component (should be numeric)
			parts := strings.Split(msgID, ".")
			if len(parts) < 2 {
				t.Errorf("generateMessageID() invalid format: %s", msgID)
			}

			// Verify uniqueness (generate two and compare)
			time.Sleep(1 * time.Millisecond)
			msgID2 := generateMessageID(tt.host)
			if msgID == msgID2 {
				t.Errorf("generateMessageID() not unique: %s == %s", msgID, msgID2)
			}
		})
	}
}

func TestSendMail_AllowsNilCSVLogger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cfg := NewConfig()
	cfg.Action = ActionSendMail
	cfg.Host = "127.0.0.1"
	cfg.Port = 1
	cfg.From = "sender@example.com"
	cfg.To = []string{"recipient@example.com"}
	cfg.Subject = "Nil Logger Test"
	cfg.Body = "test body"

	err := SendMail(ctx, cfg, nil, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	if err == nil {
		t.Fatal("SendMail() error = nil, want connection failure without panic")
	}
}

// TestBuildEmailMessage_RFCCompliance tests RFC 5322 compliance
func TestBuildEmailMessage_RFCCompliance(t *testing.T) {
	message := buildEmailMessage(
		"sender@example.com",
		[]string{"recipient@example.com"},
		nil,
		"Test Subject",
		"Test Body",
		"normal",
	)

	messageStr := string(message)

	// RFC 5322 requires CRLF line endings
	if !strings.Contains(messageStr, "\r\n") {
		t.Error("Message does not contain CRLF line endings (RFC 5322 violation)")
	}

	// Headers must be separated from body by empty line (CRLF CRLF)
	if !strings.Contains(messageStr, "\r\n\r\n") {
		t.Error("Message headers not separated from body by empty line (RFC 5322 violation)")
	}

	// Verify header order (Message-ID, Date, From, To, Subject)
	lines := strings.Split(messageStr, "\r\n")
	headerOrder := []string{"Message-ID:", "Date:", "From:", "To:", "Subject:", "MIME-Version:", "Content-Type:"}
	headerIndex := 0

	for _, line := range lines {
		if line == "" {
			break // End of headers
		}
		if headerIndex < len(headerOrder) && strings.HasPrefix(line, headerOrder[headerIndex]) {
			headerIndex++
		}
	}

	if headerIndex != len(headerOrder) {
		t.Errorf("RFC 5322 header order not preserved, found %d/%d headers", headerIndex, len(headerOrder))
	}
}

func TestCollectEnvelopeRecipients(t *testing.T) {
	tests := []struct {
		name string
		to   []string
		cc   []string
		bcc  []string
		want []string
	}{
		{
			name: "to only",
			to:   []string{"a@example.com"},
			want: []string{"a@example.com"},
		},
		{
			name: "to, cc, and bcc combined in order",
			to:   []string{"a@example.com"},
			cc:   []string{"b@example.com"},
			bcc:  []string{"c@example.com"},
			want: []string{"a@example.com", "b@example.com", "c@example.com"},
		},
		{
			name: "multiple recipients per field",
			to:   []string{"a1@example.com", "a2@example.com"},
			cc:   []string{"b1@example.com"},
			bcc:  []string{"c1@example.com", "c2@example.com"},
			want: []string{"a1@example.com", "a2@example.com", "b1@example.com", "c1@example.com", "c2@example.com"},
		},
		{
			name: "no recipients",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			cfg.To = tt.to
			cfg.Cc = tt.cc
			cfg.Bcc = tt.bcc

			got := collectEnvelopeRecipients(cfg)
			if len(got) != len(tt.want) {
				t.Fatalf("collectEnvelopeRecipients() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("collectEnvelopeRecipients()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
