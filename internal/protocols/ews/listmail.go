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

// EWS response types for FindItem.

type findItemResponseEnvelope struct {
	XMLName struct{}           `xml:"Envelope"`
	Body    findItemRespBody   `xml:"Body"`
}

type findItemRespBody struct {
	Response findItemResponse `xml:"FindItemResponse"`
}

type findItemResponse struct {
	ResponseMessages findItemRespMessages `xml:"ResponseMessages"`
}

type findItemRespMessages struct {
	Message findItemRespMessage `xml:"FindItemResponseMessage"`
}

type findItemRespMessage struct {
	ResponseClass string         `xml:"ResponseClass,attr"`
	ResponseCode  string         `xml:"ResponseCode"`
	MessageText   string         `xml:"MessageText"`
	RootFolder    findItemRoot   `xml:"RootFolder"`
}

type findItemRoot struct {
	TotalItemsInView int          `xml:"TotalItemsInView,attr"`
	Items            []ewsMessage `xml:"Items>Message"`
}

type ewsMessage struct {
	ItemId           ewsItemId    `xml:"ItemId"`
	Subject          string       `xml:"Subject"`
	DateTimeReceived string       `xml:"DateTimeReceived"`
	From             ewsFromField `xml:"From"`
}

type ewsItemId struct {
	ID        string `xml:"Id,attr"`
	ChangeKey string `xml:"ChangeKey,attr"`
}

type ewsFromField struct {
	Mailbox ewsMailboxField `xml:"Mailbox"`
}

type ewsMailboxField struct {
	Name         string `xml:"Name"`
	EmailAddress string `xml:"EmailAddress"`
}

// listMail retrieves and displays recent messages from the Inbox.
func listMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing inbox messages from https://%s:%d%s...\n\n", config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Subject", "From", "DateTimeReceived", "Response_Time_ms", "Error",
		})
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeListMailCSV(csvLogger, slogLogger, config, "", "", "", 0, err.Error())
		return err
	}

	body := fmt.Sprintf(findItemInboxSOAPBodyFmt, config.Count)

	start := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, body)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("✗ FindItem failed: %s\n", err)
		writeListMailCSV(csvLogger, slogLogger, config, "", "", "", elapsed, err.Error())
		logger.LogError(slogLogger, "listmail failed", "error", err)
		return err
	}

	var envelope findItemResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeListMailCSV(csvLogger, slogLogger, config, "", "", "", elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.Response.ResponseMessages.Message
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeListMailCSV(csvLogger, slogLogger, config, "", "", "", elapsed, errMsg)
		return errors.New(errMsg)
	}

	items := msg.RootFolder.Items
	fmt.Printf("Inbox messages (%d of %d):  (response time: %d ms)\n\n",
		len(items), msg.RootFolder.TotalItemsInView, elapsed)

	if len(items) == 0 {
		fmt.Println("No messages found.")
		writeListMailCSV(csvLogger, slogLogger, config, "No messages found", "", "", elapsed, "")
		return nil
	}

	for i, item := range items {
		from := item.From.Mailbox.EmailAddress
		if item.From.Mailbox.Name != "" {
			from = fmt.Sprintf("%s <%s>", item.From.Mailbox.Name, item.From.Mailbox.EmailAddress)
		}
		fmt.Printf("%d. %s\n   From: %s\n   Date: %s\n\n", i+1, item.Subject, from, item.DateTimeReceived)
		writeListMailCSV(csvLogger, slogLogger, config, item.Subject, from, item.DateTimeReceived, elapsed, "")
	}

	logger.LogInfo(slogLogger, "listmail completed", "count", len(items))
	return nil
}

func writeListMailCSV(
	csvLogger logger.Logger, slogLogger *slog.Logger,
	config *Config, subject, from, receivedDate string, elapsed int64, errStr string,
) {
	status := "SUCCESS"
	if errStr != "" {
		status = "FAILURE"
	}
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host,
		fmt.Sprintf("%d", config.Port), config.Username,
		subject, from, receivedDate, fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
