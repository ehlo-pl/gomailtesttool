// Package fetch defines a small, protocol-neutral API for streaming raw
// RFC 5322 messages (.eml) out of a mailbox. Protocol packages (imap, pop3,
// msgraph, gmail) implement Session; tools such as dmarcanalyzer consume it.
//
// Implementations must be quiet (no fmt.Print to stdout); progress goes to
// the *slog.Logger passed at session creation.
package fetch

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Criteria selects the messages to fetch. Zero values mean "no filter".
type Criteria struct {
	// Folder is the IMAP mailbox, Graph mail folder (id or well-known name)
	// or Gmail label. Empty means the protocol default (INBOX / inbox).
	// POP3 has a single mailbox and ignores it.
	Folder string
	// Since/Before bound the received date (Before is exclusive). IMAP
	// matches whole days; POP3 uses the Date header.
	Since, Before time.Time
	// Subject is a case-insensitive substring filter, applied client-side
	// where the server cannot do it safely.
	Subject string
	// Query is a raw provider query (Gmail search syntax only).
	Query string
	// Max limits the number of messages delivered to the sink (0 = no limit).
	Max int
	// SkipKey, if set, is consulted before downloading a message body so
	// already-fetched messages are not downloaded again.
	SkipKey func(key string) bool
}

// RawMessage is one downloaded message.
type RawMessage struct {
	// Key is a stable, protocol-specific identifier usable with Session.Delete
	// (IMAP "<uidvalidity>:<uid>", POP3 UIDL, Graph message id, Gmail id).
	Key      string
	Received time.Time // zero if unknown
	Raw      []byte
}

// Sink receives messages. Returning an error stops the fetch.
type Sink func(ctx context.Context, m RawMessage) error

// Session is an authenticated connection to one mailbox.
type Session interface {
	// Fetch streams matching messages to sink.
	Fetch(ctx context.Context, c Criteria, sink Sink) error
	// Delete removes (or, for Gmail, trashes) a message by key.
	Delete(ctx context.Context, key string) error
	// Close ends the session. For POP3, deletions are committed here.
	Close() error
}

// ErrStop can be returned by a sink to end fetching early without error.
var ErrStop = errors.New("stop fetching")

// SubjectMatches applies the case-insensitive Subject filter.
func (c Criteria) SubjectMatches(subject string) bool {
	return c.Subject == "" || strings.Contains(strings.ToLower(subject), strings.ToLower(c.Subject))
}

// InRange reports whether t satisfies Since/Before (unknown t always matches).
func (c Criteria) InRange(t time.Time) bool {
	if t.IsZero() {
		return true
	}
	if !c.Since.IsZero() && t.Before(c.Since) {
		return false
	}
	if !c.Before.IsZero() && !t.Before(c.Before) {
		return false
	}
	return true
}
