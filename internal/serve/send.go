package serve

import (
	"context"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// sendErrKind classifies a send failure so each transport (HTTP REST, MCP) can
// map it to its own status model: HTTP maps to 400/503/500, MCP to an error
// tool result.
type sendErrKind int

const (
	sendErrBadRequest  sendErrKind = iota // invalid input (HTTP 400)
	sendErrUnavailable                    // backend not configured (HTTP 503)
	sendErrInternal                       // send failed (HTTP 500)
)

// sendError is the classified error returned by the transport-agnostic send core.
// msg is the user-facing message; it mirrors the strings the HTTP API returned
// before the transport split, so behavior is unchanged for REST clients.
type sendError struct {
	kind sendErrKind
	msg  string
}

func (e *sendError) Error() string { return e.msg }

func badRequest(msg string) *sendError  { return &sendError{kind: sendErrBadRequest, msg: msg} }
func unavailable(msg string) *sendError { return &sendError{kind: sendErrUnavailable, msg: msg} }
func internalError(msg string) *sendError {
	return &sendError{kind: sendErrInternal, msg: msg}
}

// sanitizeEmailSubjectInput strips CR/LF from a subject to prevent header
// injection before it reaches the SMTP layer.
func sanitizeEmailSubjectInput(subject string) string {
	subject = strings.ReplaceAll(subject, "\r", "")
	subject = strings.ReplaceAll(subject, "\n", "")
	return strings.TrimSpace(subject)
}

// sanitizeEmailBodyInput normalises line endings and drops control characters
// (other than newline/tab) from a message body.
func sanitizeEmailBodyInput(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	var b strings.Builder
	b.Grow(len(body))
	for _, r := range body {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sendSMTP validates req against the server's SMTP base config and sends the
// message. It is transport-agnostic — both the REST handler and the MCP tool
// call it. csvLogger may be nil (e.g. stdio MCP mode), in which case no result
// row is written; the underlying SendMail tolerates a nil logger.
func (s *Server) sendSMTP(ctx context.Context, req smtpSendRequest, csvLogger logger.Logger) *sendError {
	if s.smtpBase == nil {
		return unavailable("SMTP not configured (set SMTPHOST and related env vars)")
	}

	if len(req.To) == 0 {
		return badRequest("to is required")
	}
	if req.Subject == "" {
		return badRequest("subject is required")
	}
	if req.From == "" && s.smtpBase.From == "" {
		return badRequest("from is required (set SMTPFROM or provide 'from' in request body)")
	}
	if req.From != "" {
		if err := validation.ValidateEmail(req.From); err != nil {
			return badRequest("invalid from address: " + err.Error())
		}
	}
	for _, addr := range req.To {
		if err := validation.ValidateEmail(addr); err != nil {
			return badRequest("invalid to address " + addr + ": " + err.Error())
		}
	}
	for _, addr := range req.Cc {
		if err := validation.ValidateEmail(addr); err != nil {
			return badRequest("invalid cc address " + addr + ": " + err.Error())
		}
	}
	for _, addr := range req.Bcc {
		if err := validation.ValidateEmail(addr); err != nil {
			return badRequest("invalid bcc address " + addr + ": " + err.Error())
		}
	}
	if req.Priority != "" && !validPriority(req.Priority) {
		return badRequest("invalid priority: " + req.Priority + " (must be one of: high, normal, low)")
	}

	// Clone base config and overlay request content.
	cfg := *s.smtpBase
	cfg.To = req.To
	cfg.Cc = req.Cc
	cfg.Bcc = req.Bcc
	cfg.Subject = sanitizeEmailSubjectInput(req.Subject)
	cfg.Body = sanitizeEmailBodyInput(req.Body)
	cfg.Action = smtp.ActionSendMail
	if req.From != "" {
		cfg.From = req.From
	}
	if req.Priority != "" {
		cfg.Priority = strings.ToLower(req.Priority)
	}

	sendCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if err := smtp.SendMail(sendCtx, &cfg, csvLogger, s.logger); err != nil {
		s.logger.Error("SMTP sendmail failed", "error", err)
		return internalError(err.Error())
	}
	return nil
}

// sendMsgraph validates req against the server's MS Graph base config and sends
// the message via Microsoft Graph. Transport-agnostic; csvLogger may be nil.
// Attachments are intentionally unsupported (the REST API omits them to avoid
// letting callers read arbitrary server-side files), and the MCP tool inherits
// that restriction by reusing this core.
func (s *Server) sendMsgraph(ctx context.Context, req msgraphSendRequest, csvLogger logger.Logger) *sendError {
	if s.msgraphBase == nil {
		return unavailable("Microsoft Graph not configured (set MSGRAPHTENANTID, MSGRAPHCLIENTID and auth env vars)")
	}

	if len(req.To) == 0 && len(req.Cc) == 0 && len(req.Bcc) == 0 {
		return badRequest("at least one recipient is required (to, cc, or bcc)")
	}
	if req.Subject == "" {
		return badRequest("subject is required")
	}
	for _, list := range [][]string{req.To, req.Cc, req.Bcc} {
		for _, addr := range list {
			if err := validation.ValidateEmail(addr); err != nil {
				return badRequest("invalid recipient address " + addr + ": " + err.Error())
			}
		}
	}
	if req.Priority != "" && !validPriority(req.Priority) {
		return badRequest("invalid priority: " + req.Priority + " (must be one of: high, normal, low)")
	}

	// Validation passed — now require a live client.
	if s.graphClient == nil {
		return unavailable("Microsoft Graph client not initialised (check auth credentials)")
	}

	// Clone base config for runtime settings (VerboseMode, retries, mailbox, etc.).
	// Recipients are passed directly to SendEmail.
	cfg := *s.msgraphBase
	cfg.Action = msgraph.ActionSendMail
	if req.Priority != "" {
		cfg.Priority = strings.ToLower(req.Priority)
	}

	sendCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if err := msgraph.SendEmail(sendCtx, s.graphClient, cfg.Mailbox, req.To, req.Cc, req.Bcc, req.Subject, req.Body, req.BodyHTML, nil, &cfg, csvLogger); err != nil {
		s.logger.Error("Graph sendmail failed", "error", err)
		return internalError(err.Error())
	}
	return nil
}

// validPriority reports whether p is one of the accepted priority values
// (case-insensitive).
func validPriority(p string) bool {
	switch strings.ToLower(p) {
	case "high", "normal", "low":
		return true
	default:
		return false
	}
}
