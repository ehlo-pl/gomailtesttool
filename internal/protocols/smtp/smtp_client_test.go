//go:build !integration
// +build !integration

package smtp

import (
	"bytes"
	"crypto/tls"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"msgraphtool/internal/smtp/protocol"
)

// TestDebugLogCommand tests debug logging of SMTP commands
func TestDebugLogCommand(t *testing.T) {
	tests := []struct {
		name        string
		verboseMode bool
		command     string
		wantOutput  bool
		wantContain string
	}{
		{
			name:        "Verbose mode enabled - EHLO command",
			verboseMode: true,
			command:     "EHLO smtptool.local\r\n",
			wantOutput:  true,
			wantContain: ">>> EHLO smtptool.local",
		},
		{
			name:        "Verbose mode enabled - STARTTLS command",
			verboseMode: true,
			command:     "STARTTLS\r\n",
			wantOutput:  true,
			wantContain: ">>> STARTTLS",
		},
		{
			name:        "Verbose mode enabled - QUIT command",
			verboseMode: true,
			command:     "QUIT\r\n",
			wantOutput:  true,
			wantContain: ">>> QUIT",
		},
		{
			name:        "Verbose mode disabled - no output",
			verboseMode: false,
			command:     "EHLO smtptool.local\r\n",
			wantOutput:  false,
			wantContain: "",
		},
		{
			name:        "Verbose mode enabled - empty command",
			verboseMode: true,
			command:     "",
			wantOutput:  true,
			wantContain: ">>>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create SMTPClient with test config
			config := &Config{
				VerboseMode: tt.verboseMode,
			}
			client := &SMTPClient{
				config: config,
			}

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call debug log command
			client.debugLogCommand(tt.command)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			// Verify output
			if tt.wantOutput {
				if !strings.Contains(output, tt.wantContain) {
					t.Errorf("debugLogCommand() output missing expected content\ngot:  %q\nwant: %q", output, tt.wantContain)
				}
				if !strings.Contains(output, ">>>") {
					t.Error("debugLogCommand() output missing >>> prefix")
				}
			} else {
				if output != "" {
					t.Errorf("debugLogCommand() produced output when verbose mode disabled: %q", output)
				}
			}
		})
	}
}

// TestDebugLogResponse tests debug logging of SMTP responses
func TestDebugLogResponse(t *testing.T) {
	tests := []struct {
		name        string
		verboseMode bool
		response    *protocol.SMTPResponse
		wantOutput  bool
		wantContain []string
	}{
		{
			name:        "Verbose mode enabled - single line response",
			verboseMode: true,
			response: &protocol.SMTPResponse{
				Code:    250,
				Message: "OK",
				Lines:   []string{"OK"},
			},
			wantOutput:  true,
			wantContain: []string{"<<< 250 OK"},
		},
		{
			name:        "Verbose mode enabled - multiline response",
			verboseMode: true,
			response: &protocol.SMTPResponse{
				Code:    250,
				Message: "smtp.example.com\nSTARTTLS\nAUTH PLAIN LOGIN",
				Lines:   []string{"smtp.example.com", "STARTTLS", "AUTH PLAIN LOGIN"},
			},
			wantOutput:  true,
			wantContain: []string{"<<< 250-smtp.example.com", "<<< 250-STARTTLS", "<<< 250 AUTH PLAIN LOGIN"},
		},
		{
			name:        "Verbose mode enabled - banner response",
			verboseMode: true,
			response: &protocol.SMTPResponse{
				Code:    220,
				Message: "smtp.gmail.com ESMTP",
				Lines:   []string{"smtp.gmail.com ESMTP"},
			},
			wantOutput:  true,
			wantContain: []string{"<<< 220 smtp.gmail.com ESMTP"},
		},
		{
			name:        "Verbose mode enabled - error response",
			verboseMode: true,
			response: &protocol.SMTPResponse{
				Code:    550,
				Message: "Mailbox not found",
				Lines:   []string{"Mailbox not found"},
			},
			wantOutput:  true,
			wantContain: []string{"<<< 550 Mailbox not found"},
		},
		{
			name:        "Verbose mode disabled - no output",
			verboseMode: false,
			response: &protocol.SMTPResponse{
				Code:    250,
				Message: "OK",
				Lines:   []string{"OK"},
			},
			wantOutput:  false,
			wantContain: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create SMTPClient with test config
			config := &Config{
				VerboseMode: tt.verboseMode,
			}
			client := &SMTPClient{
				config: config,
			}

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call debug log response
			client.debugLogResponse(tt.response)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			// Verify output
			if tt.wantOutput {
				for _, expectedContent := range tt.wantContain {
					if !strings.Contains(output, expectedContent) {
						t.Errorf("debugLogResponse() output missing expected content\ngot:  %q\nwant: %q", output, expectedContent)
					}
				}
				if !strings.Contains(output, "<<<") {
					t.Error("debugLogResponse() output missing <<< prefix")
				}
			} else {
				if output != "" {
					t.Errorf("debugLogResponse() produced output when verbose mode disabled: %q", output)
				}
			}
		})
	}
}

