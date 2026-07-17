package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	tmpl "github.com/ehlo-pl/gomailtesttool/internal/common/template"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// resolveTemplate renders --template (if set) and applies it to the config.
// HTML mode (non-.eml extension) fills BodyHTML with the rendered content;
// EML mode parses the rendered message and maps its recognised fields onto
// the config consumed by Email/set: recipients fall back to the EML headers
// where flags were not provided, the From header becomes the sender (before
// the username fallback), subject and bodies come from the EML. Unmapped
// headers are logged as skipped.
func resolveTemplate(config *Config, slogLogger *slog.Logger) error {
	if config.Template == "" {
		return nil
	}

	vars, err := tmpl.ParseVars(config.TemplateVars)
	if err != nil {
		return fmt.Errorf("invalid --template-vars: %w", err)
	}
	rendered, err := tmpl.Render(config.Template, vars)
	if err != nil {
		return err
	}

	if !tmpl.IsEML(config.Template) {
		config.BodyHTML = rendered
		return nil
	}

	parsed, err := tmpl.ParseEML(rendered)
	if err != nil {
		return fmt.Errorf("template %s: %w", config.Template, err)
	}
	if parsed.TextBody == "" && parsed.HTMLBody == "" {
		return fmt.Errorf("template %s has no text or HTML body", config.Template)
	}
	if len(config.To) == 0 {
		config.To = parsed.To
	}
	if len(config.Cc) == 0 {
		config.Cc = parsed.Cc
	}
	if len(config.Bcc) == 0 {
		config.Bcc = parsed.Bcc
	}
	if len(config.To)+len(config.Cc)+len(config.Bcc) == 0 {
		return fmt.Errorf("template %s has no recipients; provide --to", config.Template)
	}
	config.From = parsed.From
	if parsed.Subject != "" {
		config.Subject = parsed.Subject
	}
	config.Body = parsed.TextBody
	config.BodyHTML = parsed.HTMLBody

	if len(parsed.UnmappedHeaders) > 0 {
		logger.LogWarn(slogLogger, "Template headers not mapped to JMAP Email/set", "headers", strings.Join(parsed.UnmappedHeaders, ", "))
	}
	return nil
}

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

	// Determine the sender: an .eml --template's From header wins, then the
	// username, then the first To address as a placeholder.
	mailFrom := config.From
	if mailFrom == "" {
		mailFrom = config.Username
	}
	if mailFrom == "" && len(config.To) > 0 {
		mailFrom = config.To[0]
	}
	if mailFrom == "" {
		writeRow("FAILURE", "no sender address available")
		return fmt.Errorf("no sender address available: provide --username or a From header in the template")
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
