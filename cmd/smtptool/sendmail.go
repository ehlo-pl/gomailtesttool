package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gomailtesttool/internal/common/logger"
	smtptls "gomailtesttool/internal/smtp/tls"
)

// sendMail performs end-to-end email sending test.
func sendMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.SMTPS {
		fmt.Printf("Sending test email via %s:%d (SMTPS)...\n\n", config.Host, config.Port)
	} else {
		fmt.Printf("Sending test email via %s:%d...\n\n", config.Host, config.Port)
	}

	// Write CSV header
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "From", "To",
			"Subject", "SMTP_Response_Code", "Message_ID", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	fmt.Printf("From:    %s\n", config.From)
	fmt.Printf("To:      %s\n", strings.Join(config.To, ", "))
	fmt.Printf("Subject: %s\n\n", config.Subject)

	// Create and connect client
	client := NewSMTPClient(config.Host, config.Port, config)
	logger.LogDebug(slogLogger, "Connecting to SMTP server")

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.From, strings.Join(config.To, ", "), config.Subject, "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}
	defer client.Close()

	if config.SMTPS {
		fmt.Printf("✓ Connected with SMTPS (implicit TLS)\n")
	} else {
		fmt.Printf("✓ Connected\n")
	}

	// Send EHLO
	logger.LogDebug(slogLogger, "Sending EHLO command")
	caps, err := client.EHLO("smtptool.local")
	if err != nil {
		logger.LogError(slogLogger, "EHLO failed", "error", err)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.From, strings.Join(config.To, ", "), config.Subject, "", "", err.Error(),
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
			displayTLSCipherInfo(tlsState)
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
			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.From, strings.Join(config.To, ", "), config.Subject, "", "", fmt.Sprintf("STARTTLS failed: %v", err),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("STARTTLS failed: %w", err)
		}

		fmt.Println("✓ TLS upgrade successful")

		// Show TLS cipher information in verbose mode
		if config.VerboseMode {
			displayTLSCipherInfo(tlsState)
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
			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.From, strings.Join(config.To, ", "), config.Subject, "", "", msg,
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
				displayTLSCipherInfo(tlsState)
			}

			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.From, strings.Join(config.To, ", "), config.Subject, "", "", fmt.Sprintf("Auth failed: %v", err),
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
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.From, strings.Join(config.To, ", "), config.Subject, "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Println("✓ Message sent successfully")
	fmt.Printf("  Message-ID: <%s>\n", messageID)

	// Log to CSV
	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		config.From, strings.Join(config.To, ", "), config.Subject,
		"250", messageID, "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	fmt.Println("\n✓ Email sending test completed successfully")
	logger.LogInfo(slogLogger, "sendmail completed successfully", "messageID", messageID)

	return nil
}

// buildEmailMessage constructs an RFC 5322 email message.
// Defense-in-Depth: Email headers (From, To, Subject) are sanitized to remove
// CRLF sequences that could be used for header injection attacks. The message
// body is not sanitized as it legitimately may contain newlines.
func buildEmailMessage(from string, to []string, subject, body string) []byte {
	messageID := generateMessageID("")
	date := time.Now().Format(time.RFC1123Z)

	// Sanitize header fields to prevent header injection
	from = sanitizeEmailHeader(from)
	subject = sanitizeEmailHeader(subject)
	sanitizedTo := make([]string, len(to))
	for i, addr := range to {
		sanitizedTo[i] = sanitizeEmailHeader(addr)
	}

	message := fmt.Sprintf("Message-ID: <%s>\r\n", messageID)
	message += fmt.Sprintf("Date: %s\r\n", date)
	message += fmt.Sprintf("From: %s\r\n", from)
	message += fmt.Sprintf("To: %s\r\n", strings.Join(sanitizedTo, ", "))
	message += fmt.Sprintf("Subject: %s\r\n", subject)
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

// generateMessageID creates a unique message ID.
func generateMessageID(host string) string {
	timestamp := time.Now().UnixNano()
	if host == "" {
		host = "smtptool"
	}
	return fmt.Sprintf("%d.smtptool@%s", timestamp, host)
}

// displayTLSCipherInfo displays negotiated TLS cipher suite information.
// This is shown in verbose mode to help debug TLS connection issues.
func displayTLSCipherInfo(state *tls.ConnectionState) {
	if state == nil {
		return
	}

	fmt.Println("\nTLS Connection Information:")
	fmt.Printf("  TLS Version:       %s\n", getTLSVersionName(state.Version))
	fmt.Printf("  Cipher Suite:      %s (0x%04X)\n", tls.CipherSuiteName(state.CipherSuite), state.CipherSuite)
	fmt.Printf("  Server Name:       %s\n", state.ServerName)
	fmt.Printf("  Negotiated Proto:  %s\n", state.NegotiatedProtocol)

	// Show certificate info
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		fmt.Printf("  Server Cert CN:    %s\n", cert.Subject.CommonName)
		fmt.Printf("  Cert Valid Until:  %s\n", cert.NotAfter.Format("2006-01-02 15:04:05 MST"))
	}

	// Show supported cipher suites (from client perspective)
	fmt.Println("\nSupported Cipher Suites (Client):")
	// Note: We can't easily get the full list of ciphers that were offered
	// during negotiation without instrumenting the TLS handshake, but we can
	// show what was negotiated and document what the Go TLS library typically offers
	fmt.Printf("  Negotiated: %s\n", tls.CipherSuiteName(state.CipherSuite))
	fmt.Println("  (Full client cipher list depends on Go TLS implementation)")
}

// getTLSVersionName returns a human-readable name for a TLS version.
func getTLSVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04X)", version)
	}
}