// TestDebugLogMessage tests debug logging of informational messages
func TestDebugLogMessage(t *testing.T) {
	tests := []struct {
		name        string
		verboseMode bool
		message     string
		wantOutput  bool
		wantContain string
	}{
		{
			name:        "Verbose mode enabled - TLS handshake message",
			verboseMode: true,
			message:     "Performing TLS handshake...",
			wantOutput:  true,
			wantContain: "... Performing TLS handshake...",
		},
		{
			name:        "Verbose mode enabled - auth mechanism message",
			verboseMode: true,
			message:     "Starting authentication with mechanism: PLAIN",
			wantOutput:  true,
			wantContain: "... Starting authentication with mechanism: PLAIN",
		},
		{
			name:        "Verbose mode disabled - no output",
			verboseMode: false,
			message:     "This should not appear",
			wantOutput:  false,
			wantContain: "",
		},
		{
			name:        "Verbose mode enabled - empty message",
			verboseMode: true,
			message:     "",
			wantOutput:  true,
			wantContain: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create SMTPClient with test config
			config := &Config{
				VerboseMode: tt.verboseMode,
			}
			client := &SMTPClient{
				config: config,
			}

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call debug log message
			client.debugLogMessage(tt.message)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			// Verify output
			if tt.wantOutput {
				if !strings.Contains(output, tt.wantContain) {
					t.Errorf("debugLogMessage() output missing expected content\ngot:  %q\nwant: %q", output, tt.wantContain)
				}
				if !strings.Contains(output, "...") {
					t.Error("debugLogMessage() output missing ... prefix")
				}
			} else {
				if output != "" {
					t.Errorf("debugLogMessage() produced output when verbose mode disabled: %q", output)
				}
			}
		})
	}
}

// TestDebugLogging_NilSafety tests that debug logging handles nil values safely
func TestDebugLogging_NilSafety(t *testing.T) {
	t.Run("debugLogResponse with nil response", func(t *testing.T) {
		config := &Config{
			VerboseMode: true,
		}
		client := &SMTPClient{
			config: config,
		}

		// This should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("debugLogResponse() panicked with nil response: %v", r)
			}
		}()

		client.debugLogResponse(nil)
	})

	t.Run("debugLogCommand with nil config", func(t *testing.T) {
		client := &SMTPClient{
			config: nil,
		}

		// This should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("debugLogCommand() panicked with nil config: %v", r)
			}
		}()

		client.debugLogCommand("EHLO test\r\n")
	})
}

