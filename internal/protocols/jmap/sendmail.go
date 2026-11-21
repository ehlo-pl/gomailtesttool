package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	mimebuilder "github.com/ehlo-pl/gomailtesttool/internal/common/mime"
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

	// RFC 8621 requires every Email to belong to at least one Mailbox, so the
	// message is always created in a real mailbox (servers such as Stalwart
	// reject an empty mailboxIds). See selectSendMailboxes.
	mailboxes, err := client.GetMailboxes(ctx)
	if err != nil {
		logger.LogError(slogLogger, "Failed to retrieve mailboxes", "error", err)
		writeRow("FAILURE", err.Error())
		return fmt.Errorf("failed to retrieve mailboxes: %w", err)
	}
	mailboxIds, sentMailboxId, err := selectSendMailboxes(mailboxes, config.SaveToSent)
	if err != nil {
		writeRow("FAILURE", err.Error())
		return err
	}
	if config.SaveToSent && sentMailboxId == "" {
		logger.LogWarn(slogLogger, "No mailbox with role 'sent' found; the sent message will not be saved")
	}

	// RFC 8621 requires every EmailSubmission to name the sending Identity.
	identities, err := client.GetIdentities(ctx)
	if err != nil {
		logger.LogError(slogLogger, "Failed to retrieve identities", "error", err)
		writeRow("FAILURE", err.Error())
		return fmt.Errorf("failed to retrieve identities: %w", err)
	}
	identity, err := selectIdentity(identities, mailFrom)
	if err != nil {
		writeRow("FAILURE", err.Error())
		return err
	}
	logger.LogDebug(slogLogger, "Using JMAP identity", "identity_id", identity.Id, "identity_email", identity.Email)

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

	// The SMTP envelope must list every recipient, including Cc and Bcc.
	envelopeRcpts := make([]protocol.EmailAddress, 0, len(toAddrs)+len(ccAddrs)+len(bccAddrs))
	envelopeRcpts = append(envelopeRcpts, toAddrs...)
	envelopeRcpts = append(envelopeRcpts, ccAddrs...)
	envelopeRcpts = append(envelopeRcpts, bccAddrs...)

	bodyValues := map[string]protocol.EmailBodyValue{
		"textPart": {Value: config.Body, Charset: "utf-8"},
	}
	textBody := []protocol.EmailBodyPart{{PartId: "textPart", Type: "text/plain"}}
	var htmlBody []protocol.EmailBodyPart
	if config.BodyHTML != "" {
		bodyValues["htmlPart"] = protocol.EmailBodyValue{Value: config.BodyHTML, Charset: "utf-8"}
		htmlBody = []protocol.EmailBodyPart{{PartId: "htmlPart", Type: "text/html"}}
	}

	// Load and upload attachments.
	onSkip := func(path string, err error) {
		logger.LogWarn(slogLogger, "Skipping attachment", "path", path, "error", err)
	}
	regularAtts, err := email.LoadAttachments(config.AttachmentFiles, onSkip)
	if err != nil {
		writeRow("FAILURE", err.Error())
		return err
	}
	inlineAtts, err := email.LoadInlineAttachments(config.InlineAttachmentFiles, onSkip)
	if err != nil {
		writeRow("FAILURE", err.Error())
		return err
	}

	if config.MessageID != "" || config.MessageIDSuffix != "" {
		messageID, resolveErr := mimebuilder.ResolveMessageID(config.MessageID, config.MessageIDSuffix, mimebuilder.GenerateMessageID(config.Host, "jmaptool"))
		if resolveErr != nil {
			writeRow("FAILURE", resolveErr.Error())
			return resolveErr
		}
		rawMessage, buildErr := mimebuilder.Build(mimebuilder.Message{
			From:        mailFrom,
			To:          config.To,
			Cc:          config.Cc,
			Subject:     config.Subject,
			TextBody:    config.Body,
			HTMLBody:    config.BodyHTML,
			Attachments: regularAtts,
			Inline:      inlineAtts,
			MessageID:   messageID,
		})
		if buildErr != nil {
			writeRow("FAILURE", buildErr.Error())
			return fmt.Errorf("failed to build RFC 5322 message: %w", buildErr)
		}
		if err := client.SendRawEmail(ctx, rawMessage, mailboxIds, identity.Id, mailFrom, envelopeRcpts, sentMailboxId); err != nil {
			logger.LogError(slogLogger, "Failed to send mail", "error", err)
			writeRow("FAILURE", err.Error())
			return fmt.Errorf("failed to send mail: %w", err)
		}
		fmt.Printf("✓ Email sent successfully\n")
		fmt.Printf("  From:    %s\n", mailFrom)
		fmt.Printf("  To:      %s\n", strings.Join(config.To, ", "))
		fmt.Printf("  Subject: %s\n", config.Subject)
		writeRow("SUCCESS", "")
		logger.LogInfo(slogLogger, "sendmail completed", "to", strings.Join(config.To, ", "), "subject", config.Subject, "message_id", messageID)
		return nil
	}

	var jmapAttachments []protocol.EmailAttachment
	for _, att := range append(regularAtts, inlineAtts...) {
		blobId, uploadErr := client.UploadBlob(ctx, att.ContentType, att.Data)
		if uploadErr != nil {
			writeRow("FAILURE", uploadErr.Error())
			return fmt.Errorf("failed to upload attachment %q: %w", att.Name, uploadErr)
		}
		ja := protocol.EmailAttachment{
			BlobId: blobId,
			Type:   att.ContentType,
			Name:   att.Name,
		}
		if att.Inline {
			ja.Disposition = "inline"
			ja.Cid = att.ContentID
		} else {
			ja.Disposition = "attachment"
		}
		jmapAttachments = append(jmapAttachments, ja)
	}

	draft := protocol.EmailCreate{
		MailboxIds:  mailboxIds,
		Keywords:    map[string]bool{"$draft": true},
		From:        []protocol.EmailAddress{{Email: mailFrom}},
		To:          toAddrs,
		Cc:          ccAddrs,
		Bcc:         bccAddrs,
		Subject:     config.Subject,
		BodyValues:  bodyValues,
		TextBody:    textBody,
		HTMLBody:    htmlBody,
		Attachments: jmapAttachments,
	}

	logger.LogDebug(slogLogger, "Sending mail",
		"from", mailFrom, "to", strings.Join(config.To, ", "),
		"subject", config.Subject, "session_api", session.APIURL)

	if err := client.SendEmail(ctx, draft, identity.Id, mailFrom, envelopeRcpts, sentMailboxId); err != nil {
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

// selectSendMailboxes chooses where the outgoing message is created and where
// it ends up after submission. The message is created in Drafts (falling back
// to Sent) because RFC 8621 requires a non-empty mailboxIds. sentMailboxId is
// returned only when saveToSent is set and a Sent mailbox exists; when empty,
// the draft is destroyed after a successful submission.
func selectSendMailboxes(mailboxes []protocol.Mailbox, saveToSent bool) (map[protocol.Id]bool, protocol.Id, error) {
	var sentMailboxId protocol.Id
	sentMb := findMailboxByRole(mailboxes, "sent")
	if saveToSent && sentMb != nil {
		sentMailboxId = sentMb.Id
	}

	createIn := findMailboxByRole(mailboxes, "drafts")
	if createIn == nil {
		createIn = sentMb
	}
	if createIn == nil {
		return nil, "", fmt.Errorf("no mailbox with role 'drafts' or 'sent' found; JMAP requires the message to be created in a mailbox before submission")
	}
	return map[protocol.Id]bool{createIn.Id: true}, sentMailboxId, nil
}

// selectIdentity picks the Identity used to submit mail from mailFrom: an exact
// (case-insensitive) email match first, then a wildcard "*@domain" identity.
// When nothing matches and the account has a single identity, that one is
// used and the server decides whether the From address is allowed.
func selectIdentity(identities []protocol.Identity, mailFrom string) (*protocol.Identity, error) {
	if len(identities) == 0 {
		return nil, fmt.Errorf("no JMAP sending identities found for this account; create one on the server")
	}
	for i := range identities {
		if strings.EqualFold(identities[i].Email, mailFrom) {
			return &identities[i], nil
		}
	}
	if at := strings.LastIndex(mailFrom, "@"); at >= 0 {
		wildcard := "*" + mailFrom[at:]
		for i := range identities {
			if strings.EqualFold(identities[i].Email, wildcard) {
				return &identities[i], nil
			}
		}
	}
	if len(identities) == 1 {
		return &identities[0], nil
	}
	emails := make([]string, 0, len(identities))
	for _, id := range identities {
		emails = append(emails, id.Email)
	}
	return nil, fmt.Errorf("no JMAP identity matches sender %q; available identities: %s", mailFrom, strings.Join(emails, ", "))
}
