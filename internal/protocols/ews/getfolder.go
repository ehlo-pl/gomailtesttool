package ews

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// getFolder calls GetFolder(Inbox) and displays folder properties.
func getFolder(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Getting EWS Inbox folder from https://%s:%d%s...\n\n", config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Folder_Name", "Total_Count", "Unread_Count", "Folder_ID",
			"Response_Time_ms", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeFolderCSV(csvLogger, slogLogger, config, "", 0, 0, "", 0, err.Error())
		return err
	}

	logger.LogDebug(slogLogger, "Sending GetFolder(inbox)")

	start := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, getFolderSOAPBody)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("✗ GetFolder failed: %s\n", err)
		writeFolderCSV(csvLogger, slogLogger, config, "", 0, 0, "", elapsed, err.Error())
		logger.LogError(slogLogger, "getfolder failed", "error", err)
		return err
	}

	var envelope getFolderResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeFolderCSV(csvLogger, slogLogger, config, "", 0, 0, "", elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.GetFolderResponse.ResponseMessages.GetFolderResponseMessage
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeFolderCSV(csvLogger, slogLogger, config, "", 0, 0, "", elapsed, errMsg)
		return errors.New(errMsg)
	}

	folder := msg.Folders.Folder
	fmt.Println("Inbox Folder:")
	fmt.Printf("  Display Name:  %s\n", folder.DisplayName)
	fmt.Printf("  Total Count:   %d\n", folder.TotalCount)
	fmt.Printf("  Unread Count:  %d\n", folder.UnreadCount)
	fmt.Printf("  Folder ID:     %s\n", folder.FolderID.ID)
	fmt.Printf("\n  Response time: %d ms\n\n", elapsed)
	fmt.Println("✓ GetFolder completed successfully")

	logger.LogInfo(slogLogger, "getfolder completed",
		"display_name", folder.DisplayName,
		"total", folder.TotalCount,
		"unread", folder.UnreadCount,
		"elapsed_ms", elapsed)

	writeFolderCSV(csvLogger, slogLogger, config,
		folder.DisplayName, folder.TotalCount, folder.UnreadCount,
		folder.FolderID.ID, elapsed, "")
	return nil
}

func writeFolderCSV(
	csvLogger logger.Logger, slogLogger *slog.Logger,
	config *Config, displayName string, total, unread int,
	folderID string, elapsed int64, errStr string,
) {
	status := "SUCCESS"
	if errStr != "" {
		status = "FAILURE"
	}
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host,
		fmt.Sprintf("%d", config.Port), config.Username,
		displayName,
		fmt.Sprintf("%d", total),
		fmt.Sprintf("%d", unread),
		folderID,
		fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