// TestDebugLogging_MultilineResponseFormatting tests correct formatting of multiline SMTP responses
func TestDebugLogging_MultilineResponseFormatting(t *testing.T) {
	config := &Config{
		VerboseMode: true,
	}
	client := &SMTPClient{
		config: config,
	}

	// Create a typical EHLO response with multiple capabilities
	response := &protocol.SMTPResponse{
		Code:    250,
		Message: "smtp.example.com\nSIZE 35882577\n8BITMIME\nSTARTTLS\nAUTH PLAIN LOGIN",
		Lines: []string{
			"smtp.example.com",
			"SIZE 35882577",
			"8BITMIME",
			"STARTTLS",
			"AUTH PLAIN LOGIN",
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	client.debugLogResponse(response)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify multiline format
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 5 {
		t.Errorf("Expected 5 output lines for multiline response, got %d", len(lines))
	}

	// First 4 lines should have hyphen after code
	for i := 0; i < 4; i++ {
		if !strings.HasPrefix(lines[i], "<<< 250-") {
			t.Errorf("Line %d should start with '<<< 250-', got: %s", i, lines[i])
		}
	}

	// Last line should have space after code
	if !strings.HasPrefix(lines[4], "<<< 250 ") {
		t.Errorf("Last line should start with '<<< 250 ', got: %s", lines[4])
	}
}

// TestSelectAuthMechanism tests authentication mechanism selection
func TestSelectAuthMechanism(t *testing.T) {
	tests := []struct {
		name           string
		requested      []string
		available      []string
		hasAccessToken bool
		expected       string
	}{
		// Auto-selection without access token (prefer CRAM-MD5 > PLAIN > LOGIN)
		{
			name:           "Auto-select CRAM-MD5 when available",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "CRAM-MD5"},
			hasAccessToken: false,
			expected:       "CRAM-MD5",
		},
		{
			name:           "Auto-select PLAIN when CRAM-MD5 not available",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN"},
			hasAccessToken: false,
			expected:       "PLAIN",
		},
		{
			name:           "Auto-select LOGIN when only option",
			requested:      []string{"auto"},
			available:      []string{"LOGIN"},
			hasAccessToken: false,
			expected:       "LOGIN",
		},

		// Auto-selection WITH access token (prefer XOAUTH2)
		{
			name:           "Auto-select XOAUTH2 when access token provided",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "XOAUTH2", "CRAM-MD5"},
			hasAccessToken: true,
			expected:       "XOAUTH2",
		},
		{
			name:           "Fallback to CRAM-MD5 when XOAUTH2 not available but token provided",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "CRAM-MD5"},
			hasAccessToken: true,
			expected:       "CRAM-MD5",
		},

		// Explicit mechanism selection
		{
			name:           "Explicit PLAIN selection",
			requested:      []string{"PLAIN"},
			available:      []string{"LOGIN", "PLAIN", "CRAM-MD5"},
			hasAccessToken: false,
			expected:       "PLAIN",
		},
		{
			name:           "Explicit LOGIN selection",
			requested:      []string{"LOGIN"},
			available:      []string{"LOGIN", "PLAIN", "CRAM-MD5"},
			hasAccessToken: false,
			expected:       "LOGIN",
		},
		{
			name:           "Explicit XOAUTH2 selection",
			requested:      []string{"XOAUTH2"},
			available:      []string{"LOGIN", "PLAIN", "XOAUTH2"},
			hasAccessToken: true,
			expected:       "XOAUTH2",
		},
		{
			name:           "Explicit mechanism not available",
			requested:      []string{"CRAM-MD5"},
			available:      []string{"LOGIN", "PLAIN"},
			hasAccessToken: false,
			expected:       "",
		},

		// GSSAPI auto-selection
		{
			name:           "Auto-select GSSAPI when available (highest priority)",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "GSSAPI", "NTLM", "CRAM-MD5"},
			hasAccessToken: false,
			expected:       "GSSAPI",
		},
		{
			name:           "Auto-select CRAM-MD5 when GSSAPI not available",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "CRAM-MD5"},
			hasAccessToken: false,
			expected:       "CRAM-MD5",
		},
		{
			name:           "Explicit GSSAPI selection",
			requested:      []string{"GSSAPI"},
			available:      []string{"LOGIN", "PLAIN", "GSSAPI"},
			hasAccessToken: false,
			expected:       "GSSAPI",
		},
		{
			name:           "Explicit GSSAPI not available returns empty",
			requested:      []string{"GSSAPI"},
			available:      []string{"LOGIN", "PLAIN", "NTLM"},
			hasAccessToken: false,
			expected:       "",
		},

		// NTLM auto-selection
		{
			name:           "Auto-select NTLM when available and no CRAM-MD5",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "NTLM"},
			hasAccessToken: false,
			expected:       "NTLM",
		},
		{
			name:           "Auto-select CRAM-MD5 over NTLM when both available",
			requested:      []string{"auto"},
			available:      []string{"LOGIN", "PLAIN", "NTLM", "CRAM-MD5"},
			hasAccessToken: false,
			expected:       "CRAM-MD5",
		},
		{
			name:           "Explicit NTLM selection",
			requested:      []string{"NTLM"},
			available:      []string{"LOGIN", "PLAIN", "NTLM"},
			hasAccessToken: false,
			expected:       "NTLM",
		},
		{
			name:           "Explicit NTLM not available returns empty",
			requested:      []string{"NTLM"},
			available:      []string{"LOGIN", "PLAIN"},
			hasAccessToken: false,
			expected:       "",
		},

		// Case insensitivity
		{
			name:           "Case insensitive - lowercase requested",
			requested:      []string{"plain"},
			available:      []string{"PLAIN", "LOGIN"},
			hasAccessToken: false,
			expected:       "PLAIN",
		},
		{
			name:           "Case insensitive - lowercase available",
			requested:      []string{"PLAIN"},
			available:      []string{"plain", "login"},
			hasAccessToken: false,
			expected:       "PLAIN",
		},

		// Edge cases
		{
			name:           "Empty available list",
			requested:      []string{"auto"},
			available:      []string{},
			hasAccessToken: false,
			expected:       "",
		},
		{
			name:           "Empty requested list falls through to auto-select",
			requested:      []string{},
			available:      []string{"PLAIN", "LOGIN"},
			hasAccessToken: false,
			expected:       "PLAIN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectAuthMechanism(tt.requested, tt.available, tt.hasAccessToken)
			if result != tt.expected {
				t.Errorf("selectAuthMechanism(%v, %v, %v) = %q, want %q",
					tt.requested, tt.available, tt.hasAccessToken, result, tt.expected)
			}
		})
	}
}

