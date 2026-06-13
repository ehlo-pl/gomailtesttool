//go:build !integration
// +build !integration

package serve

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/ziembor/gomailtesttool/internal/protocols/msgraph"
	"github.com/ziembor/gomailtesttool/internal/protocols/smtp"
)

// newTestServer builds a Server configured for unit tests.
func newTestServer(smtpBase *smtp.Config, msgraphBase *msgraph.Config) *Server {
	return New(
		&Config{Port: 8080, Listen: "", APIKey: "testkey"},
		smtpBase,
		msgraphBase,
		nil, // graphClient — tests that need it set it manually
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

// baseSmtpConfig returns a minimal smtp.Config sufficient to pass handler
// validation without requiring a real SMTP connection.
func baseSmtpConfig() *smtp.Config {
	cfg := smtp.NewConfig()
	cfg.Host = "smtp.example.com"
	cfg.Port = 587
	cfg.From = "sender@example.com"
	return cfg
}

// baseMsgraphConfig returns a minimal msgraph.Config with enough fields to
// reach the handler body without triggering the startup-level 503.
func baseMsgraphConfig() *msgraph.Config {
	v := viper.New()
	msgraph.BindEnvs(v)
	cfg := msgraph.ConfigFromViper(v)
	cfg.TenantID = "00000000-0000-0000-0000-000000000001"
	cfg.ClientID = "00000000-0000-0000-0000-000000000002"
	cfg.Mailbox = "box@example.com"
	return cfg
}

// serve fires a request through the full middleware+mux stack and returns the recorder.
func serve(t *testing.T, srv *Server, method, path string, body any, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.handleHealth)
	mux.HandleFunc("POST /smtp/sendmail", srv.handleSMTPSendMail)
	mux.HandleFunc("POST /msgraph/sendmail", srv.handleMsgraphSendMail)
	mux.HandleFunc("POST /ews/sendmail", srv.handleEWSSendMail)
	srv.apiKeyMiddleware(mux).ServeHTTP(rr, req)
	return rr
}

func serveRaw(t *testing.T, srv *Server, method, path, rawBody, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /smtp/sendmail", srv.handleSMTPSendMail)
	mux.HandleFunc("POST /msgraph/sendmail", srv.handleMsgraphSendMail)
	srv.apiKeyMiddleware(mux).ServeHTTP(rr, req)
	return rr
}

func decodeResp(t *testing.T, rr *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var resp apiResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response %q: %v", rr.Body.String(), err)
	}
	return resp
}

// --- API key middleware ---

func TestAPIKeyMiddleware(t *testing.T) {
	srv := newTestServer(nil, nil)

	tests := []struct {
		name       string
		method     string
		path       string
		apiKey     string
		wantStatus int
	}{
		{"Health requires no key", http.MethodGet, "/health", "", http.StatusOK},
		{"Health accepts key if present", http.MethodGet, "/health", "testkey", http.StatusOK},
		{"Protected endpoint: missing key → 401", http.MethodPost, "/smtp/sendmail", "", http.StatusUnauthorized},
		{"Protected endpoint: wrong key → 401", http.MethodPost, "/smtp/sendmail", "wrongkey", http.StatusUnauthorized},
		{"Protected endpoint: correct key passes through", http.MethodPost, "/smtp/sendmail", "testkey", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := serve(t, srv, tt.method, tt.path, map[string]any{}, tt.apiKey)
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body)
			}
		})
	}
}

func TestAPIKeyMiddleware_EmptyConfiguredKeyDeniesAll(t *testing.T) {
	srv := newTestServer(nil, nil)
	srv.config.APIKey = ""

	rr := serve(t, srv, http.MethodPost, "/smtp/sendmail", map[string]any{}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when configured API key is empty (body: %s)", rr.Code, rr.Body)
	}
}

