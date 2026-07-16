package ews

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// EWS response types for GetItem (MIME content).

type getItemResponseEnvelope struct {
	XMLName struct{}          `xml:"Envelope"`
	Body    getItemRespBody   `xml:"Body"`
}

type getItemRespBody struct {
	Response getItemResponse `xml:"GetItemResponse"`
}

type getItemResponse struct {
	ResponseMessages getItemRespMessages `xml:"ResponseMessages"`
}

type getItemRespMessages struct {
	Message getItemRespMessage `xml:"GetItemResponseMessage"`
}

type getItemRespMessage struct {
	ResponseClass string        `xml:"ResponseClass,attr"`
	ResponseCode  string        `xml:"ResponseCode"`
	MessageText   string        `xml:"MessageText"`
	Items         getItemItems  `xml:"Items"`
}

type getItemItems struct {
	Message getItemMessage `xml:"Message"`
}

type getItemMessage struct {
	ItemId      ewsItemId   `xml:"ItemId"`
	Subject     string      `xml:"Subject"`
	MimeContent string      `xml:"MimeContent"`
}

// exportMessages searches by subject and/or Message-ID, downloads MIME content,
// and writes each matching message as a .eml file.
func exportMessages(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Searching and exporting messages from https://%s:%d%s...\n\n", config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"MessageId", "Subject", "FilePath", "Error",
		})
	}

	writeRow := func(msgId, subject, filePath, errStr string) {
		status := "SUCCESS"
		if errStr != "" {
			status = "FAILURE"
		}
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			msgId, subject, filePath, errStr,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeRow("", "", "", err.Error())
		return err
	}

	// Build FindItem SOAP body based on which search criteria are provided.
	var findBody string
	msgID := strings.TrimSpace(config.MessageID)
	subj := strings.TrimSpace(config.Subject)

	switch {
	case msgID != "" && subj != "":
		findBody = fmt.Sprintf(findItemByBothSOAPBodyFmt, config.Count, xmlEscape(msgID), xmlEscape(subj))
	case msgID != "":
		findBody = fmt.Sprintf(findItemByMsgIDSOAPBodyFmt, config.Count, xmlEscape(msgID))
	default:
		findBody = fmt.Sprintf(findItemBySubjectSOAPBodyFmt, config.Count, xmlEscape(subj))
	}

	findRespBytes, err := ewsClient.SendSOAP(ctx, findBody)
	if err != nil {
		fmt.Printf("✗ FindItem failed: %s\n", err)
		writeRow("", "", "", err.Error())
		logger.LogError(slogLogger, "FindItem failed", "error", err)
		return err
	}

	var findEnvelope findItemResponseEnvelope
	if xmlErr := xml.Unmarshal(findRespBytes, &findEnvelope); xmlErr != nil {
		writeRow("", "", "", xmlErr.Error())
		return xmlErr
	}

	findMsg := findEnvelope.Body.Response.ResponseMessages.Message
	if findMsg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS FindItem error: %s — %s", findMsg.ResponseCode, findMsg.MessageText)
		writeRow("", "", "", errMsg)
		return errors.New(errMsg)
	}

	items := findMsg.RootFolder.Items
	if len(items) == 0 {
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
	for _, item := range items {
		// Fetch raw MIME via GetItem.
		getMIMEBody := fmt.Sprintf(getItemMIMESOAPBodyFmt, xmlEscape(item.ItemId.ID))
		start := time.Now()
		getRespBytes, getErr := ewsClient.SendSOAP(ctx, getMIMEBody)
		elapsed := time.Since(start).Milliseconds()
		_ = elapsed

		if getErr != nil {
			logger.LogError(slogLogger, "GetItem failed", "item_id", item.ItemId.ID, "error", getErr)
			writeRow("", item.Subject, "", getErr.Error())
			continue
		}

		var getEnvelope getItemResponseEnvelope
		if xmlErr := xml.Unmarshal(getRespBytes, &getEnvelope); xmlErr != nil {
			writeRow("", item.Subject, "", xmlErr.Error())
			continue
		}

		getMsg := getEnvelope.Body.Response.ResponseMessages.Message
		if getMsg.ResponseClass != "Success" {
			errMsg := fmt.Sprintf("EWS GetItem error: %s — %s", getMsg.ResponseCode, getMsg.MessageText)
			writeRow("", item.Subject, "", errMsg)
			continue
		}

		mimeData, decErr := base64.StdEncoding.DecodeString(getMsg.Items.Message.MimeContent)
		if decErr != nil {
			writeRow("", item.Subject, "", "base64 decode error: "+decErr.Error())
			continue
		}

		subject := getMsg.Items.Message.Subject
		if subject == "" {
			subject = item.Subject
		}
		safeSubject := export.SanitizeFilename(strings.TrimSpace(subject))
		if len(safeSubject) > 80 {
			safeSubject = safeSubject[:80]
		}
		emlPath := filepath.Join(exportDir, safeSubject+"_"+sanitizeID(item.ItemId.ID)+".eml")

		if writeErr := os.WriteFile(emlPath, mimeData, 0600); writeErr != nil {
			writeRow("", subject, "", writeErr.Error())
			continue
		}

		fmt.Printf("  Exported: %s\n", emlPath)
		writeRow("", subject, emlPath, "")
		successCount++
	}

	fmt.Printf("\n✓ Exported %d/%d messages\n", successCount, len(items))
	logger.LogInfo(slogLogger, "exportmessages completed", "total", len(items), "success", successCount)
	return nil
}

// sanitizeID returns the last 16 characters of an EWS item ID for use in filenames.
func sanitizeID(id string) string {
	if len(id) > 16 {
		id = id[len(id)-16:]
	}
	return export.SanitizeFilename(id)
}