// TestXOAUTH2Auth tests XOAUTH2 token format generation
func TestXOAUTH2Auth(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		accessToken string
		wantMech    string
		wantContain []string
	}{
		{
			name:        "Standard Gmail format",
			username:    "user@gmail.com",
			accessToken: "ya29.token123",
			wantMech:    "XOAUTH2",
			wantContain: []string{"user=user@gmail.com", "auth=Bearer ya29.token123"},
		},
		{
			name:        "Microsoft 365 format",
			username:    "user@company.onmicrosoft.com",
			accessToken: "eyJ0eXAi.jwt.token",
			wantMech:    "XOAUTH2",
			wantContain: []string{"user=user@company.onmicrosoft.com", "auth=Bearer eyJ0eXAi.jwt.token"},
		},
		{
			name:        "Empty username",
			username:    "",
			accessToken: "token",
			wantMech:    "XOAUTH2",
			wantContain: []string{"user=", "auth=Bearer token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &xoauth2Auth{
				username:    tt.username,
				accessToken: tt.accessToken,
			}

			mechanism, resp, err := auth.Start(nil)
			if err != nil {
				t.Fatalf("xoauth2Auth.Start() error = %v", err)
			}

			if mechanism != tt.wantMech {
				t.Errorf("xoauth2Auth.Start() mechanism = %q, want %q", mechanism, tt.wantMech)
			}

			respStr := string(resp)
			for _, want := range tt.wantContain {
				if !strings.Contains(respStr, want) {
					t.Errorf("xoauth2Auth.Start() response missing %q\ngot: %q", want, respStr)
				}
			}

			// Verify SOH separators (\x01)
			if !strings.Contains(respStr, "\x01") {
				t.Error("xoauth2Auth.Start() response missing SOH (\\x01) separators")
			}

			// Verify ends with double SOH
			if !strings.HasSuffix(respStr, "\x01\x01") {
				t.Error("xoauth2Auth.Start() response should end with \\x01\\x01")
			}
		})
	}
}

// TestXOAUTH2Auth_Next tests XOAUTH2 Next method (single-step auth)
func TestXOAUTH2Auth_Next(t *testing.T) {
	auth := &xoauth2Auth{
		username:    "user@example.com",
		accessToken: "token123",
	}

	// XOAUTH2 is single-step, Next should return nil
	resp, err := auth.Next([]byte("error response"), true)
	if err != nil {
		t.Errorf("xoauth2Auth.Next() unexpected error = %v", err)
	}
	if resp != nil {
		t.Errorf("xoauth2Auth.Next() should return nil, got %v", resp)
	}

	// Also test with more=false
	resp, err = auth.Next(nil, false)
	if err != nil {
		t.Errorf("xoauth2Auth.Next() unexpected error = %v", err)
	}
	if resp != nil {
		t.Errorf("xoauth2Auth.Next() should return nil, got %v", resp)
	}
}

