package ews

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// sendInvite creates a calendar meeting via EWS CreateItem with
// SendMeetingInvitations="SendToAllAndSaveCopy", inviting the --to attendees.
func sendInvite(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	start, err := parseFlexibleTime(config.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}
	end, err := parseFlexibleTime(config.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}
	if !end.After(start) {
		return fmt.Errorf("end time must be after start time")
	}

	fmt.Printf("Creating calendar invitation via EWS at https://%s:%d%s...\n\n", config.Host, config.Port, config.EWSPath)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"To", "Subject", "Start", "End", "Response_Time_ms", "Error",
		})
	}

	toStr := strings.Join(config.To, "; ")

	writeRow := func(status string, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			toStr, config.Subject, ewsDateTime(start), ewsDateTime(end),
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

	bodyType := "Text"
	bodyContent := config.Body
	if config.BodyHTML != "" {
		bodyType = "HTML"
		bodyContent = config.BodyHTML
	}

	soapBody := fmt.Sprintf(createCalendarItemSOAPBodyFmt,
		xmlEscape(config.Subject),
		bodyType,
		xmlEscape(bodyContent),
		ewsDateTime(start),
		ewsDateTime(end),
		buildAttendeesXML(config.To),
	)

	reqStart := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, soapBody)
	elapsed := time.Since(reqStart).Milliseconds()

	if err != nil {
		fmt.Printf("✗ CreateItem (CalendarItem) failed: %s\n", err)
		writeRow("FAILURE", elapsed, err.Error())
		logger.LogError(slogLogger, "sendinvite failed", "error", err)
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

	fmt.Printf("✓ Calendar invitation sent successfully (response time: %d ms)\n", elapsed)
	fmt.Printf("  To:      %s\n", toStr)
	fmt.Printf("  Subject: %s\n", config.Subject)
	fmt.Printf("  Start:   %s\n", ewsDateTime(start))
	fmt.Printf("  End:     %s\n", ewsDateTime(end))

	writeRow("SUCCESS", elapsed, "")
	logger.LogInfo(slogLogger, "sendinvite completed", "to", toStr, "subject", config.Subject, "elapsed_ms", elapsed)
	return nil
}

// buildAttendeesXML builds the EWS Attendee XML for a list of email addresses.
func buildAttendeesXML(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, addr := range addrs {
		fmt.Fprintf(&b, `        <t:Attendee><t:Mailbox><t:EmailAddress>%s</t:EmailAddress></t:Mailbox></t:Attendee>`, xmlEscape(addr))
		b.WriteByte('\n')
	}
	return b.String()
}
