package serve

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

// newMCPServer builds an MCP server that exposes the same sendmail capabilities
// as the REST endpoints. The tools reuse the transport-agnostic send core
// (sendSMTP / sendMsgraph), so validation, credential handling, and the
// no-attachments restriction match the HTTP handlers exactly.
//
// Tools are always registered even when their backend is unconfigured; a call
// then returns an error result (mirroring the REST 503) rather than the tool
// being invisible. This keeps parity and is simpler than conditional
// registration.
func (s *Server) newMCPServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "gomailtest-serve",
		Version: version.Get(),
	}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "smtp_sendmail",
		Description: "Send an email via SMTP using the server's configured SMTP credentials. Only message content is supplied per call; connection credentials come from the server's startup environment.",
	}, s.mcpSMTPSendMail)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "msgraph_sendmail",
		Description: "Send an email via Microsoft Graph (Exchange Online) using the server's configured credentials. Only message content is supplied per call; attachments are not supported.",
	}, s.mcpMsgraphSendMail)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_backends",
		Description: "Report which send backends (SMTP, Microsoft Graph) are configured and available on this server.",
	}, s.mcpListBackends)

	return srv
}

// mcpSMTPSendMail is the smtp_sendmail tool handler. It reuses sendSMTP and maps
// a classified send failure to an error tool result. CSV logging is disabled
// (nil) for MCP calls: in stdio mode the logger's "Logging to:" stdout banner
// would corrupt the JSON-RPC channel, and the send core tolerates nil.
func (s *Server) mcpSMTPSendMail(ctx context.Context, _ *mcp.CallToolRequest, req smtpSendRequest) (*mcp.CallToolResult, any, error) {
	if se := s.sendSMTP(ctx, req, nil); se != nil {
		return toolError(se.msg), nil, nil
	}
	return toolText("Email sent via SMTP."), nil, nil
}

// mcpMsgraphSendMail is the msgraph_sendmail tool handler.
func (s *Server) mcpMsgraphSendMail(ctx context.Context, _ *mcp.CallToolRequest, req msgraphSendRequest) (*mcp.CallToolResult, any, error) {
	if se := s.sendMsgraph(ctx, req, nil); se != nil {
		return toolError(se.msg), nil, nil
	}
	return toolText("Email sent via Microsoft Graph."), nil, nil
}

// mcpListBackends is the list_backends tool handler. It takes no arguments.
func (s *Server) mcpListBackends(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	smtpAvailable := s.smtpBase != nil
	graphAvailable := s.msgraphBase != nil && s.graphClient != nil
	msg := fmt.Sprintf(
		"gomailtest serve (version %s)\nBackends:\n- smtp_sendmail: %s\n- msgraph_sendmail: %s",
		version.Get(), availability(smtpAvailable), availability(graphAvailable),
	)
	return toolText(msg), nil, nil
}

func availability(ok bool) string {
	if ok {
		return "available"
	}
	return "not configured"
}

// toolText returns a successful text tool result.
func toolText(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// toolError returns an error tool result carrying msg as text. Business-logic
// failures are reported this way (IsError) rather than as a protocol error, so
// the calling model sees the message.
func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// RunMCPStdio serves the MCP server over stdio, reading from r and writing the
// JSON-RPC channel to w. It blocks until ctx is cancelled or the connection
// closes.
//
// The caller (NewCmd) is responsible for preserving the real stdout as w and
// repointing the process's os.Stdout to stderr *before* any logging is
// initialised — the reused send functions and the CSV logger write
// human-readable progress to stdout, which would otherwise corrupt the protocol.
func (s *Server) RunMCPStdio(ctx context.Context, r io.ReadCloser, w io.WriteCloser) error {
	s.logger.Info("Starting MCP server over stdio")
	err := s.newMCPServer().Run(ctx, &mcp.IOTransport{Reader: r, Writer: w})
	// A closed input stream (client disconnected) or a cancelled context (SIGINT/
	// SIGTERM) is a normal end of session for a stdio server, not an error.
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// nopWriteCloser wraps an io.Writer so that Close is a no-op, preventing the MCP
// transport from closing the underlying (real stdout) file descriptor on
// shutdown.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
