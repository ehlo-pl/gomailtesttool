// Package mailbox is a small, protocol-neutral API for finding one message in
// a mailbox's inbox or junk folder and acting on it. It complements package
// fetch (bulk download); protocol packages (imap, jmap) implement Session.
//
// Implementations must be quiet (no fmt.Print to stdout).
package mailbox

import (
	"context"
	"strings"
	"time"
)

// Placement says where a message was found.
type Placement string

// Placements.
const (
	Inbox Placement = "inbox"
	Junk  Placement = "junk"
)

// Message is a located message.
type Message struct {
	Placement Placement
	Folder    string    // server folder name
	Header    []byte    // the full header block
	Received  time.Time // server's delivery time; zero if unknown
}

// Session is an authenticated connection to one mailbox. Messages are
// identified by their Message-ID without angle brackets.
type Session interface {
	// Folders returns the inbox and junk folder names used.
	Folders() (inbox, junk string)
	// Locate searches inbox and junk; it returns nil, nil when the message is
	// in neither. It must not change the message's read state.
	Locate(ctx context.Context, messageID string) (*Message, error)
	// MarkRead marks the located message as read.
	MarkRead(ctx context.Context, messageID string) error
	// Rescue moves the message from junk to the inbox. It does nothing when
	// the message is not in junk.
	Rescue(ctx context.Context, messageID string) error
	Close() error
}

// ValidMessageID reports whether id is safe to put in a search command.
func ValidMessageID(id string) bool {
	return id != "" && !strings.ContainsAny(id, "\r\n\"<> \x00") && strings.Count(id, "@") == 1
}
