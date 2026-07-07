//go:build !integration
// +build !integration

package serve

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolResultText concatenates the text of every TextContent block in a tool
// result, failing the test if a non-text content block is encountered.
func toolResultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			t.Fatalf("unexpected non-text content %T", c)
		}
		b.WriteString(tc.Text)
	}
	return b.String()
}

// TestNewMCPServer_Registers ensures the MCP server builds and every tool's
// input schema can be inferred from its request struct. AddTool panics on an
// un-inferable schema, so simply constructing the server exercises that.
func TestNewMCPServer_Registers(t *testing.T) {
	srv := newTestServer(baseSmtpConfig(), baseMsgraphConfig())
	if got := srv.newMCPServer(); got == nil {
		t.Fatal("newMCPServer returned nil")
	}
}

// TestHandler_NoPatternConflict guards against http.ServeMux pattern conflicts
// (which panic at registration): mounting /mcp must not collide with "GET /".
func TestHandler_NoPatternConflict(t *testing.T) {
	srv := newTestServer(nil, nil)
	srv.config.EnableMCP = true
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building handler panicked (mux pattern conflict?): %v", r)
		}
	}()
	if srv.handler() == nil {
		t.Fatal("handler() returned nil")
	}
}

// --- send core classification ---

func TestSendSMTP_Classification(t *testing.T) {
	tests := []struct {
		name     string
		base     bool // whether an SMTP base config is present
		req      smtpSendRequest
		wantKind sendErrKind
		wantMsg  string
	}{
		{"Nil base → unavailable", false,
			smtpSendRequest{To: []string{"a@b.com"}, Subject: "hi"}, sendErrUnavailable, "SMTP not configured"},
		{"Missing to → bad request", true,
			smtpSendRequest{Subject: "hi"}, sendErrBadRequest, "to is required"},
		{"Missing subject → bad request", true,
			smtpSendRequest{To: []string{"a@b.com"}}, sendErrBadRequest, "subject is required"},
		{"Invalid to → bad request", true,
			smtpSendRequest{To: []string{"notanemail"}, Subject: "hi"}, sendErrBadRequest, "invalid to address"},
		{"Invalid priority → bad request", true,
			smtpSendRequest{To: []string{"a@b.com"}, Subject: "hi", Priority: "urgent"}, sendErrBadRequest, "invalid priority"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var srv *Server
			if tt.base {
				srv = newTestServer(baseSmtpConfig(), nil)
			} else {
				srv = newTestServer(nil, nil)
			}

			se := srv.sendSMTP(context.Background(), tt.req, nil)
			if se == nil {
				t.Fatalf("expected a send error, got nil")
			}
			if se.kind != tt.wantKind {
				t.Errorf("kind = %d, want %d", se.kind, tt.wantKind)
			}
			if !strings.Contains(se.msg, tt.wantMsg) {
				t.Errorf("msg = %q, want to contain %q", se.msg, tt.wantMsg)
			}
		})
	}
}

func TestSendMsgraph_Classification(t *testing.T) {
	tests := []struct {
		name     string
		base     bool // msgraph base present (graphClient stays nil)
		req      msgraphSendRequest
		wantKind sendErrKind
		wantMsg  string
	}{
		{"Nil base → unavailable", false,
			msgraphSendRequest{To: []string{"a@b.com"}, Subject: "hi"}, sendErrUnavailable, "Microsoft Graph not configured"},
		{"No recipients → bad request", true,
			msgraphSendRequest{Subject: "hi"}, sendErrBadRequest, "at least one recipient"},
		{"Missing subject → bad request", true,
			msgraphSendRequest{To: []string{"a@b.com"}}, sendErrBadRequest, "subject is required"},
		{"Invalid recipient → bad request", true,
			msgraphSendRequest{To: []string{"bad"}, Subject: "hi"}, sendErrBadRequest, "invalid recipient address"},
		{"Valid but nil client → unavailable", true,
			msgraphSendRequest{To: []string{"a@b.com"}, Subject: "hi"}, sendErrUnavailable, "client not initialised"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var srv *Server
			if tt.base {
				srv = newTestServer(nil, baseMsgraphConfig()) // graphClient nil
			} else {
				srv = newTestServer(nil, nil)
			}

			se := srv.sendMsgraph(context.Background(), tt.req, nil)
			if se == nil {
				t.Fatalf("expected a send error, got nil")
			}
			if se.kind != tt.wantKind {
				t.Errorf("kind = %d, want %d", se.kind, tt.wantKind)
			}
			if !strings.Contains(se.msg, tt.wantMsg) {
				t.Errorf("msg = %q, want to contain %q", se.msg, tt.wantMsg)
			}
		})
	}
}

// --- MCP tool handlers report failures as error results (IsError) ---

func TestMCPSMTPSendMail_UnconfiguredBackend(t *testing.T) {
	srv := newTestServer(nil, nil)
	res, _, err := srv.mcpSMTPSendMail(context.Background(), nil,
		smtpSendRequest{To: []string{"a@b.com"}, Subject: "hi"})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError result for unconfigured SMTP backend")
	}
	if msg := toolResultText(t, res); !strings.Contains(msg, "SMTP not configured") {
		t.Errorf("message %q should mention SMTP not configured", msg)
	}
}

func TestMCPSMTPSendMail_ValidationError(t *testing.T) {
	srv := newTestServer(baseSmtpConfig(), nil)
	res, _, err := srv.mcpSMTPSendMail(context.Background(), nil,
		smtpSendRequest{To: []string{"a@b.com"}}) // missing subject
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError result for missing subject")
	}
	if msg := toolResultText(t, res); !strings.Contains(msg, "subject is required") {
		t.Errorf("message %q should mention subject is required", msg)
	}
}

func TestMCPMsgraphSendMail_UnconfiguredBackend(t *testing.T) {
	srv := newTestServer(nil, nil)
	res, _, err := srv.mcpMsgraphSendMail(context.Background(), nil,
		msgraphSendRequest{To: []string{"a@b.com"}, Subject: "hi"})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError result for unconfigured Graph backend")
	}
	if msg := toolResultText(t, res); !strings.Contains(msg, "Microsoft Graph not configured") {
		t.Errorf("message %q should mention Microsoft Graph not configured", msg)
	}
}

func TestMCPListBackends(t *testing.T) {
	srv := newTestServer(baseSmtpConfig(), nil) // SMTP available, Graph not
	res, _, err := srv.mcpListBackends(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Error("list_backends should not be an error result")
	}
	msg := toolResultText(t, res)
	if !strings.Contains(msg, "smtp_sendmail: available") {
		t.Errorf("message %q should report smtp_sendmail available", msg)
	}
	if !strings.Contains(msg, "msgraph_sendmail: not configured") {
		t.Errorf("message %q should report msgraph_sendmail not configured", msg)
	}
}
