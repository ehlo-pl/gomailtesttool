package msgraph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

// fetchPageSize is the $top used when paging through a folder.
const fetchPageSize = 100

// fetchSession implements fetch.Session over Microsoft Graph.
type fetchSession struct {
	client  *msgraphsdk.GraphServiceClient
	config  *Config
	mailbox string
	log     *slog.Logger
}

// OpenFetchSession authenticates with the configured credential (bearer token,
// client secret, PFX or certificate thumbprint). Reading needs the Mail.Read
// application permission; Delete needs Mail.ReadWrite.
func OpenFetchSession(ctx context.Context, config *Config, log *slog.Logger) (fetch.Session, error) {
	if config.Mailbox == "" {
		return nil, errors.New("msgraph: --mailbox is required")
	}
	client, err := NewGraphServiceClient(ctx, config, log)
	if err != nil {
		return nil, err
	}
	return &fetchSession{client: client, config: config, mailbox: config.Mailbox, log: log}, nil
}

// buildDateFilter returns an OData filter on receivedDateTime. Values are
// formatted from time.Time by this code (never from user strings), so the
// filter cannot be injected. receivedDateTime is the first (and only)
// filter property so it can be combined with $orderby=receivedDateTime
// without Graph's InefficientFilter error.
func buildDateFilter(since, before time.Time) string {
	var clauses []string
	if !since.IsZero() {
		clauses = append(clauses, "receivedDateTime ge "+since.UTC().Format(time.RFC3339))
	}
	if !before.IsZero() {
		clauses = append(clauses, "receivedDateTime lt "+before.UTC().Format(time.RFC3339))
	}
	return strings.Join(clauses, " and ")
}

// Fetch pages through the folder (following @odata.nextLink), oldest first,
// applies the subject filter client-side and downloads MIME via $value.
func (s *fetchSession) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	folder := c.Folder
	if folder == "" {
		folder = "inbox"
	}
	filter := buildDateFilter(c.Since, c.Before)
	q := &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
		Top:     Int32Ptr(fetchPageSize),
		Orderby: []string{"receivedDateTime asc"},
		Select:  []string{"id", "internetMessageId", "subject", "receivedDateTime"},
	}
	if filter != "" {
		q.Filter = &filter
	}
	builder := s.client.Users().ByUserId(s.mailbox).MailFolders().ByMailFolderId(folder).Messages()
	s.log.Debug("graph fetch", "mailbox", s.mailbox, "folder", folder, "filter", filter)

	delivered, pages := 0, 0
	var next *string
	for {
		var page models.MessageCollectionResponseable
		err := retryWithBackoff(ctx, s.config.MaxRetries, s.config.RetryDelay, func() error {
			var apiErr error
			if next == nil {
				page, apiErr = builder.Get(ctx, &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{QueryParameters: q})
			} else {
				page, apiErr = builder.WithUrl(*next).Get(ctx, nil)
			}
			return apiErr
		})
		if err != nil {
			return fmt.Errorf("list messages in %s/%s: %w", s.mailbox, folder, enrichGraphAPIError(err, nil, "fetchRaw"))
		}
		pages++
		for _, m := range page.GetValue() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if c.Max > 0 && delivered >= c.Max {
				return nil
			}
			if m.GetId() == nil {
				continue
			}
			id := *m.GetId()
			if c.SkipKey != nil && c.SkipKey(id) {
				continue
			}
			subject := ""
			if m.GetSubject() != nil {
				subject = *m.GetSubject()
			}
			if !c.SubjectMatches(subject) {
				continue
			}
			var received time.Time
			if m.GetReceivedDateTime() != nil {
				received = m.GetReceivedDateTime().UTC()
			}
			var raw []byte
			err := retryWithBackoff(ctx, s.config.MaxRetries, s.config.RetryDelay, func() error {
				content, apiErr := s.client.Users().ByUserId(s.mailbox).Messages().ByMessageId(id).Content().Get(ctx, nil)
				raw = content
				return apiErr
			})
			if err != nil {
				return fmt.Errorf("download message %s: %w", id, enrichGraphAPIError(err, nil, "fetchRaw"))
			}
			if err := sink(ctx, fetch.RawMessage{Key: id, Received: received, Raw: raw}); err != nil {
				if errors.Is(err, fetch.ErrStop) {
					return nil
				}
				return err
			}
			delivered++
		}
		next = page.GetOdataNextLink()
		if next == nil || *next == "" {
			s.log.Debug("graph fetch complete", "pages", pages, "delivered", delivered)
			return nil
		}
	}
}

// Delete moves the message to Deleted Items (soft delete).
func (s *fetchSession) Delete(ctx context.Context, key string) error {
	return retryWithBackoff(ctx, s.config.MaxRetries, s.config.RetryDelay, func() error {
		return deleteGraphMessage(ctx, s.client, s.mailbox, key, false)
	})
}

func (s *fetchSession) Close() error { return nil }
