package gmail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

// fetchSession implements fetch.Session over the Gmail API.
type fetchSession struct {
	svc *gmailapi.Service
	log *slog.Logger
}

// OpenFetchSession authenticates with the configured credential (bearer
// token, interactive OAuth or service account with domain-wide delegation).
// Least privilege: gmail.readonly, or gmail.modify when withDelete is true
// (Delete moves messages to Trash).
func OpenFetchSession(ctx context.Context, config *Config, log *slog.Logger, withDelete bool) (fetch.Session, error) {
	cfg := *config
	cfg.Action = ActionExportMessages
	if withDelete {
		cfg.Action = ActionDeleteMessages
	}
	svc, err := newGmailService(ctx, &cfg, log)
	if err != nil {
		return nil, err
	}
	return &fetchSession{svc: svc, log: log}, nil
}

// buildFetchQuery assembles a Gmail search query from the criteria. Label and
// subject are quoted; embedded double quotes are rejected to keep the query
// well-formed. Dates use epoch seconds, which Gmail accepts for after:/before:.
func buildFetchQuery(c fetch.Criteria) (string, error) {
	var parts []string
	if c.Folder != "" {
		if strings.ContainsAny(c.Folder, "\"\r\n") {
			return "", fmt.Errorf("invalid Gmail label %q", c.Folder)
		}
		parts = append(parts, fmt.Sprintf("label:\"%s\"", c.Folder))
	}
	if c.Subject != "" {
		if strings.ContainsAny(c.Subject, "\"\r\n") {
			return "", fmt.Errorf("invalid subject filter %q", c.Subject)
		}
		parts = append(parts, fmt.Sprintf("subject:\"%s\"", c.Subject))
	}
	if !c.Since.IsZero() {
		parts = append(parts, fmt.Sprintf("after:%d", c.Since.Unix()))
	}
	if !c.Before.IsZero() {
		parts = append(parts, fmt.Sprintf("before:%d", c.Before.Unix()))
	}
	if q := strings.TrimSpace(c.Query); q != "" {
		parts = append(parts, q)
	}
	return strings.Join(parts, " "), nil
}

// Fetch pages through matching messages (PageToken) and downloads format=raw.
func (s *fetchSession) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	q, err := buildFetchQuery(c)
	if err != nil {
		return err
	}
	s.log.Debug("gmail fetch", "query", q)
	delivered := 0
	pageToken := ""
	for {
		call := s.svc.Users.Messages.List("me").Q(q).MaxResults(100).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		res, err := call.Do()
		if err != nil {
			return fmt.Errorf("gmail list: %w", err)
		}
		for _, m := range res.Messages {
			if err := ctx.Err(); err != nil {
				return err
			}
			if c.Max > 0 && delivered >= c.Max {
				return nil
			}
			if c.SkipKey != nil && c.SkipKey(m.Id) {
				continue
			}
			full, err := s.svc.Users.Messages.Get("me", m.Id).Format("raw").Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("gmail get %s: %w", m.Id, err)
			}
			raw, err := decodeGmailRaw(full.Raw)
			if err != nil {
				return fmt.Errorf("gmail decode %s: %w", m.Id, err)
			}
			var received time.Time
			if full.InternalDate > 0 {
				received = time.UnixMilli(full.InternalDate).UTC()
			}
			if err := sink(ctx, fetch.RawMessage{Key: m.Id, Received: received, Raw: raw}); err != nil {
				if errors.Is(err, fetch.ErrStop) {
					return nil
				}
				return err
			}
			delivered++
		}
		if res.NextPageToken == "" {
			return nil
		}
		pageToken = res.NextPageToken
	}
}

// Delete moves the message to Trash (requires gmail.modify).
func (s *fetchSession) Delete(ctx context.Context, key string) error {
	_, err := s.svc.Users.Messages.Trash("me", key).Context(ctx).Do()
	return err
}

func (s *fetchSession) Close() error { return nil }
