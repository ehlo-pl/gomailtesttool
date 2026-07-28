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

// meetingResponseElement maps a response type string to the EWS CreateItem element name.
func meetingResponseElement(response string) (string, error) {
	switch strings.ToLower(response) {
	case "accept":
		return "AcceptItem", nil
	case "decline":
		return "DeclineItem", nil
	case "tentative":
		return "TentativelyAcceptItem", nil
	default:
		return "", fmt.Errorf("invalid response type %q (must be accept, decline, or tentative)", response)
	}
}

// respondMeeting sends a meeting response (accept/decline/tentative) for the
// configured meeting request item via EWS CreateItem.
func respondMeeting(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	elementName, err := meetingResponseElement(config.MeetingResponse)
	if err != nil {
		return err
	}

	disposition := "SendAndSaveCopy"
	if !config.SendMeetingResponse {
		disposition = "SaveOnly"
	}

	refItem := fmt.Sprintf(`<t:ReferenceItemId Id="%s"/>`, xmlEscape(config.ItemID))
	if config.ChangeKey != "" {
		refItem = fmt.Sprintf(`<t:ReferenceItemId Id="%s" ChangeKey="%s"/>`, xmlEscape(config.ItemID), xmlEscape(config.ChangeKey))
	}

	bodyXML := ""
	if config.Comment != "" {
		bodyXML = fmt.Sprintf("\n          <t:Body BodyType=\"Text\">%s</t:Body>", xmlEscape(config.Comment))
	}

	soapBody := fmt.Sprintf(respondMeetingSOAPBodyFmt,
		disposition,
		elementName,
		refItem,
		bodyXML,
		elementName,
	)

	fmt.Printf("Sending meeting response '%s' for item %s via EWS at https://%s:%d%s...\n\n",
		config.MeetingResponse, config.ItemID, config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"ItemID", "Response", "SendResponse", "Response_Time_ms", "Error",
		})
	}

	writeRow := func(status string, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			config.ItemID, config.MeetingResponse,
			fmt.Sprintf("%v", config.SendMeetingResponse),
			fmt.Sprintf("%d", elapsed), errStr,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeRow("FAILURE", 0, err.Error())
		return err
	}

	reqStart := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, soapBody)
	elapsed := time.Since(reqStart).Milliseconds()

	if err != nil {
		fmt.Printf("✗ CreateItem (%s) failed: %s\n", elementName, err)
		writeRow("FAILURE", elapsed, err.Error())
		logger.LogError(slogLogger, "respondmeeting failed", "error", err)
		return err
	}

	var envelope createItemResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeRow("FAILURE", elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.Response.ResponseMessages.Message
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeRow("FAILURE", elapsed, errMsg)
		return errors.New(errMsg)
	}

	fmt.Printf("✓ Meeting response '%s' sent successfully (response time: %d ms)\n",
		config.MeetingResponse, elapsed)
	fmt.Printf("  Item ID: %s\n", config.ItemID)
	if config.Comment != "" {
		fmt.Printf("  Comment: %s\n", config.Comment)
	}

	writeRow("SUCCESS", elapsed, "")
	logger.LogInfo(slogLogger, "respondmeeting completed",
		"item_id", config.ItemID,
		"response", config.MeetingResponse,
		"elapsed_ms", elapsed)
	return nil
}
