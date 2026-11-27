package imap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/emersion/go-imap/v2"

	"github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

const inboxName = "INBOX"

// mailboxSession implements mailbox.Session over IMAP.
type mailboxSession struct {
	client *IMAPClient
	junk   string
}

// OpenMailbox connects and authenticates using config and determines the junk
// folder: junkOverride when set, otherwise the folder carrying the \Junk
// special-use attribute (RFC 6154).
func OpenMailbox(ctx context.Context, config *Config, junkOverride string) (mailbox.Session, error) {
	c := NewIMAPClient(config)
	if err := c.Connect(ctx); err != nil {
		return nil, fmt.Errorf("imap connect %s:%d: %w", config.Host, config.Port, err)
	}
	if err := c.Auth(ctx, config.Username, config.Password, config.AccessToken); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("imap authentication failed for %s: %w", security.MaskUsername(config.Username), err)
	}
	junk, err := c.findJunk(junkOverride)
	if err != nil {
		_ = c.Logout()
		return nil, err
	}
	return &mailboxSession{client: c, junk: junk}, nil
}

// findJunk lists the folders and picks the junk one.
func (c *IMAPClient) findJunk(override string) (string, error) {
	boxes, err := c.client.List("", "*", nil).Collect()
	if err != nil {
		return "", fmt.Errorf("LIST failed: %w", err)
	}
	var names []string
	special := ""
	for _, b := range boxes {
		names = append(names, b.Mailbox)
		if slices.Contains(b.Attrs, imap.MailboxAttrJunk) && special == "" {
			special = b.Mailbox
		}
	}
	switch {
	case override != "":
		if !slices.Contains(names, override) {
			return "", fmt.Errorf("junk folder %q does not exist; folders: %s", override, strings.Join(names, ", "))
		}
		return override, nil
	case special != "":
		return special, nil
	}
	return "", fmt.Errorf("no folder has the \\Junk attribute; set 'junk:' on the endpoint to one of: %s", strings.Join(names, ", "))
}

func (s *mailboxSession) Folders() (string, string) { return inboxName, s.junk }

// find selects inbox, then junk, and returns the UID of the message in the
// first folder that has it. The folder stays selected.
func (s *mailboxSession) find(ctx context.Context, messageID string) (imap.UID, string, error) {
	if !mailbox.ValidMessageID(messageID) {
		return 0, "", fmt.Errorf("invalid Message-ID %q", messageID)
	}
	for _, folder := range []string{inboxName, s.junk} {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		// Selecting again refreshes the view of newly delivered mail.
		if err := s.client.SelectMailbox(ctx, folder); err != nil {
			return 0, "", err
		}
		uids, err := s.client.SearchMessages(ctx, messageID, "")
		if err != nil {
			return 0, "", err
		}
		if len(uids) > 0 {
			return uids[len(uids)-1], folder, nil
		}
	}
	return 0, "", nil
}

// Locate implements mailbox.Session. Headers are fetched with BODY.PEEK so
// the message stays unread.
func (s *mailboxSession) Locate(ctx context.Context, messageID string) (*mailbox.Message, error) {
	uid, folder, err := s.find(ctx, messageID)
	if err != nil || uid == 0 {
		return nil, err
	}
	section := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, Peek: true}
	msgs, err := s.client.client.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		InternalDate: true,
		BodySection:  []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("FETCH headers of UID %d: %w", uid, err)
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	m := &mailbox.Message{Placement: mailbox.Inbox, Folder: folder, Received: msgs[0].InternalDate,
		Header: msgs[0].FindBodySection(section)}
	if folder != inboxName {
		m.Placement = mailbox.Junk
	}
	if m.Header == nil {
		return nil, errors.New("server returned no header section")
	}
	return m, nil
}

// MarkRead sets \Seen.
func (s *mailboxSession) MarkRead(ctx context.Context, messageID string) error {
	uid, _, err := s.find(ctx, messageID)
	if err != nil || uid == 0 {
		return err
	}
	_, err = s.client.client.Store(imap.UIDSetNum(uid), &imap.StoreFlags{
		Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen},
	}, nil).Collect()
	if err != nil {
		return fmt.Errorf("marking UID %d read: %w", uid, err)
	}
	return nil
}

// Rescue moves the message from junk to INBOX.
func (s *mailboxSession) Rescue(ctx context.Context, messageID string) error {
	uid, folder, err := s.find(ctx, messageID)
	if err != nil || uid == 0 || folder == inboxName {
		return err
	}
	if _, err := s.client.client.Move(imap.UIDSetNum(uid), inboxName).Wait(); err != nil {
		return fmt.Errorf("moving UID %d to %s: %w", uid, inboxName, err)
	}
	return nil
}

func (s *mailboxSession) Close() error { return s.client.Logout() }
