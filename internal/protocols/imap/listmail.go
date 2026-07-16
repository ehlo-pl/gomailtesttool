package imap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// listMail lists envelope data for the newest messages of the configured
// mailbox (default INBOX), newest first.
func listMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing messages in %s on %s:%d...\n", config.Mailbox, config.Host, config.Port)

	columns := []string{"Action", "Status", "Server", "Port", "Mailbox", "UID", "Subject", "From", "Date", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewIMAPClient(config)

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err, "host", config.Host, "port", config.Port)
		writeListMailRow(csvLogger, slogLogger, config, false, MessageSummary{}, err.Error())
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Logout() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	caps := client.GetCapabilities()
	authMethod := config.AuthMethod
	if strings.EqualFold(authMethod, "auto") {
		if config.AccessToken != "" {
			if caps != nil && caps.SupportsXOAUTH2() {
				authMethod = "XOAUTH2"
			} else {
				authMethod = "PLAIN"
			}
		} else if caps != nil && caps.SupportsPlain() {
			authMethod = "PLAIN"
		} else {
			authMethod = "LOGIN"
		}
	}

	var authErr error
	if config.AccessToken != "" && strings.EqualFold(authMethod, "XOAUTH2") {
		authErr = client.Auth(ctx, config.Username, "", config.AccessToken)
	} else {
		authErr = client.Auth(ctx, config.Username, config.Password, "")
	}
	if authErr != nil {
		logger.LogError(slogLogger, "Authentication failed", "error", authErr, "username", security.MaskUsername(config.Username))
		writeListMailRow(csvLogger, slogLogger, config, false, MessageSummary{}, fmt.Sprintf("Auth failed: %v", authErr))
		return fmt.Errorf("authentication failed: %w", authErr)
	}
	fmt.Println("✓ Authentication successful")

	if err := client.SelectMailbox(ctx, config.Mailbox); err != nil {
		logger.LogError(slogLogger, "SELECT failed", "error", err, "mailbox", config.Mailbox)
		writeListMailRow(csvLogger, slogLogger, config, false, MessageSummary{}, err.Error())
		return err
	}

	messages, err := client.ListMessages(ctx, config.Count)
	if err != nil {
		logger.LogError(slogLogger, "FETCH failed", "error", err, "mailbox", config.Mailbox)
		writeListMailRow(csvLogger, slogLogger, config, false, MessageSummary{}, err.Error())
		return err
	}

	if len(messages) == 0 {
		fmt.Printf("No messages found in %s.\n", config.Mailbox)
		writeListMailRow(csvLogger, slogLogger, config, true, MessageSummary{Subject: "No messages found (0 messages)"}, "")
		return nil
	}

	fmt.Printf("\nNewest %d messages in %s:\n\n", len(messages), config.Mailbox)
	for i, msg := range messages {
		fmt.Printf("%d. Subject: %s\n   From: %s\n   Date: %s\n   UID: %d\n\n", i+1, msg.Subject, msg.From, msg.Date, msg.UID)
		writeListMailRow(csvLogger, slogLogger, config, true, msg, "")
	}

	logger.LogInfo(slogLogger, "List mail completed", "host", config.Host, "mailbox", config.Mailbox, "message_count", len(messages))
	fmt.Printf("✓ Listed %d messages\n", len(messages))
	return nil
}

// writeListMailRow writes a single CSV row for the listmail action.
func writeListMailRow(csvLogger logger.Logger, slogLogger *slog.Logger, config *Config, success bool, msg MessageSummary, errStr string) {
	status := "FAILURE"
	if success {
		status = "SUCCESS"
	}
	uid := ""
	if msg.UID != 0 {
		uid = fmt.Sprintf("%d", msg.UID)
	}
	if logErr := csvLogger.WriteRow([]string{
		ActionListMail, status, config.Host, fmt.Sprintf("%d", config.Port),
		config.Mailbox, uid, msg.Subject, msg.From, msg.Date, errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
