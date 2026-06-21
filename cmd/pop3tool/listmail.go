package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gomailtesttool/internal/common/logger"
)

// listMail lists messages in the mailbox.
func listMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing messages on %s:%d...\n", config.Host, config.Port)

	// CSV columns for listmail
	columns := []string{"Action", "Status", "Server", "Port", "Total_Messages", "Total_Size", "Message_Number", "Message_Size", "UIDL", "Error"}
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
			"username", maskUsername(config.Username))

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

	fmt.Printf("\nMessages (showing %d of %d):\n", displayCount, len(messages))
	fmt.Println("  Num    Size       UIDL")
	fmt.Println("  ---    ----       ----")

	for i := 0; i < displayCount; i++ {
		msg := messages[i]
		uidl := ""
		if uidlMap != nil {
			uidl = uidlMap[msg.Number]
		}

		fmt.Printf("  %3d    %8d   %s\n", msg.Number, msg.Size, uidl)

		// Log each message to CSV
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
			fmt.Sprintf("%d", count), fmt.Sprintf("%d", size),
			fmt.Sprintf("%d", msg.Number), fmt.Sprintf("%d", msg.Size), uidl, "",
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	if len(messages) > config.MaxMessages {
		fmt.Printf("\n  ... and %d more messages (use -maxmessages to show more)\n", len(messages)-config.MaxMessages)
	}

	logger.LogInfo(slogLogger, "List mail completed",
		"host", config.Host,
		"total_messages", count,
		"total_size", size)

	fmt.Println("\n✓ List mail completed")
	return nil
}
