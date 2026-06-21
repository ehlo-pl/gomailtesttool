package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
	smtptls "github.com/ehlo-pl/gomailtesttool/internal/smtp/tls"
)

// SendMail performs end-to-end email sending test.
func SendMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.SMTPS {
		if config.ConnectAddress != "" {
			fmt.Printf("Sending test email via %s:%d (SMTPS) (connecting via %s)...\n\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			fmt.Printf("Sending test email via %s:%d (SMTPS)...\n\n", config.Host, config.Port)
		}
	} else {
		if config.ConnectAddress != "" {
			fmt.Printf("Sending test email via %s:%d (connecting via %s)...\n\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			fmt.Printf("Sending test email via %s:%d...\n\n", config.Host, config.Port)
		}
	}

	// Write CSV header
	if err := writeSMTPCSVHeader(csvLogger, []string{
		"Action", "Status", "Server", "Port", "Connect_Address", "From", "To", "Cc", "Bcc",
		"Subject", "Body_Type", "Attachment_Count", "SMTP_Response_Code", "Message_ID",
		"TLS_Version", "Cipher_Suite", "Cipher_Strength",
		"Cert_Subject", "Cert_Issuer", "Cert_SANs",
		"Cert_Valid_From", "Cert_Valid_To", "Cert_Verification_Status",
		"Error",
	}); err != nil {
		logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
	}

	bodyType := "Text"
	if config.BodyHTML != "" {
		bodyType = "HTML"
	}
	attachmentCount := len(config.Attachments) + len(config.InlineAttachments)
	attachmentCountStr := fmt.Sprintf("%d", attachmentCount)

	fmt.Printf("From:    %s\n", config.From)
	fmt.Printf("To:      %s\n", strings.Join(config.To, ", "))
	if len(config.Cc) > 0 {
		fmt.Printf("Cc:      %s\n", strings.Join(config.Cc, ", "))
	}
	if len(config.Bcc) > 0 {
		fmt.Printf("Bcc:     %s\n", strings.Join(config.Bcc, ", "))
	}
	fmt.Printf("Subject: %s\n", config.Subject)
	fmt.Printf("Body Type: %s\n", bodyType)
	if attachmentCount > 0 {
		fmt.Printf("Attachments: %d file(s)\n", attachmentCount)
	}
	fmt.Println()

	// Create and connect client
	client := NewSMTPClient(config.Host, config.Port, config)
	logger.LogDebug(slogLogger, "Connecting to SMTP server")

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err)
		if logErr := writeSMTPCSVRow(csvLogger, []string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
			"", "", "", "", "", "", "", "", "", // No TLS info on connection failure
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}
	defer client.Close()

	if config.SMTPS {
		if config.ConnectAddress != "" {
			fmt.Printf("✓ Connected with SMTPS (implicit TLS) via %s\n", config.ConnectAddress)
		} else {
			fmt.Printf("✓ Connected with SMTPS (implicit TLS)\n")
		}
	} else {
		if config.ConnectAddress != "" {
			fmt.Printf("✓ Connected via %s\n", config.ConnectAddress)
		} else {
			fmt.Printf("✓ Connected\n")
		}
	}

	// Send EHLO
	logger.LogDebug(slogLogger, "Sending EHLO command")
	caps, err := client.EHLO("smtptool.local")
	if err != nil {
		logger.LogError(slogLogger, "EHLO failed", "error", err)
		if logErr := writeSMTPCSVRow(csvLogger, []string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
			"", "", "", "", "", "", "", "", "", // No TLS info on EHLO failure
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}

	// Handle TLS: either already established via SMTPS, or upgrade via STARTTLS
	var tlsState *tls.ConnectionState
	if config.SMTPS {
		// For SMTPS, TLS is already established
		tlsState = client.GetTLSState()
		if config.VerboseMode && tlsState != nil {
			displayComprehensiveTLSInfo(tlsState, client.GetHost(), config.VerboseMode)
		}
	} else if !config.NoStartTLS && (config.Port == 25 || config.Port == 587 || config.Port == 2525 || config.Port == 2526 || config.Port == 1025) && caps.SupportsSTARTTLS() {
		// STARTTLS if on common SMTP submission ports and available
		// Ports: 25 (SMTP), 587 (Submission), 2525/2526 (Alternative submission), 1025 (Testing/Alt)
		fmt.Println("Upgrading to TLS...")
		tlsVersion := smtptls.ParseTLSVersion(config.TLSVersion)
		tlsConfig := &tls.Config{
			ServerName:         client.GetHost(), // resolved MX hostname if --use-mx, otherwise --host
			InsecureSkipVerify: config.SkipVerify,
			MinVersion:         tlsVersion,
			MaxVersion:         tlsVersion, // Force exact TLS version
		}

		tlsState, err = client.StartTLS(tlsConfig)
		if err != nil {
			logger.LogError(slogLogger, "STARTTLS failed", "error", err)
			if logErr := writeSMTPCSVRow(csvLogger, []string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
				"", "", "", "", "", "", "", "", "", // No TLS info on STARTTLS failure
				fmt.Sprintf("STARTTLS failed: %v", err),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("STARTTLS failed: %w", err)
		}

		fmt.Println("✓ TLS upgrade successful")

		// Show TLS cipher information in verbose mode
		if config.VerboseMode {
			displayComprehensiveTLSInfo(tlsState, client.GetHost(), config.VerboseMode)
		}

		// Re-run EHLO on encrypted connection
		caps, err = client.EHLO("smtptool.local")
		if err != nil {
			return fmt.Errorf("EHLO on encrypted connection failed: %w", err)
		}
	}

	// Authenticate if credentials provided (password or access token)
	if config.Username != "" && (config.Password != "" || config.AccessToken != "") {
		fmt.Println("Authenticating...")
		authMechanisms := caps.GetAuthMechanisms()
		methodToUse := selectAuthMechanism([]string{config.AuthMethod}, authMechanisms, config.AccessToken != "")

		if methodToUse == "" {
			msg := "No compatible authentication mechanism found"
			tlsData := formatTLSInfoForCSV(tlsState, client.GetHost())
			if logErr := writeSMTPCSVRow(csvLogger, []string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
				tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
				tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
				tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
				msg,
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("no compatible authentication mechanism found")
		}

		if err := client.Auth(config.Username, config.Password, config.AccessToken, []string{methodToUse}); err != nil {
			logger.LogError(slogLogger, "Authentication failed",
				"error", err,
				"username", security.MaskUsername(config.Username),
				"password", security.MaskPassword(config.Password),
				"accesstoken", security.MaskAccessToken(config.AccessToken),
				"method", methodToUse)

			// Show TLS cipher information on auth failure if verbose and TLS was used
			if config.VerboseMode && tlsState != nil {
				fmt.Println("\nAuthentication failed. TLS Connection Details:")
				displayComprehensiveTLSInfo(tlsState, client.GetHost(), config.VerboseMode)
			}

			tlsData := formatTLSInfoForCSV(tlsState, client.GetHost())
			if logErr := writeSMTPCSVRow(csvLogger, []string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
				tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
				tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
				tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
				fmt.Sprintf("Auth failed: %v", err),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("authentication failed: %w", err)
		}

		fmt.Println("✓ Authentication successful")
	}

	// Build email message
	messageData, err := buildMIMEMessage(config, slogLogger)
	if err != nil {
		logger.LogError(slogLogger, "Failed to build email message", "error", err)
		if logErr := writeSMTPCSVRow(csvLogger, []string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
			"", "", "", "", "", "", "", "", "",
			fmt.Sprintf("Failed to build message: %v", err),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("failed to build email message: %w", err)
	}
	messageID := generateMessageID(config.Host)

	// Send email. The SMTP envelope (RCPT TO) includes To, Cc, and Bcc
	// recipients; only To and Cc appear in the message headers.
	envelopeRecipients := collectEnvelopeRecipients(config)

	fmt.Println("\nSending message...")
	logger.LogDebug(slogLogger, "Sending email", "from", config.From, "to", config.To, "cc", config.Cc, "bcc", config.Bcc)

	err = client.SendMail(config.From, envelopeRecipients, messageData)
	if err != nil {
		logger.LogError(slogLogger, "Failed to send email", "error", err)
		tlsData := formatTLSInfoForCSV(tlsState, client.GetHost())
		if logErr := writeSMTPCSVRow(csvLogger, []string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject, bodyType, attachmentCountStr, "", "",
			tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
			tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
			tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Println("✓ Message sent successfully")
	fmt.Printf("  Message-ID: <%s>\n", messageID)

	// Log to CSV
	tlsData := formatTLSInfoForCSV(tlsState, client.GetHost())
	if logErr := writeSMTPCSVRow(csvLogger, []string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		config.ConnectAddress, config.From, strings.Join(config.To, ", "), strings.Join(config.Cc, ", "), strings.Join(config.Bcc, ", "), config.Subject,
		bodyType, attachmentCountStr,
		"250", messageID,
		tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
		tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
		tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
		"",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	fmt.Println("\n✓ Email sending test completed successfully")
	logger.LogInfo(slogLogger, "sendmail completed successfully", "messageID", messageID)

	return nil
}

func writeSMTPCSVHeader(csvLogger logger.Logger, columns []string) error {
	if csvLogger == nil {
		return nil
	}

	shouldWrite, err := csvLogger.ShouldWriteHeader()
	if err != nil {
		return err
	}
	if !shouldWrite {
		return nil
	}

	return csvLogger.WriteHeader(columns)
}

func writeSMTPCSVRow(csvLogger logger.Logger, row []string) error {
	if csvLogger == nil {
		return nil
	}

	return csvLogger.WriteRow(row)
}

// buildEmailMessage constructs an RFC 5322 email message.
// Defense-in-Depth: Email headers (From, To, Subject) are sanitized to remove
// CRLF sequences that could be used for header injection attacks. The message
// body is written as provided after the header/body separator. Body safety
// controls (e.g. DATA dot-stuffing) are handled at the SMTP transport layer.
func buildEmailMessage(from string, to, cc []string, subject, body, priority string) []byte {
	messageID := generateMessageID("")
	date := time.Now().Format(time.RFC1123Z)

	// Sanitize header fields to prevent header injection
	sanitizedFrom := sanitizeEmailHeader(from)
	sanitizedSubject := sanitizeEmailHeader(subject)
	sanitizedTo := sanitizeEmailHeaders(to)
	message := fmt.Sprintf("Message-ID: <%s>\r\n", messageID)
	message += fmt.Sprintf("Date: %s\r\n", date)
	message += fmt.Sprintf("From: %s\r\n", sanitizedFrom)
	message += fmt.Sprintf("To: %s\r\n", strings.Join(sanitizedTo, ", "))
	if len(cc) > 0 {
		message += fmt.Sprintf("Cc: %s\r\n", strings.Join(sanitizeEmailHeaders(cc), ", "))
	}
	message += fmt.Sprintf("Subject: %s\r\n", sanitizedSubject)
	for _, line := range priorityHeaderLines(priority) {
		message += line + "\r\n"
	}
	message += "MIME-Version: 1.0\r\n"
	message += "Content-Type: text/plain; charset=UTF-8\r\n"
	message += "\r\n"
	message += body
	message += "\r\n"

	return []byte(message)
}

// buildMIMEMessage constructs an RFC 5322 email message, adding MIME multipart
// structure as needed for an HTML body, attachments, inline attachments, and/or
// custom headers. If none of these extras are configured, it falls back to the
// simple plain-text message produced by buildEmailMessage (unchanged behavior).
func buildMIMEMessage(config *Config, slogLogger *slog.Logger) ([]byte, error) {
	hasExtras := config.BodyHTML != "" || len(config.Attachments) > 0 || len(config.InlineAttachments) > 0 || len(config.Headers) > 0
	if !hasExtras {
		return buildEmailMessage(config.From, config.To, config.Cc, config.Subject, config.Body, config.Priority), nil
	}

	customHeaders, err := email.ParseHeaders(config.Headers)
	if err != nil {
		return nil, err
	}

	onSkip := func(path string, loadErr error) {
		logger.LogWarn(slogLogger, "Could not read attachment file", "path", path, "error", loadErr)
	}

	attachments, err := email.LoadAttachments(config.Attachments, onSkip)
	if err != nil {
		return nil, fmt.Errorf("attachments: %w", err)
	}
	inlineAttachments, err := email.LoadInlineAttachments(config.InlineAttachments, onSkip)
	if err != nil {
		return nil, fmt.Errorf("inline attachments: %w", err)
	}

	// Build the body content (text/HTML, with inline attachments and file attachments
	// nested as needed) and determine the top-level Content-Type.
	contentType, body, err := buildMIMEBody(config.Body, config.BodyHTML, inlineAttachments, attachments)
	if err != nil {
		return nil, err
	}

	messageID := generateMessageID("")
	date := time.Now().Format(time.RFC1123Z)

	sanitizedFrom := sanitizeEmailHeader(config.From)
	sanitizedSubject := sanitizeEmailHeader(config.Subject)
	sanitizedTo := sanitizeEmailHeaders(config.To)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Message-ID: <%s>\r\n", messageID)
	fmt.Fprintf(&buf, "Date: %s\r\n", date)
	fmt.Fprintf(&buf, "From: %s\r\n", sanitizedFrom)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(sanitizedTo, ", "))
	if len(config.Cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(sanitizeEmailHeaders(config.Cc), ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", sanitizedSubject)
	for _, line := range priorityHeaderLines(config.Priority) {
		buf.WriteString(line + "\r\n")
	}
	for _, h := range customHeaders {
		fmt.Fprintf(&buf, "%s: %s\r\n", h.Name, h.Value)
	}
	buf.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: %s\r\n", contentType)
	buf.WriteString("\r\n")
	buf.Write(body)

	return buf.Bytes(), nil
}

// buildMIMEBody assembles the MIME body for a message, nesting parts as needed:
//
//	multipart/mixed          (only if file attachments are present)
//	  multipart/related      (only if inline attachments are present)
//	    multipart/alternative (only if both plain text and HTML bodies are present)
//	      text/plain
//	      text/html
//	    inline attachment parts (cid:...)
//	  attachment parts
//
// It returns the Content-Type header value and body bytes for the outermost part.
func buildMIMEBody(textBody, htmlBody string, inline, attachments []email.Attachment) (string, []byte, error) {
	contentType, body, err := textOrAlternativePart(textBody, htmlBody)
	if err != nil {
		return "", nil, err
	}

	contentType, body, err = wrapRelatedPart(contentType, body, inline)
	if err != nil {
		return "", nil, err
	}

	return wrapMixedPart(contentType, body, attachments)
}

// textOrAlternativePart returns a single text/plain or text/html part, or
// (if both bodies are provided) a multipart/alternative wrapping both.
func textOrAlternativePart(textBody, htmlBody string) (string, []byte, error) {
	if textBody != "" && htmlBody != "" {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)

		textHeader := textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}}
		pw, err := mw.CreatePart(textHeader)
		if err != nil {
			return "", nil, err
		}
		if _, err := pw.Write([]byte(textBody)); err != nil {
			return "", nil, err
		}

		htmlHeader := textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}}
		pw, err = mw.CreatePart(htmlHeader)
		if err != nil {
			return "", nil, err
		}
		if _, err := pw.Write([]byte(htmlBody)); err != nil {
			return "", nil, err
		}

		if err := mw.Close(); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("multipart/alternative; boundary=%s", mw.Boundary()), buf.Bytes(), nil
	}

	if htmlBody != "" {
		return "text/html; charset=UTF-8", []byte(htmlBody), nil
	}

	return "text/plain; charset=UTF-8", []byte(textBody), nil
}

// wrapRelatedPart wraps the given part in a multipart/related part alongside
// inline attachments. If there are no inline attachments, the part is returned
// unchanged.
func wrapRelatedPart(contentType string, body []byte, inline []email.Attachment) (string, []byte, error) {
	if len(inline) == 0 {
		return contentType, body, nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	header := textproto.MIMEHeader{"Content-Type": {contentType}}
	pw, err := mw.CreatePart(header)
	if err != nil {
		return "", nil, err
	}
	if _, err := pw.Write(body); err != nil {
		return "", nil, err
	}

	for _, att := range inline {
		if err := writeAttachmentPart(mw, att); err != nil {
			return "", nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return "", nil, err
	}

	return fmt.Sprintf("multipart/related; boundary=%s", mw.Boundary()), buf.Bytes(), nil
}

// wrapMixedPart wraps the given part in a multipart/mixed part alongside file
// attachments. If there are no attachments, the part is returned unchanged.
func wrapMixedPart(contentType string, body []byte, attachments []email.Attachment) (string, []byte, error) {
	if len(attachments) == 0 {
		return contentType, body, nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	header := textproto.MIMEHeader{"Content-Type": {contentType}}
	pw, err := mw.CreatePart(header)
	if err != nil {
		return "", nil, err
	}
	if _, err := pw.Write(body); err != nil {
		return "", nil, err
	}

	for _, att := range attachments {
		if err := writeAttachmentPart(mw, att); err != nil {
			return "", nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return "", nil, err
	}

	return fmt.Sprintf("multipart/mixed; boundary=%s", mw.Boundary()), buf.Bytes(), nil
}

// writeAttachmentPart writes a single base64-encoded attachment part (inline or
// regular) to the given multipart writer.
func writeAttachmentPart(mw *multipart.Writer, att email.Attachment) error {
	header := textproto.MIMEHeader{
		"Content-Type":              {att.ContentType},
		"Content-Transfer-Encoding": {"base64"},
	}
	if att.Inline {
		header.Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", att.Name))
		header.Set("Content-ID", fmt.Sprintf("<%s>", att.ContentID))
	} else {
		header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Name))
	}

	pw, err := mw.CreatePart(header)
	if err != nil {
		return err
	}

	return writeBase64(pw, att.Data)
}

// writeBase64 writes base64-encoded data in RFC 2045 compliant lines of 76
// characters, terminated with CRLF.
func writeBase64(w io.Writer, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		if _, err := w.Write([]byte(encoded[i:end])); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return err
		}
	}
	return nil
}

// sanitizeEmailHeader removes CRLF sequences from email header values to prevent
// header injection attacks. This is a defense-in-depth measure.
func sanitizeEmailHeader(header string) string {
	header = strings.ReplaceAll(header, "\r", "")
	header = strings.ReplaceAll(header, "\n", "")
	return header
}

// priorityHeaderLines returns the "Name: Value" header lines (without CRLF)
// for the given priority. "normal" (and any other value) adds no headers,
// since it matches default mail client behavior.
func priorityHeaderLines(priority string) []string {
	switch priority {
	case "high":
		return []string{"X-Priority: 1 (Highest)", "Importance: High", "Priority: urgent"}
	case "low":
		return []string{"X-Priority: 5 (Lowest)", "Importance: Low", "Priority: non-urgent"}
	default:
		return nil
	}
}

// collectEnvelopeRecipients returns the full list of SMTP envelope (RCPT TO)
// recipients: To, then Cc, then Bcc, in that order.
func collectEnvelopeRecipients(config *Config) []string {
	recipients := make([]string, 0, len(config.To)+len(config.Cc)+len(config.Bcc))
	recipients = append(recipients, config.To...)
	recipients = append(recipients, config.Cc...)
	recipients = append(recipients, config.Bcc...)
	return recipients
}

// sanitizeEmailHeaders applies sanitizeEmailHeader to each address in a list.
func sanitizeEmailHeaders(addrs []string) []string {
	sanitized := make([]string, len(addrs))
	for i, addr := range addrs {
		sanitized[i] = sanitizeEmailHeader(addr)
	}
	return sanitized
}

// generateMessageID creates a unique message ID.
func generateMessageID(host string) string {
	timestamp := time.Now().UnixNano()
	if host == "" {
		host = "smtptool"
	}
	return fmt.Sprintf("%d.smtptool@%s", timestamp, host)
}
