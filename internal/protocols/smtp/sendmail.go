package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"msgraphtool/internal/common/logger"
	smtptls "msgraphtool/internal/smtp/tls"
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
		"Action", "Status", "Server", "Port", "Connect_Address", "From", "To",
		"Subject", "SMTP_Response_Code", "Message_ID",
		"TLS_Version", "Cipher_Suite", "Cipher_Strength",
		"Cert_Subject", "Cert_Issuer", "Cert_SANs",
		"Cert_Valid_From", "Cert_Valid_To", "Cert_Verification_Status",
		"Error",
	}); err != nil {
		logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
	}

	fmt.Printf("From:    %s\n", config.From)
	fmt.Printf("To:      %s\n", strings.Join(config.To, ", "))
	fmt.Printf("Subject: %s\n\n", config.Subject)

	// Create and connect client
	client := NewSMTPClient(config.Host, config.Port, config)
	logger.LogDebug(slogLogger, "Connecting to SMTP server")

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err)
		if logErr := writeSMTPCSVRow(csvLogger, []string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject, "", "",
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
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject, "", "",
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
			displayComprehensiveTLSInfo(tlsState, config.Host, config.VerboseMode)
		}
	} else if (config.Port == 25 || config.Port == 587 || config.Port == 2525 || config.Port == 2526 || config.Port == 1025) && caps.SupportsSTARTTLS() {
		// STARTTLS if on common SMTP submission ports and available
		// Ports: 25 (SMTP), 587 (Submission), 2525/2526 (Alternative submission), 1025 (Testing/Alt)
		fmt.Println("Upgrading to TLS...")
		tlsVersion := smtptls.ParseTLSVersion(config.TLSVersion)
		tlsConfig := &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: config.SkipVerify,
			MinVersion:         tlsVersion,
			MaxVersion:         tlsVersion, // Force exact TLS version
		}

		tlsState, err = client.StartTLS(tlsConfig)
		if err != nil {
			logger.LogError(slogLogger, "STARTTLS failed", "error", err)
			if logErr := writeSMTPCSVRow(csvLogger, []string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject, "", "",
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
			displayComprehensiveTLSInfo(tlsState, config.Host, config.VerboseMode)
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
			tlsData := formatTLSInfoForCSV(tlsState, config.Host)
			if logErr := writeSMTPCSVRow(csvLogger, []string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject, "", "",
				tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
				tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
				tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
				msg,
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return errors.New(msg)
		}

		if err := client.Auth(config.Username, config.Password, config.AccessToken, []string{methodToUse}); err != nil {
			logger.LogError(slogLogger, "Authentication failed",
				"error", err,
				"username", maskUsername(config.Username),
				"password", maskPassword(config.Password),
				"accesstoken", maskAccessToken(config.AccessToken),
				"method", methodToUse)

			// Show TLS cipher information on auth failure if verbose and TLS was used
			if config.VerboseMode && tlsState != nil {
				fmt.Println("\nAuthentication failed. TLS Connection Details:")
				displayComprehensiveTLSInfo(tlsState, config.Host, config.VerboseMode)
			}

			tlsData := formatTLSInfoForCSV(tlsState, config.Host)
			if logErr := writeSMTPCSVRow(csvLogger, []string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject, "", "",
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
	messageData := buildEmailMessage(config.From, config.To, config.Subject, config.Body)
	messageID := generateMessageID(config.Host)

	// Send email
	fmt.Println("\nSending message...")
	logger.LogDebug(slogLogger, "Sending email", "from", config.From, "to", config.To)

	err = client.SendMail(config.From, config.To, messageData)
	if err != nil {
		logger.LogError(slogLogger, "Failed to send email", "error", err)
		tlsData := formatTLSInfoForCSV(tlsState, config.Host)
		if logErr := writeSMTPCSVRow(csvLogger, []string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject, "", "",
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
	tlsData := formatTLSInfoForCSV(tlsState, config.Host)
	if logErr := writeSMTPCSVRow(csvLogger, []string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		config.ConnectAddress, config.From, strings.Join(config.To, ", "), config.Subject,
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
func buildEmailMessage(from string, to []string, subject, body string) []byte {
	messageID := generateMessageID("")
	date := time.Now().Format(time.RFC1123Z)

	// Sanitize header fields to prevent header injection
	sanitizedFrom := sanitizeEmailHeader(from)
	sanitizedSubject := sanitizeEmailHeader(subject)
	sanitizedTo := make([]string, len(to))
	for i, addr := range to {
		sanitizedTo[i] = sanitizeEmailHeader(addr)
	}
	message := fmt.Sprintf("Message-ID: <%s>\r\n", messageID)
	message += fmt.Sprintf("Date: %s\r\n", date)
	message += fmt.Sprintf("From: %s\r\n", sanitizedFrom)
	message += fmt.Sprintf("To: %s\r\n", strings.Join(sanitizedTo, ", "))
	message += fmt.Sprintf("Subject: %s\r\n", sanitizedSubject)
	message += "MIME-Version: 1.0\r\n"
	message += "Content-Type: text/plain; charset=UTF-8\r\n"
	message += "\r\n"
	message += body
	message += "\r\n"

	return []byte(message)
}

// sanitizeEmailHeader removes CRLF sequences from email header values to prevent
// header injection attacks. This is a defense-in-depth measure.
func sanitizeEmailHeader(header string) string {
	header = strings.ReplaceAll(header, "\r", "")
	header = strings.ReplaceAll(header, "\n", "")
	return header
}

// sanitizeEmailBody normalizes line endings and removes unsafe control
// characters from body content while preserving regular text formatting.
func sanitizeEmailBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	var b strings.Builder
	b.Grow(len(body))
	for _, r := range body {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// generateMessageID creates a unique message ID.
func generateMessageID(host string) string {
	timestamp := time.Now().UnixNano()
	if host == "" {
		host = "smtptool"
	}
	return fmt.Sprintf("%d.smtptool@%s", timestamp, host)
}
