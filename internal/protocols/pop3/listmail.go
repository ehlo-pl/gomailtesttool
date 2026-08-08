package pop3

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// listMail lists messages in the mailbox.
func listMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing messages on %s:%d...\n", config.Host, config.Port)

	// CSV columns for listmail
	columns := []string{"Action", "Status", "Server", "Port", "Total_Messages", "Total_Size", "Message_Number", "Subject", "From", "To", "Date", "MessageID", "UIDL", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewPOP3Client(config)

	// Connect to server
	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed",
			"error", err,
			"host", config.Host,
			"port", config.Port)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"", "", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Quit() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	// Upgrade to TLS if needed
	if config.StartTLS && client.GetTLSState() == nil {
		fmt.Println("Upgrading to TLS via STLS...")
		if err := client.StartTLS(nil); err != nil {
			logger.LogError(slogLogger, "STLS upgrade failed", "error", err)

			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				"", "", "", "", "", fmt.Sprintf("STLS failed: %v", err),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("STLS failed: %w", err)
		}
		fmt.Println("✓ TLS upgrade successful")
	}

	// Get capabilities
	caps, _ := client.Capabilities(ctx)

	// Authenticate
	authMethod := config.AuthMethod
	if strings.EqualFold(authMethod, "auto") {
		if config.AccessToken != "" {
			if caps != nil && caps.SupportsXOAUTH2() {
				authMethod = "XOAUTH2"
			} else {
				authMethod = "USER"
			}
		} else {
			authMethod = "USER"
		}
	}

	fmt.Printf("Authenticating with method: %s\n", authMethod)

	var authErr error
	if config.AccessToken != "" && strings.EqualFold(authMethod, "XOAUTH2") {
		authErr = client.Auth(ctx, config.Username, "", config.AccessToken)
	} else {
		authErr = client.Auth(ctx, config.Username, config.Password, "")
	}

	if authErr != nil {
		logger.LogError(slogLogger, "Authentication failed",
			"error", authErr,
			"username", security.MaskUsername(config.Username))

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"", "", "", "", "", fmt.Sprintf("Auth failed: %v", authErr),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("authentication failed: %w", authErr)
	}
	fmt.Println("✓ Authentication successful")

	// Get mailbox statistics
	count, size, err := client.Stat(ctx)
	if err != nil {
		logger.LogError(slogLogger, "STAT command failed", "error", err)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"", "", "", "", "", fmt.Sprintf("STAT failed: %v", err),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("STAT failed: %w", err)
	}

	fmt.Printf("\nMailbox Statistics:\n")
	fmt.Printf("  Total messages: %d\n", count)
	fmt.Printf("  Total size: %d bytes\n", size)

	if count == 0 {
		fmt.Println("\nNo messages in mailbox")
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
			fmt.Sprintf("%d", count), fmt.Sprintf("%d", size), "", "", "", "",
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return nil
	}

	// List messages
	messages, err := client.List(ctx)
	if err != nil {
		logger.LogError(slogLogger, "LIST command failed", "error", err)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			fmt.Sprintf("%d", count), fmt.Sprintf("%d", size), "", "", "", fmt.Sprintf("LIST failed: %v", err),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("LIST failed: %w", err)
	}

	// Try to get UIDLs if supported
	var uidlMap map[int]string
	if caps != nil && caps.SupportsUIDL() {
		uidls, err := client.UIDL(ctx)
		if err == nil {
			uidlMap = make(map[int]string)
			for _, msg := range uidls {
				uidlMap[msg.Number] = msg.UIDL
			}
		}
	}

	// Display messages (limited by MaxMessages)
	displayCount := len(messages)
	if displayCount > config.MaxMessages {
		displayCount = config.MaxMessages
	}

	fmt.Printf("\nMessages (showing %d of %d):\n\n", displayCount, len(messages))

	for i := 0; i < displayCount; i++ {
		msg := messages[i]
		uidl := ""
		if uidlMap != nil {
			uidl = uidlMap[msg.Number]
		}

		// Fetch message headers via TOP to extract envelope fields.
		subject, from, to, date, messageID := "", "", "", "", ""
		if headerBytes, topErr := client.Top(ctx, msg.Number, 0); topErr == nil {
			subject = extractHeader(headerBytes, "Subject")
			from = extractHeader(headerBytes, "From")
			to = extractHeader(headerBytes, "To")
			date = extractHeader(headerBytes, "Date")
			messageID = extractHeader(headerBytes, "Message-ID")
		}

		fmt.Printf("%d. Subject: %s\n   From: %s\n   To: %s\n   Date: %s\n   Message-ID: %s\n   UIDL: %s\n\n",
			i+1, subject, from, to, date, messageID, uidl)

		// Log each message to CSV
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
			fmt.Sprintf("%d", count), fmt.Sprintf("%d", size),
			fmt.Sprintf("%d", msg.Number), subject, from, to, date, messageID, uidl, "",
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	if len(messages) > config.MaxMessages {
		fmt.Printf("\n  ... and %d more messages (use --maxmessages to show more)\n", len(messages)-config.MaxMessages)
	}

	logger.LogInfo(slogLogger, "List mail completed",
		"host", config.Host,
		"total_messages", count,
		"total_size", size)

	fmt.Println("✓ List mail completed")
	return nil
}

// extractHeader extracts the value of the first occurrence of a header field
// from raw message bytes returned by TOP. It handles folded (multi-line) header
// values by joining continuation lines.
func extractHeader(raw []byte, name string) string {
	lines := strings.Split(string(raw), "\n")
	lowerName := strings.ToLower(name) + ":"
	var value strings.Builder
	collecting := false
	for _, line := range lines {
		if line == "" || line == "\r" {
			// Blank line marks end of headers.
			break
		}
		if collecting {
			// Continuation line starts with whitespace.
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				value.WriteString(" ")
				value.WriteString(strings.TrimRight(strings.TrimLeft(line, " \t"), "\r"))
				continue
			}
			// New header – stop collecting.
			break
		}
		if strings.HasPrefix(strings.ToLower(line), lowerName) {
			v := strings.TrimLeft(line[len(lowerName):], " \t")
			value.WriteString(strings.TrimRight(v, "\r"))
			collecting = true
		}
	}
	return value.String()
}
