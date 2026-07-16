package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// sendMail builds an EmailCreate payload and submits it via Email/set +
// EmailSubmission/set. The sender address is derived from config.Username
// (falling back to the first To address as a placeholder when no username is
// configured, which is unusual but prevents a crash).
func sendMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Sending mail via JMAP at %s...\n\n", config.Host)

	columns := []string{"Action", "Status", "Server", "To", "Subject", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	writeRow := func(status, errStr string) {
		_ = csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			strings.Join(config.To, "; "), config.Subject, errStr,
		})
	}

	client := NewJMAPClient(config)

	session, err := client.Discover(ctx)
	if err != nil {
		logger.LogError(slogLogger, "JMAP discovery failed", "error", err)
		writeRow("FAILURE", err.Error())
		return fmt.Errorf("JMAP discovery failed: %w", err)
	}

	// Determine the sender: prefer username, fall back to first To.
	mailFrom := config.Username
	if mailFrom == "" {
		mailFrom = config.To[0]
	}

	// Find Sent mailbox for outgoing mail storage.
	mailboxes, err := client.GetMailboxes(ctx)
	if err != nil {
		logger.LogError(slogLogger, "Failed to retrieve mailboxes", "error", err)
		writeRow("FAILURE", err.Error())
		return fmt.Errorf("failed to retrieve mailboxes: %w", err)
	}

	sentMb := findMailboxByRole(mailboxes, "sent")

	mailboxIds := map[protocol.Id]bool{}
	if sentMb != nil {
		mailboxIds[sentMb.Id] = true
	}

	toAddrs := make([]protocol.EmailAddress, 0, len(config.To))
	for _, addr := range config.To {
		toAddrs = append(toAddrs, protocol.EmailAddress{Email: addr})
	}
	ccAddrs := make([]protocol.EmailAddress, 0, len(config.Cc))
	for _, addr := range config.Cc {
		ccAddrs = append(ccAddrs, protocol.EmailAddress{Email: addr})
	}
	bccAddrs := make([]protocol.EmailAddress, 0, len(config.Bcc))
	for _, addr := range config.Bcc {
		bccAddrs = append(bccAddrs, protocol.EmailAddress{Email: addr})
	}

	bodyValues := map[string]protocol.EmailBodyValue{
		"textPart": {Value: config.Body, Charset: "utf-8"},
	}
	textBody := []protocol.EmailBodyPart{{PartId: "textPart", Type: "text/plain"}}
	var htmlBody []protocol.EmailBodyPart
	if config.BodyHTML != "" {
		bodyValues["htmlPart"] = protocol.EmailBodyValue{Value: config.BodyHTML, Charset: "utf-8"}
		htmlBody = []protocol.EmailBodyPart{{PartId: "htmlPart", Type: "text/html"}}
	}

	draft := protocol.EmailCreate{
		MailboxIds: mailboxIds,
		Keywords:   map[string]bool{"$draft": true},
		From:       []protocol.EmailAddress{{Email: mailFrom}},
		To:         toAddrs,
		Cc:         ccAddrs,
		Bcc:        bccAddrs,
		Subject:    config.Subject,
		BodyValues: bodyValues,
		TextBody:   textBody,
		HTMLBody:   htmlBody,
	}

	logger.LogDebug(slogLogger, "Sending mail",
		"from", mailFrom, "to", strings.Join(config.To, ", "),
		"subject", config.Subject, "session_api", session.APIURL)

	if err := client.SendEmail(ctx, draft, mailFrom, toAddrs); err != nil {
		logger.LogError(slogLogger, "Failed to send mail", "error", err)
		writeRow("FAILURE", err.Error())
		return fmt.Errorf("failed to send mail: %w", err)
	}

	fmt.Printf("✓ Email sent successfully\n")
	fmt.Printf("  From:    %s\n", mailFrom)
	fmt.Printf("  To:      %s\n", strings.Join(config.To, ", "))
	fmt.Printf("  Subject: %s\n", config.Subject)

	writeRow("SUCCESS", "")
	logger.LogInfo(slogLogger, "sendmail completed", "to", strings.Join(config.To, ", "), "subject", config.Subject)
	return nil
}