// TestParseKerberosCredentials tests Kerberos username parsing
func TestParseKerberosCredentials(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		wantUser  string
		wantRealm string
	}{
		{
			name:      "user@REALM format",
			username:  "alice@CONTOSO.COM",
			wantUser:  "alice",
			wantRealm: "CONTOSO.COM",
		},
		{
			name:      "user@realm lowercased realm auto-uppercased",
			username:  "alice@contoso.com",
			wantUser:  "alice",
			wantRealm: "CONTOSO.COM",
		},
		{
			name:      "DOMAIN\\user format",
			username:  `CONTOSO\alice`,
			wantUser:  "alice",
			wantRealm: "CONTOSO",
		},
		{
			name:      "domain\\user lowercased domain auto-uppercased",
			username:  `contoso\alice`,
			wantUser:  "alice",
			wantRealm: "CONTOSO",
		},
		{
			name:      "plain username no realm",
			username:  "alice",
			wantUser:  "alice",
			wantRealm: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, realm := parseKerberosCredentials(tt.username)
			if user != tt.wantUser {
				t.Errorf("parseKerberosCredentials(%q) user = %q, want %q", tt.username, user, tt.wantUser)
			}
			if realm != tt.wantRealm {
				t.Errorf("parseKerberosCredentials(%q) realm = %q, want %q", tt.username, realm, tt.wantRealm)
			}
		})
	}
}

// TestGSSAPIAuth tests GSSAPI auth struct behaviour without a live KDC
func TestGSSAPIAuth(t *testing.T) {
	t.Run("Start returns GSSAPI mechanism name", func(t *testing.T) {
		auth := &gssapiAuth{
			username: "alice@CONTOSO.COM",
			password: "secret",
			target:   "exchange.contoso.com",
			// No KDC — Start() will fail on KDC login, but mechanism name is returned first
		}
		// mechanism name is what matters for unit testing; network failure is expected
		mechanism, _, _ := auth.Start(nil)
		if mechanism != "GSSAPI" && mechanism != "" {
			t.Errorf("gssapiAuth.Start() mechanism = %q, want %q or %q", mechanism, "GSSAPI", "")
		}
	})

	t.Run("Start fails with descriptive error when realm cannot be determined", func(t *testing.T) {
		auth := &gssapiAuth{
			username: "alice", // plain username, no realm, no --realm flag
			password: "secret",
			target:   "exchange.contoso.com",
		}
		_, _, err := auth.Start(nil)
		if err == nil {
			t.Error("gssapiAuth.Start() expected error for missing realm, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "realm") {
			t.Errorf("gssapiAuth.Start() error should mention 'realm', got: %v", err)
		}
	})

	t.Run("Next with more=false returns nil (auth complete)", func(t *testing.T) {
		auth := &gssapiAuth{}
		resp, err := auth.Next(nil, false)
		if err != nil {
			t.Errorf("gssapiAuth.Next(more=false) error = %v", err)
		}
		if resp != nil {
			t.Errorf("gssapiAuth.Next(more=false) should return nil, got %v", resp)
		}
	})
}

// TestNTLMAuth tests NTLM negotiate message generation
func TestNTLMAuth(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{
			name:     "Plain username",
			username: "user@example.com",
			password: "secret",
		},
		{
			name:     "Domain-prefixed username",
			username: `CONTOSO\user`,
			password: "secret",
		},
		{
			name:     "Empty domain username",
			username: "user",
			password: "pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &ntlmAuth{
				username: tt.username,
				password: tt.password,
			}

			mechanism, resp, err := auth.Start(nil)
			if err != nil {
				t.Fatalf("ntlmAuth.Start() error = %v", err)
			}

			if mechanism != "NTLM" {
				t.Errorf("ntlmAuth.Start() mechanism = %q, want NTLM", mechanism)
			}

			if len(resp) == 0 {
				t.Fatal("ntlmAuth.Start() returned empty negotiate message")
			}

			// NTLM type-1 message starts with signature "NTLMSSP\x00" then type 1 (little-endian)
			const ntlmSignature = "NTLMSSP\x00"
			if len(resp) < len(ntlmSignature)+4 {
				t.Fatalf("ntlmAuth.Start() response too short: %d bytes", len(resp))
			}
			if string(resp[:len(ntlmSignature)]) != ntlmSignature {
				t.Errorf("ntlmAuth.Start() missing NTLMSSP signature, got: %x", resp[:8])
			}
			// Message type should be 1 (negotiate)
			msgType := uint32(resp[8]) | uint32(resp[9])<<8 | uint32(resp[10])<<16 | uint32(resp[11])<<24
			if msgType != 1 {
				t.Errorf("ntlmAuth.Start() message type = %d, want 1", msgType)
			}

			// Next with more=false should return nil (auth complete)
			resp2, err := auth.Next(nil, false)
			if err != nil {
				t.Errorf("ntlmAuth.Next(more=false) error = %v", err)
			}
			if resp2 != nil {
				t.Errorf("ntlmAuth.Next(more=false) should return nil, got %v", resp2)
			}
		})
	}
}

