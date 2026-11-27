package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	mb "github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// mailboxSession implements mb.Session over JMAP.
type mailboxSession struct {
	client             *JMAPClient
	account            protocol.Id
	inbox, junk        protocol.Id
	inboxName, junkNme string
}

// OpenMailbox discovers the JMAP session and finds the inbox and junk
// mailboxes by role.
func OpenMailbox(ctx context.Context, config *Config) (mb.Session, error) {
	c := NewJMAPClient(config)
	if _, err := c.Discover(ctx); err != nil {
		return nil, fmt.Errorf("jmap discovery %s: %w", config.Host, err)
	}
	account, ok := c.session.GetPrimaryMailAccountId()
	if !ok {
		return nil, errors.New("jmap: no primary mail account")
	}
	boxes, err := c.GetMailboxes(ctx)
	if err != nil {
		return nil, fmt.Errorf("jmap mailboxes: %w", err)
	}
	s := &mailboxSession{client: c, account: account}
	for _, b := range boxes {
		if b.Role == nil {
			continue
		}
		switch *b.Role {
		case "inbox":
			s.inbox, s.inboxName = b.Id, b.Name
		case "junk":
			s.junk, s.junkNme = b.Id, b.Name
		}
	}
	if s.inbox == "" || s.junk == "" {
		return nil, errors.New("jmap: the account has no mailbox with role inbox and junk")
	}
	return s, nil
}

func (s *mailboxSession) Folders() (string, string) { return s.inboxName, s.junkNme }

func (s *mailboxSession) Close() error { return nil }

// call performs a single method call and returns its response arguments.
func (s *mailboxSession) call(ctx context.Context, request protocol.Request) (*protocol.MethodResponse, error) {
	resp, err := s.client.makeAPIRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	if len(resp.MethodResponses) == 0 {
		return nil, errors.New("jmap: no method responses")
	}
	mr := &resp.MethodResponses[0]
	if protocol.IsErrorResponse(mr.Name) {
		return nil, fmt.Errorf("jmap error: %s", string(mr.Arguments))
	}
	return mr, nil
}

// find returns the newest email in inbox or junk with exactly this
// Message-ID, or nil.
func (s *mailboxSession) find(ctx context.Context, messageID string) (*protocol.Email, error) {
	if !mb.ValidMessageID(messageID) {
		return nil, fmt.Errorf("invalid Message-ID %q", messageID)
	}
	mr, err := s.call(ctx, *protocol.NewEmailQueryRequest(s.account, protocol.BuildEmailSearchFilter(messageID, ""), 10))
	if err != nil {
		return nil, fmt.Errorf("jmap Email/query: %w", err)
	}
	query, err := protocol.ParseEmailQueryResponse(mr)
	if err != nil {
		return nil, fmt.Errorf("jmap Email/query: %w", err)
	}
	if len(query.Ids) == 0 {
		return nil, nil
	}
	mr, err = s.call(ctx, *protocol.NewEmailGetRequest(s.account, query.Ids,
		[]string{"id", "blobId", "messageId", "mailboxIds", "keywords", "receivedAt"}))
	if err != nil {
		return nil, fmt.Errorf("jmap Email/get: %w", err)
	}
	got, err := protocol.ParseEmailGetResponse(mr)
	if err != nil {
		return nil, fmt.Errorf("jmap Email/get: %w", err)
	}
	for i := range got.List {
		e := &got.List[i]
		// The header filter matches substrings; require the exact ID.
		for _, id := range e.MessageId {
			if id == messageID && (e.MailboxIds[s.inbox] || e.MailboxIds[s.junk]) {
				return e, nil
			}
		}
	}
	return nil, nil
}

// Locate implements mb.Session. Downloading the blob does not change
// keywords, so the message stays unread.
func (s *mailboxSession) Locate(ctx context.Context, messageID string) (*mb.Message, error) {
	e, err := s.find(ctx, messageID)
	if err != nil || e == nil {
		return nil, err
	}
	raw, err := s.client.DownloadBlob(ctx, s.account, e.BlobId, "message.eml")
	if err != nil {
		return nil, err
	}
	m := &mb.Message{Placement: mb.Inbox, Folder: s.inboxName, Header: headerBlock(raw)}
	if !e.MailboxIds[s.inbox] {
		m.Placement, m.Folder = mb.Junk, s.junkNme
	}
	if t, err := time.Parse(time.RFC3339, e.ReceivedAt); err == nil {
		m.Received = t.UTC()
	}
	return m, nil
}

func headerBlock(raw []byte) []byte {
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		if i := bytes.Index(raw, []byte(sep)); i >= 0 {
			return raw[:i+len(sep)]
		}
	}
	return raw
}

// update patches one email with Email/set.
func (s *mailboxSession) update(ctx context.Context, id protocol.Id, patch map[string]interface{}) error {
	mr, err := s.call(ctx, protocol.Request{
		Using: []string{protocol.CoreCapability, protocol.MailCapability},
		MethodCalls: []protocol.MethodCall{{
			Name: protocol.MethodEmailSet,
			Arguments: map[string]interface{}{
				"accountId": s.account,
				"update":    map[protocol.Id]interface{}{id: patch},
			},
			CallId: "u",
		}},
	})
	if err != nil {
		return fmt.Errorf("jmap Email/set: %w", err)
	}
	var result struct {
		NotUpdated map[protocol.Id]json.RawMessage `json:"notUpdated"`
	}
	if err := json.Unmarshal(mr.Arguments, &result); err != nil {
		return fmt.Errorf("jmap Email/set: %w", err)
	}
	if reason, failed := result.NotUpdated[id]; failed {
		return fmt.Errorf("jmap Email/set did not update %s: %s", id, reason)
	}
	return nil
}

// MarkRead sets the $seen keyword.
func (s *mailboxSession) MarkRead(ctx context.Context, messageID string) error {
	e, err := s.find(ctx, messageID)
	if err != nil || e == nil {
		return err
	}
	return s.update(ctx, e.Id, map[string]interface{}{"keywords/$seen": true})
}

// Rescue moves the message from junk to the inbox and flags it as not junk.
func (s *mailboxSession) Rescue(ctx context.Context, messageID string) error {
	e, err := s.find(ctx, messageID)
	if err != nil || e == nil || e.MailboxIds[s.inbox] {
		return err
	}
	return s.update(ctx, e.Id, map[string]interface{}{
		"mailboxIds":        map[protocol.Id]bool{s.inbox: true},
		"keywords/$junk":    nil,
		"keywords/$notjunk": true,
	})
}
