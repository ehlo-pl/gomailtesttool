package pop3

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/mail"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// fetchSession implements fetch.Session over POP3. Deletions (DELE) are only
// committed by the server when the session is closed with QUIT.
type fetchSession struct {
	client *POP3Client
	log    *slog.Logger
	byKey  map[string]int // UIDL -> message number, filled by Fetch
}

// OpenFetchSession connects (with STLS when configured) and authenticates.
func OpenFetchSession(ctx context.Context, config *Config, log *slog.Logger) (fetch.Session, error) {
	c := NewPOP3Client(config)
	if err := c.Connect(ctx); err != nil {
		return nil, fmt.Errorf("pop3 connect %s:%d: %w", config.Host, config.Port, err)
	}
	if config.StartTLS && c.GetTLSState() == nil {
		if err := c.StartTLS(nil); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("STLS failed: %w", err)
		}
	}
	_, _ = c.Capabilities(ctx)
	if err := c.Auth(ctx, config.Username, config.Password, config.AccessToken); err != nil {
		_ = c.Quit()
		return nil, fmt.Errorf("pop3 authentication failed for %s: %w", security.MaskUsername(config.Username), err)
	}
	log.Debug("pop3 session ready", "host", config.Host, "user", security.MaskUsername(config.Username))
	return &fetchSession{client: c, log: log, byKey: map[string]int{}}, nil
}

var wordDecoder = &mime.WordDecoder{}

// Fetch lists messages by UIDL, skips known keys before any download, reads
// headers with TOP 0 to apply date/subject filters and downloads with RETR.
func (s *fetchSession) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	msgs, err := s.client.UIDL(ctx)
	if err != nil {
		return fmt.Errorf("UIDL failed (server must support UIDL for incremental collection): %w", err)
	}
	s.log.Info("pop3 mailbox listed", "messages", len(msgs))
	delivered := 0
	for _, m := range msgs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.Max > 0 && delivered >= c.Max {
			break
		}
		if m.UIDL == "" {
			continue
		}
		s.byKey[m.UIDL] = m.Number
		if c.SkipKey != nil && c.SkipKey(m.UIDL) {
			continue
		}
		header, err := s.client.Top(ctx, m.Number, 0)
		if err != nil {
			s.log.Warn("TOP failed", "msgnum", m.Number, "error", err)
			continue
		}
		parsed, err := mail.ReadMessage(strings.NewReader(string(header) + "\r\n"))
		if err != nil {
			s.log.Warn("unparsable headers", "msgnum", m.Number, "error", err)
			continue
		}
		subject := parsed.Header.Get("Subject")
		if d, err := wordDecoder.DecodeHeader(subject); err == nil {
			subject = d
		}
		date, _ := parsed.Header.Date()
		if !c.SubjectMatches(subject) || !c.InRange(date) {
			continue
		}
		raw, err := s.client.Retr(ctx, m.Number)
		if err != nil {
			return fmt.Errorf("RETR %d: %w", m.Number, err)
		}
		if err := sink(ctx, fetch.RawMessage{Key: m.UIDL, Received: date, Raw: raw}); err != nil {
			if errors.Is(err, fetch.ErrStop) {
				return nil
			}
			return err
		}
		delivered++
	}
	return nil
}

// Delete marks the message with the given UIDL for deletion (committed on Close).
func (s *fetchSession) Delete(ctx context.Context, key string) error {
	n, ok := s.byKey[key]
	if !ok {
		return fmt.Errorf("unknown POP3 UIDL %q in this session", key)
	}
	return s.client.Delete(ctx, n)
}

// Close sends QUIT, which commits pending deletions.
func (s *fetchSession) Close() error { return s.client.Quit() }