// TestPlainAuth tests PLAIN authentication token format
func TestPlainAuth(t *testing.T) {
	auth := &plainAuth{
		username: "user@example.com",
		password: "secret123",
	}

	mechanism, resp, err := auth.Start(nil)
	if err != nil {
		t.Fatalf("plainAuth.Start() error = %v", err)
	}

	if mechanism != "PLAIN" {
		t.Errorf("plainAuth.Start() mechanism = %q, want PLAIN", mechanism)
	}

	// PLAIN format: \0username\0password
	expected := "\x00user@example.com\x00secret123"
	if string(resp) != expected {
		t.Errorf("plainAuth.Start() response = %q, want %q", string(resp), expected)
	}
}

// TestLoginAuth tests LOGIN authentication flow
func TestLoginAuth(t *testing.T) {
	auth := &loginAuth{
		username: "user@example.com",
		password: "secret123",
	}

	// Start should return LOGIN with no initial response
	mechanism, resp, err := auth.Start(nil)
	if err != nil {
		t.Fatalf("loginAuth.Start() error = %v", err)
	}
	if mechanism != "LOGIN" {
		t.Errorf("loginAuth.Start() mechanism = %q, want LOGIN", mechanism)
	}
	if resp != nil {
		t.Errorf("loginAuth.Start() should return nil response, got %v", resp)
	}

	// Next should respond to username prompt
	resp, err = auth.Next([]byte("Username:"), true)
	if err != nil {
		t.Errorf("loginAuth.Next(Username) error = %v", err)
	}
	if string(resp) != "user@example.com" {
		t.Errorf("loginAuth.Next(Username) = %q, want user@example.com", string(resp))
	}

	// Next should respond to password prompt
	resp, err = auth.Next([]byte("Password:"), true)
	if err != nil {
		t.Errorf("loginAuth.Next(Password) error = %v", err)
	}
	if string(resp) != "secret123" {
		t.Errorf("loginAuth.Next(Password) = %q, want secret123", string(resp))
	}

	// Next with more=false should return nil
	resp, err = auth.Next(nil, false)
	if err != nil {
		t.Errorf("loginAuth.Next(more=false) error = %v", err)
	}
	if resp != nil {
		t.Errorf("loginAuth.Next(more=false) should return nil, got %v", resp)
	}
}

