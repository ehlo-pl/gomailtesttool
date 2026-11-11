// Package send defines a small, protocol-neutral API for submitting a
// ready-made RFC 5322 message. It is the sending twin of package fetch:
// protocol packages (smtp, msgraph) implement Sender; tools such as
// testmailflow consume it.
//
// Implementations must be quiet (no fmt.Print to stdout, no CSV rows);
// progress goes to the *slog.Logger passed at creation.
package send

import "context"

// Message is one message to submit.
type Message struct {
	// From is the envelope sender (SMTP MAIL FROM). Graph ignores it and
	// sends as the configured mailbox.
	From string
	// To holds the envelope recipients (SMTP RCPT TO). Graph derives the
	// recipients from the message headers.
	To []string
	// Raw is the complete RFC 5322 message with CRLF line endings.
	Raw []byte
}

// Result describes an accepted submission.
type Result struct {
	// ServerResponse is the server's final reply (SMTP "250 ..." text), if any.
	ServerResponse string
	// RemoteHost is the server that accepted the message (resolved MX for SMTP).
	RemoteHost string
	// TLSVersion is the negotiated TLS version ("" when the session was not encrypted).
	TLSVersion string
}

// Sender submits messages over one configured endpoint. Implementations must
// be safe for sequential reuse; callers serialize access per Sender.
type Sender interface {
	Send(ctx context.Context, m Message) (Result, error)
	Close() error
}
