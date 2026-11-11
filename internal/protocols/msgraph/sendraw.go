package msgraph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"

	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// rawSender implements send.Sender over Microsoft Graph using the MIME form
// of sendMail, so the caller's own headers (Message-ID, X-*) are submitted
// as written.
type rawSender struct {
	client  *msgraphsdk.GraphServiceClient
	config  *Config
	mailbox string
	log     *slog.Logger
}

// OpenSender authenticates with the configured credential and returns a quiet
// sender for config.Mailbox. Sending needs the Mail.Send application permission.
func OpenSender(ctx context.Context, config *Config, log *slog.Logger) (send.Sender, error) {
	if config == nil || config.Mailbox == "" {
		return nil, errors.New("msgraph: mailbox is required")
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	client, err := NewGraphServiceClient(ctx, config, log)
	if err != nil {
		return nil, err
	}
	return &rawSender{client: client, config: config, mailbox: config.Mailbox, log: log}, nil
}

// Send posts m.Raw as base64 MIME. The envelope fields of m are ignored:
// Graph sends as the mailbox and takes recipients from the headers.
func (s *rawSender) Send(ctx context.Context, m send.Message) (send.Result, error) {
	res := send.Result{RemoteHost: "graph.microsoft.com", TLSVersion: "TLS"}
	if len(m.Raw) == 0 {
		return res, errors.New("msgraph: empty message")
	}
	err := retryWithBackoff(ctx, s.config.MaxRetries, s.config.RetryDelay, func() error {
		req, err := buildMIMERequestFromBytes(s.client.GetAdapter(), s.mailbox, m.Raw)
		if err != nil {
			return err
		}
		return s.client.GetAdapter().SendNoContent(ctx, req, nil)
	})
	if err != nil {
		return res, fmt.Errorf("graph sendMail as %s: %w", s.mailbox, enrichGraphAPIError(err, nil, "sendRaw"))
	}
	res.ServerResponse = "202 Accepted"
	s.log.Debug("graph message accepted", "mailbox", s.mailbox)
	return res, nil
}

func (s *rawSender) Close() error { return nil }
