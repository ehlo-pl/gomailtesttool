package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// exportMessages searches for messages matching messageID and/or subject,
// downloads each as raw RFC822 via the blob endpoint, and writes .eml files.
func exportMessages(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Searching and exporting messages from %s...\n\n", config.Host)

	columns := []string{"Action", "Status", "Server", "MessageId", "Subject", "FilePath", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	writeRow := func(msgId, subject, filePath, errStr string) {
		status := "SUCCESS"
		if errStr != "" {
			status = "FAILURE"
		}
		_ = csvLogger.WriteRow([]string{config.Action, status, config.Host, msgId, subject, filePath, errStr})
	}

	client := NewJMAPClient(config)

	session, err := client.Discover(ctx)
	if err != nil {
		logger.LogError(slogLogger, "JMAP discovery failed", "error", err)
		writeRow("", "", "", err.Error())
		return fmt.Errorf("JMAP discovery failed: %w", err)
	}

	accountId, ok := session.GetPrimaryMailAccountId()
	if !ok {
		return fmt.Errorf("no primary mail account found")
	}

	emails, err := client.QueryEmailsByFilter(ctx, config.MessageID, config.Subject, uint32(config.Count))
	if err != nil {
		logger.LogError(slogLogger, "Search failed", "error", err)
		writeRow("", "", "", err.Error())
		return fmt.Errorf("failed to search messages: %w", err)
	}

	if len(emails) == 0 {
		fmt.Println("No matching messages found.")
		writeRow("", "", "", "no matches found")
		return nil
	}

	exportDir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}
	fmt.Printf("Export directory: %s\n\n", exportDir)

	successCount := 0
	for _, email := range emails {
		msgId := ""
		if len(email.MessageId) > 0 {
			msgId = email.MessageId[0]
		}

		blobName := buildEMLFilename(email, exportDir)
		emlData, dlErr := client.DownloadBlob(ctx, accountId, email.BlobId, filepath.Base(blobName))
		if dlErr != nil {
			logger.LogError(slogLogger, "Failed to download blob", "email_id", email.Id, "error", dlErr)
			writeRow(msgId, email.Subject, "", dlErr.Error())
			continue
		}

		if writeErr := os.WriteFile(blobName, emlData, 0600); writeErr != nil {
			logger.LogError(slogLogger, "Failed to write .eml file", "path", blobName, "error", writeErr)
			writeRow(msgId, email.Subject, "", writeErr.Error())
			continue
		}

		fmt.Printf("  Exported: %s\n", blobName)
		writeRow(msgId, email.Subject, blobName, "")
		successCount++
	}

	fmt.Printf("\n✓ Exported %d/%d messages\n", successCount, len(emails))
	logger.LogInfo(slogLogger, "exportmessages completed",
		"total", len(emails), "success", successCount)
	return nil
}

// buildEMLFilename constructs a safe .eml file path under exportDir.
func buildEMLFilename(email protocol.Email, exportDir string) string {
	name := email.Subject
	if name == "" {
		name = string(email.Id)
	}
	name = export.SanitizeFilename(strings.TrimSpace(name))
	if len(name) > 80 {
		name = name[:80]
	}
	return filepath.Join(exportDir, name+"_"+string(email.Id)+".eml")
}
