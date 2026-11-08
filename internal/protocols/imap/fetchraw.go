package imap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// fetchSession implements fetch.Session over IMAP.
type fetchSession struct {
	client   *IMAPClient
	config   *Config
	log      *slog.Logger
	mailbox  string
	validity uint32
}

// OpenFetchSession connects and authenticates using config (host, TLS and
// credentials as for the other imap actions). The mailbox is selected on Fetch.
func OpenFetchSession(ctx context.Context, config *Config, log *slog.Logger) (fetch.Session, error) {
	c := NewIMAPClient(config)
	if err := c.Connect(ctx); err != nil {
		return nil, fmt.Errorf("imap connect %s:%d: %w", config.Host, config.Port, err)
	}
	if err := c.Auth(ctx, config.Username, config.Password, config.AccessToken); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("imap authentication failed for %s: %w", security.MaskUsername(config.Username), err)
	}
	log.Debug("imap session ready", "host", config.Host, "user", security.MaskUsername(config.Username))
	return &fetchSession{client: c, config: config, log: log}, nil
}

func (s *fetchSession) selectMailbox(ctx context.Context, mailbox string) error {
	if mailbox == "" {
		mailbox = s.config.Mailbox
	}
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if s.mailbox == mailbox && s.client.selected != nil {
		return nil
	}
	if err := s.client.SelectMailbox(ctx, mailbox); err != nil {
		return err
	}
	s.mailbox = mailbox
	s.validity = s.client.selected.UIDValidity
	return nil
}

// Fetch searches by date (whole days, IMAP SINCE/BEFORE) and optional subject,
// then downloads BODY[] for every UID not skipped.
func (s *fetchSession) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	if err := s.selectMailbox(ctx, c.Folder); err != nil {
		return err
	}
	criteria := &imap.SearchCriteria{Since: c.Since, Before: c.Before}
	if c.Subject != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "Subject", Value: c.Subject})
	}
	data, err := s.client.client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return fmt.Errorf("SEARCH failed: %w", err)
	}
	var uids []imap.UID
	if set, ok := data.All.(imap.UIDSet); ok {
		uids, _ = set.Nums()
	}
	s.log.Info("imap search complete", "mailbox", s.mailbox, "matches", len(uids))

	delivered := 0
	for _, uid := range uids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.Max > 0 && delivered >= c.Max {
			break
		}
		key := fmt.Sprintf("%d:%d", s.validity, uid)
		if c.SkipKey != nil && c.SkipKey(key) {
			continue
		}
		raw, err := s.client.FetchRFC822(ctx, uid)
		if err != nil {
			return err
		}
		if err := sink(ctx, fetch.RawMessage{Key: key, Raw: raw}); err != nil {
			if errors.Is(err, fetch.ErrStop) {
				return nil
			}
			return err
		}
		delivered++
	}
	return nil
}

// Delete expunges a message by "<uidvalidity>:<uid>" key in the selected mailbox.
func (s *fetchSession) Delete(ctx context.Context, key string) error {
	v, u, ok := strings.Cut(key, ":")
	validity, err1 := strconv.ParseUint(v, 10, 32)
	uid, err2 := strconv.ParseUint(u, 10, 32)
	if !ok || err1 != nil || err2 != nil {
		return fmt.Errorf("invalid IMAP message key %q", key)
	}
	if s.client.selected == nil {
		return errors.New("no mailbox selected")
	}
	if uint32(validity) != s.validity {
		return fmt.Errorf("UIDVALIDITY changed (%d != %d); refusing to delete", validity, s.validity)
	}
	return s.client.DeleteMessage(ctx, imap.UID(uid))
}

func (s *fetchSession) Close() error { return s.client.Logout() }