func TestAPIKeyMiddleware_401Body(t *testing.T) {
	srv := newTestServer(nil, nil)
	rr := serve(t, srv, http.MethodPost, "/smtp/sendmail", nil, "bad")

	resp := decodeResp(t, rr)
	if resp.Status != "error" {
		t.Errorf("status field = %q, want error", resp.Status)
	}
	if !strings.Contains(resp.Message, "X-API-Key") {
		t.Errorf("message %q should mention X-API-Key", resp.Message)
	}
}

// --- Health ---

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(nil, nil)
	rr := serve(t, srv, http.MethodGet, "/health", nil, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
	if body["version"] == "" {
		t.Error("version field is empty")
	}
}

// --- EWS ---

func TestHandleEWSSendMail_NotImplemented(t *testing.T) {
	srv := newTestServer(nil, nil)
	rr := serve(t, srv, http.MethodPost, "/ews/sendmail", map[string]any{}, "testkey")

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rr.Code)
	}
	resp := decodeResp(t, rr)
	if resp.Status != "error" {
		t.Errorf("status field = %q, want error", resp.Status)
	}
}

// --- SMTP handler ---

func TestHandleSMTPSendMail_NilBase_Returns503(t *testing.T) {
	srv := newTestServer(nil, nil)
	rr := serve(t, srv, http.MethodPost, "/smtp/sendmail",
		map[string]any{"to": []string{"a@b.com"}, "subject": "hi"},
		"testkey")

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestHandleSMTPSendMail_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		rawBody    string // used instead of body when non-empty
		smtpFrom   string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "Missing to field",
			body:       map[string]any{"subject": "Test"},
			smtpFrom:   "sender@example.com",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "to is required",
		},
		{
			name:       "Empty to array",
			body:       map[string]any{"to": []string{}, "subject": "Test"},
			smtpFrom:   "sender@example.com",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "to is required",
		},
		{
			name:       "Missing subject",
			body:       map[string]any{"to": []string{"a@b.com"}},
			smtpFrom:   "sender@example.com",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "subject is required",
		},
		{
			name:       "No from anywhere → 400",
			body:       map[string]any{"to": []string{"a@b.com"}, "subject": "Test"},
			smtpFrom:   "",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "from is required",
		},
		{
			name:       "Invalid email in to",
			body:       map[string]any{"to": []string{"notanemail"}, "subject": "Test"},
			smtpFrom:   "sender@example.com",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid to address",
		},
		{
			name:       "Invalid from in request body",
			body:       map[string]any{"to": []string{"a@b.com"}, "subject": "Test", "from": "notanemail"},
			smtpFrom:   "sender@example.com",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid from address",
		},
		{
			name:       "Invalid JSON body",
			rawBody:    `{bad json`,
			smtpFrom:   "sender@example.com",
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := baseSmtpConfig()
			base.From = tt.smtpFrom
			srv := newTestServer(base, nil)

			var rr *httptest.ResponseRecorder
			if tt.rawBody != "" {
				rr = serveRaw(t, srv, http.MethodPost, "/smtp/sendmail", tt.rawBody, "testkey")
			} else {
				rr = serve(t, srv, http.MethodPost, "/smtp/sendmail", tt.body, "testkey")
			}

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body)
			}
			resp := decodeResp(t, rr)
			if !strings.Contains(resp.Message, tt.wantMsg) {
				t.Errorf("message = %q, want to contain %q", resp.Message, tt.wantMsg)
			}
		})
	}
}

func TestHandleSMTPSendMail_FromFallback(t *testing.T) {
	// When "from" is absent from the request, base config From should be used.
	// Port 1 ensures the SMTP connection fails immediately (not a validation error).
	base := baseSmtpConfig()
	base.From = "base@example.com"
	base.Host = "127.0.0.1"
	base.Port = 1

	srv := newTestServer(base, nil)
	rr := serve(t, srv, http.MethodPost, "/smtp/sendmail",
		map[string]any{"to": []string{"a@b.com"}, "subject": "Test"},
		"testkey")

	if rr.Code == http.StatusBadRequest {
		t.Errorf("got 400 but expected validation to pass (from should come from base config): %s", rr.Body)
	}
}

