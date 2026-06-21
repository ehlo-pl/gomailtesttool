package ews

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// getFolderResponseEnvelope is used to parse the EWS GetFolder SOAP response.
type getFolderResponseEnvelope struct {
	XMLName struct{}              `xml:"Envelope"`
	Body    getFolderResponseBody `xml:"Body"`
}

type getFolderResponseBody struct {
	GetFolderResponse getFolderResponse `xml:"GetFolderResponse"`
}

type getFolderResponse struct {
	ResponseMessages getFolderResponseMessages `xml:"ResponseMessages"`
}

type getFolderResponseMessages struct {
	GetFolderResponseMessage getFolderResponseMessage `xml:"GetFolderResponseMessage"`
}

type getFolderResponseMessage struct {
	ResponseClass string        `xml:"ResponseClass,attr"`
	ResponseCode  string        `xml:"ResponseCode"`
	MessageText   string        `xml:"MessageText"`
	Folders       getFolderList `xml:"Folders"`
}

type getFolderList struct {
	Folder ewsFolder `xml:"Folder"`
}

type ewsFolder struct {
	FolderID    ewsFolderID `xml:"FolderId"`
	DisplayName string      `xml:"DisplayName"`
	TotalCount  int         `xml:"TotalCount"`
	UnreadCount int         `xml:"UnreadCount"`
}

type ewsFolderID struct {
	ID        string `xml:"Id,attr"`
	ChangeKey string `xml:"ChangeKey,attr"`
}

// testAuth authenticates to EWS and verifies credentials by performing GetFolder(Inbox).
func testAuth(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Testing EWS authentication to https://%s:%d%s...\n", config.Host, config.Port, config.EWSPath)
	fmt.Printf("  Auth method: %s\n", strings.ToUpper(config.AuthMethod))
	fmt.Printf("  Username:    %s\n\n", config.Username)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username", "AuthMethod",
			"Response_Time_ms", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeAuthCSV(csvLogger, slogLogger, config, 0, err.Error())
		return err
	}

	logger.LogDebug(slogLogger, "Sending GetFolder(inbox) to verify credentials")

	start := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, getFolderSOAPBody)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("✗ Authentication failed: %s\n", err)
		writeAuthCSV(csvLogger, slogLogger, config, elapsed, err.Error())
		logger.LogError(slogLogger, "testauth failed", "error", err)
		return err
	}

	// Parse response to confirm success
	var envelope getFolderResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeAuthCSV(csvLogger, slogLogger, config, elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.GetFolderResponse.ResponseMessages.GetFolderResponseMessage
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeAuthCSV(csvLogger, slogLogger, config, elapsed, errMsg)
		return errors.New(errMsg)
	}

	fmt.Printf("✓ Authentication successful (%s)\n", strings.ToUpper(config.AuthMethod))
	fmt.Printf("  Response time: %d ms\n", elapsed)
	folderID := msg.Folders.Folder.FolderID.ID
	if len(folderID) > 16 {
		folderID = folderID[:16] + "..."
	}
	fmt.Printf("  Inbox folder ID: %s\n\n", folderID)

	logger.LogInfo(slogLogger, "testauth completed successfully",
		"auth_method", config.AuthMethod, "elapsed_ms", elapsed)

	writeAuthCSV(csvLogger, slogLogger, config, elapsed, "")
	return nil
}

func writeAuthCSV(csvLogger logger.Logger, slogLogger *slog.Logger, config *Config, elapsed int64, errStr string) {
	status := "SUCCESS"
	if errStr != "" {
		status = "FAILURE"
	}
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host,
		fmt.Sprintf("%d", config.Port),
		config.Username, config.AuthMethod,
		fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
