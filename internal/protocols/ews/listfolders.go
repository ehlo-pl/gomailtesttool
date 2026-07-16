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

// EWS response types for FindFolder.

type findFolderResponseEnvelope struct {
	XMLName struct{}              `xml:"Envelope"`
	Body    findFolderRespBody    `xml:"Body"`
}

type findFolderRespBody struct {
	Response findFolderResponse `xml:"FindFolderResponse"`
}

type findFolderResponse struct {
	ResponseMessages findFolderRespMessages `xml:"ResponseMessages"`
}

type findFolderRespMessages struct {
	Message findFolderRespMessage `xml:"FindFolderResponseMessage"`
}

type findFolderRespMessage struct {
	ResponseClass string         `xml:"ResponseClass,attr"`
	ResponseCode  string         `xml:"ResponseCode"`
	MessageText   string         `xml:"MessageText"`
	RootFolder    findFolderRoot `xml:"RootFolder"`
}

type findFolderRoot struct {
	Folders []ewsFolder `xml:"Folders>Folder"`
}

// listFolders calls FindFolder on the root and displays all top-level folders.
func listFolders(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing EWS folders from https://%s:%d%s...\n\n", config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Folder_Name", "Total_Count", "Unread_Count", "Folder_ID",
			"Response_Time_ms", "Error",
		})
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeListFoldersCSV(csvLogger, slogLogger, config, "", 0, 0, "", 0, err.Error())
		return err
	}

	start := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, findFolderSOAPBody)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("✗ FindFolder failed: %s\n", err)
		writeListFoldersCSV(csvLogger, slogLogger, config, "", 0, 0, "", elapsed, err.Error())
		logger.LogError(slogLogger, "listfolders failed", "error", err)
		return err
	}

	var envelope findFolderResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeListFoldersCSV(csvLogger, slogLogger, config, "", 0, 0, "", elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.Response.ResponseMessages.Message
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeListFoldersCSV(csvLogger, slogLogger, config, "", 0, 0, "", elapsed, errMsg)
		return errors.New(errMsg)
	}

	folders := msg.RootFolder.Folders
	fmt.Printf("Folders (%d):  (response time: %d ms)\n\n", len(folders), elapsed)
	fmt.Println("  Name                              Total   Unread")
	fmt.Println("  ----                              -----   ------")
	for _, f := range folders {
		fmt.Printf("  %-34s %5d   %5d\n", f.DisplayName, f.TotalCount, f.UnreadCount)
		writeListFoldersCSV(csvLogger, slogLogger, config, f.DisplayName, f.TotalCount, f.UnreadCount, f.FolderID.ID, elapsed, "")
	}

	logger.LogInfo(slogLogger, "listfolders completed", "folder_count", len(folders))
	fmt.Printf("\n✓ Listed %d folders\n", len(folders))
	return nil
}

func writeListFoldersCSV(
	csvLogger logger.Logger, slogLogger *slog.Logger,
	config *Config, name string, total, unread int, folderID string, elapsed int64, errStr string,
) {
	status := "SUCCESS"
	if errStr != "" {
		status = "FAILURE"
	}
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host,
		fmt.Sprintf("%d", config.Port), config.Username,
		name, fmt.Sprintf("%d", total), fmt.Sprintf("%d", unread),
		folderID, fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