// TestIsEncrypted tests the IsEncrypted method for SMTPS/STARTTLS state tracking
func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name     string
		tlsState *tls.ConnectionState
		expected bool
	}{
		{
			name:     "No TLS - not encrypted",
			tlsState: nil,
			expected: false,
		},
		{
			name:     "TLS active - encrypted",
			tlsState: &tls.ConnectionState{Version: tls.VersionTLS12},
			expected: true,
		},
		{
			name:     "TLS 1.3 active - encrypted",
			tlsState: &tls.ConnectionState{Version: tls.VersionTLS13},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &SMTPClient{
				tlsState: tt.tlsState,
			}

			result := client.IsEncrypted()
			if result != tt.expected {
				t.Errorf("IsEncrypted() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetTLSState tests the GetTLSState method for retrieving TLS connection info
func TestGetTLSState(t *testing.T) {
	t.Run("Returns nil when no TLS", func(t *testing.T) {
		client := &SMTPClient{
			tlsState: nil,
		}

		state := client.GetTLSState()
		if state != nil {
			t.Errorf("GetTLSState() = %v, want nil", state)
		}
	})

	t.Run("Returns state when TLS active", func(t *testing.T) {
		expectedState := &tls.ConnectionState{
			Version:           tls.VersionTLS12,
			CipherSuite:       tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			ServerName:        "smtp.example.com",
			HandshakeComplete: true,
		}

		client := &SMTPClient{
			tlsState: expectedState,
		}

		state := client.GetTLSState()
		if state == nil {
			t.Fatal("GetTLSState() = nil, want non-nil")
		}
		if state.Version != expectedState.Version {
			t.Errorf("GetTLSState().Version = %v, want %v", state.Version, expectedState.Version)
		}
		if state.ServerName != expectedState.ServerName {
			t.Errorf("GetTLSState().ServerName = %v, want %v", state.ServerName, expectedState.ServerName)
		}
	})
}

// TestSMTPClient_NewSMTPClient tests client initialization
func TestSMTPClient_NewSMTPClient(t *testing.T) {
	config := &Config{
		Timeout:     30 * time.Second,
		VerboseMode: true,
		SMTPS:       true,
		RateLimit:   10.0,
	}

	client := NewSMTPClient("smtp.example.com", 465, config)

	if client.host != "smtp.example.com" {
		t.Errorf("NewSMTPClient() host = %q, want %q", client.host, "smtp.example.com")
	}
	if client.port != 465 {
		t.Errorf("NewSMTPClient() port = %d, want %d", client.port, 465)
	}
	if client.config != config {
		t.Error("NewSMTPClient() config not set correctly")
	}
	if client.limiter == nil {
		t.Error("NewSMTPClient() limiter not initialized")
	}
	// New client should not be encrypted yet
	if client.IsEncrypted() {
		t.Error("NewSMTPClient() should not be encrypted before Connect()")
	}
}

// TestSMTPClient_GetBanner tests banner retrieval
func TestSMTPClient_GetBanner(t *testing.T) {
	t.Run("Empty banner before connect", func(t *testing.T) {
		client := &SMTPClient{}
		if banner := client.GetBanner(); banner != "" {
			t.Errorf("GetBanner() = %q, want empty string", banner)
		}
	})

	t.Run("Returns stored banner", func(t *testing.T) {
		client := &SMTPClient{
			banner: "smtp.example.com ESMTP ready",
		}
		expected := "smtp.example.com ESMTP ready"
		if banner := client.GetBanner(); banner != expected {
			t.Errorf("GetBanner() = %q, want %q", banner, expected)
		}
	})
}

// TestSMTPClient_GetCapabilities tests capabilities retrieval
func TestSMTPClient_GetCapabilities(t *testing.T) {
	t.Run("Empty capabilities before EHLO", func(t *testing.T) {
		client := &SMTPClient{}
		caps := client.GetCapabilities()
		if len(caps) > 0 {
			t.Errorf("GetCapabilities() = %v, want nil or empty", caps)
		}
	})

	t.Run("Returns stored capabilities", func(t *testing.T) {
		caps := protocol.Capabilities{
			"STARTTLS": []string{},
			"AUTH":     []string{"PLAIN", "LOGIN"},
			"SIZE":     []string{"35882577"},
			"8BITMIME": []string{},
		}
		client := &SMTPClient{
			capabilities: caps,
		}

		result := client.GetCapabilities()
		if result == nil {
			t.Fatal("GetCapabilities() = nil, want non-nil")
		}
		if _, ok := result["STARTTLS"]; !ok {
			t.Error("GetCapabilities() missing STARTTLS")
		}
		if _, ok := result["AUTH"]; !ok {
			t.Error("GetCapabilities() missing AUTH")
		}
	})
}

// TestSMTPSConfig tests SMTPS-specific configuration handling
func TestSMTPSConfig(t *testing.T) {
	t.Run("SMTPS config sets correct defaults", func(t *testing.T) {
		config := NewConfig()
		config.SMTPS = true
		config.Host = "smtp.gmail.com"
		config.Action = ActionTestConnect

		// Validate should change port from 25 to 465
		err := validateConfiguration(config)
		if err != nil {
			t.Fatalf("validateConfiguration() error = %v", err)
		}
		if config.Port != 465 {
			t.Errorf("SMTPS config port = %d, want 465", config.Port)
		}
	})

	t.Run("SMTPS with explicit port preserves port", func(t *testing.T) {
		config := NewConfig()
		config.SMTPS = true
		config.Host = "smtp.example.com"
		config.Port = 587 // Explicit non-default port
		config.Action = ActionTestConnect

		err := validateConfiguration(config)
		if err != nil {
			t.Fatalf("validateConfiguration() error = %v", err)
		}
		if config.Port != 587 {
			t.Errorf("SMTPS config port = %d, want 587 (explicit)", config.Port)
		}
	})
}
