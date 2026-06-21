package imap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// exportMessages searches the configured mailbox for messages matching
// config.MessageID and/or config.Subject and exports each match as a raw
// .eml (RFC822) file.
func exportMessages(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Searching %s on %s:%d for matching messages...\n", config.Mailbox, config.Host, config.Port)

	columns := []string{"Action", "Status", "Server", "Port", "Mailbox", "Subject", "UID", "Filename", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewIMAPClient(config)

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err, "host", config.Host, "port", config.Port)
		writeExportRow(csvLogger, slogLogger, config, false, "", "", err.Error())
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
		logger.LogError(slogLogger, "Authentication failed", "error", authErr, "username", maskUsername(config.Username))
		writeExportRow(csvLogger, slogLogger, config, false, "", "", fmt.Sprintf("Auth failed: %v", authErr))
		return fmt.Errorf("authentication failed: %w", authErr)
	}
	fmt.Println("✓ Authentication successful")

	if err := client.SelectMailbox(ctx, config.Mailbox); err != nil {
		logger.LogError(slogLogger, "SELECT failed", "error", err, "mailbox", config.Mailbox)
		writeExportRow(csvLogger, slogLogger, config, false, "", "", err.Error())
		return err
	}

	uids, err := client.SearchMessages(ctx, config.MessageID, config.Subject)
	if err != nil {
		logger.LogError(slogLogger, "SEARCH failed", "error", err)
		writeExportRow(csvLogger, slogLogger, config, false, "", "", err.Error())
		return err
	}

	if len(uids) == 0 {
		fmt.Println("No messages found matching the given criteria.")
		writeExportRow(csvLogger, slogLogger, config, true, "", "", "No messages found (0 messages)")
		return nil
	}

	if len(uids) > config.Count {
		uids = uids[:config.Count]
	}

	exportDir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}
	fmt.Printf("Export directory: %s\n", exportDir)

	successCount := 0
	for _, uid := range uids {
		uidStr := fmt.Sprintf("%d", uid)

		body, err := client.FetchRFC822(ctx, uid)
		if err != nil {
			logger.LogError(slogLogger, "FETCH failed", "error", err, "uid", uint32(uid))
			writeExportRow(csvLogger, slogLogger, config, false, uidStr, "", err.Error())
			continue
		}

		filename := fmt.Sprintf("msg_%s.eml", export.SanitizeFilename(uidStr))
		filePath := filepath.Join(exportDir, filename)
		if err := os.WriteFile(filePath, body, 0644); err != nil {
			logger.LogError(slogLogger, "Failed to write EML file", "error", err, "uid", uint32(uid))
			writeExportRow(csvLogger, slogLogger, config, false, uidStr, "", err.Error())
			continue
		}

		successCount++
		fmt.Printf("Successfully exported UID %d -> %s\n", uid, filePath)
		writeExportRow(csvLogger, slogLogger, config, true, uidStr, filePath, "")
	}

	fmt.Printf("Successfully exported %d/%d messages.\n", successCount, len(uids))
	return nil
}

// writeExportRow writes a single CSV row for the exportmessages action.
func writeExportRow(csvLogger logger.Logger, slogLogger *slog.Logger, config *Config, success bool, uid, filename, errStr string) {
	status := "FAILURE"
	if success {
		status = "SUCCESS"
	}
	if logErr := csvLogger.WriteRow([]string{
		ActionExportMessages, status, config.Host, fmt.Sprintf("%d", config.Port),
		config.Mailbox, config.Subject, uid, filename, errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
