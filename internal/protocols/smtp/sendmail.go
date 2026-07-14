package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	mimebuilder "github.com/ehlo-pl/gomailtesttool/internal/common/mime"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
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
		tlsVersion := tlsutil.ParseTLSVersion(config.TLSVersion)
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

// buildEmailMessage constructs a simple RFC 5322 plain-text email message.
// It delegates assembly to the shared mime builder. The plain-text path loads
// no attachments and parses no custom headers, so Build cannot fail. SMTP omits
// Bcc from the headers — recipients travel in the envelope (RCPT TO).
func buildEmailMessage(from string, to, cc []string, subject, body, priority string) []byte {
	data, _ := mimebuilder.Build(mimebuilder.Message{
		From:      from,
		To:        to,
		Cc:        cc,
		Subject:   subject,
		TextBody:  body,
		Priority:  priority,
		MessageID: generateMessageID(""),
	})
	return data
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

	// SMTP omits Bcc from the message headers — the envelope carries recipients.
	return mimebuilder.Build(mimebuilder.Message{
		From:        config.From,
		To:          config.To,
		Cc:          config.Cc,
		Subject:     config.Subject,
		TextBody:    config.Body,
		HTMLBody:    config.BodyHTML,
		Priority:    config.Priority,
		Headers:     customHeaders,
		Attachments: attachments,
		Inline:      inlineAttachments,
		MessageID:   generateMessageID(""),
	})
}

// sanitizeEmailHeader removes CRLF sequences from email header values to prevent
// header injection attacks. This is a defense-in-depth measure.
func sanitizeEmailHeader(header string) string {
	return mimebuilder.SanitizeHeader(header)
}

// priorityHeaderLines returns the "Name: Value" header lines (without CRLF)
// for the given priority.
func priorityHeaderLines(priority string) []string {
	return mimebuilder.PriorityHeaderLines(priority)
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

// generateMessageID creates a unique message ID in the SMTP tool's historic
// "<timestamp>.smtptool@<host>" form.
func generateMessageID(host string) string {
	return mimebuilder.GenerateMessageID(host, "smtptool")
}