// --- MS Graph handler ---

func TestHandleMsgraphSendMail_NilBase_Returns503(t *testing.T) {
	srv := newTestServer(nil, nil)
	rr := serve(t, srv, http.MethodPost, "/msgraph/sendmail",
		map[string]any{"to": []string{"a@b.com"}, "subject": "test"},
		"testkey")

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestHandleMsgraphSendMail_Validation(t *testing.T) {
	// msgraphBase is set but graphClient remains nil.
	// After the handler restructure, validation (400) fires before the graphClient check.
	tests := []struct {
		name       string
		body       any
		rawBody    string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "No recipients at all",
			body:       map[string]any{"subject": "Test"},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "at least one recipient",
		},
		{
			name:       "Empty to, cc, bcc arrays",
			body:       map[string]any{"to": []string{}, "cc": []string{}, "bcc": []string{}, "subject": "Test"},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "at least one recipient",
		},
		{
			name:       "Missing subject",
			body:       map[string]any{"to": []string{"a@b.com"}},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "subject is required",
		},
		{
			name:       "Invalid email in to",
			body:       map[string]any{"to": []string{"notanemail"}, "subject": "Test"},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid recipient address",
		},
		{
			name:       "Invalid email in cc",
			body:       map[string]any{"to": []string{"a@b.com"}, "cc": []string{"bad"}, "subject": "Test"},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid recipient address",
		},
		{
			name:       "Invalid email in bcc",
			body:       map[string]any{"to": []string{"a@b.com"}, "bcc": []string{"bad"}, "subject": "Test"},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid recipient address",
		},
		{
			name:       "Invalid JSON",
			rawBody:    `{bad json`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(nil, baseMsgraphConfig()) // graphClient stays nil

			var rr *httptest.ResponseRecorder
			if tt.rawBody != "" {
				rr = serveRaw(t, srv, http.MethodPost, "/msgraph/sendmail", tt.rawBody, "testkey")
			} else {
				rr = serve(t, srv, http.MethodPost, "/msgraph/sendmail", tt.body, "testkey")
			}

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body)
			}
			resp := decodeResp(t, rr)
			if !strings.Contains(resp.Message, tt.wantMsg) {
				t.Errorf("message = %q, want to contain %q", resp.Message, tt.wantMsg)
			}
		})
	}
}

func TestHandleMsgraphSendMail_ValidRequest_NilClient_Returns503(t *testing.T) {
	// A valid request with msgraphBase set but no graphClient should get 503,
	// not 400. This confirms validation passed and the client check fires second.
	srv := newTestServer(nil, baseMsgraphConfig()) // graphClient = nil

	rr := serve(t, srv, http.MethodPost, "/msgraph/sendmail",
		map[string]any{"to": []string{"a@b.com"}, "subject": "Test"},
		"testkey")

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (body: %s)", rr.Code, rr.Body)
	}
}

func TestHandleMsgraphSendMail_AttachmentsFieldIgnored(t *testing.T) {
	// "attachments" was removed from the HTTP API to prevent path traversal.
	// A request that includes it should be decoded without error (JSON unknown
	// fields are silently ignored) and fail only at the graphClient nil check.
	srv := newTestServer(nil, baseMsgraphConfig()) // graphClient = nil

	rr := serve(t, srv, http.MethodPost, "/msgraph/sendmail",
		map[string]any{
			"to":          []string{"a@b.com"},
			"subject":     "Test",
			"attachments": []string{"/etc/passwd"}, // must not cause path traversal
		},
		"testkey")

	// Should be 503 (validation passed, client nil), NOT 400
	if rr.Code == http.StatusBadRequest {
		t.Errorf("attachments field caused a 400 — it should be silently ignored: %s", rr.Body)
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (body: %s)", rr.Code, rr.Body)
	}
}
