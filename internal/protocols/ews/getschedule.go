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

// soapActionGetUserAvailability is required by some Exchange versions for the
// GetUserAvailability operation (the usual empty SOAPAction is rejected).
const soapActionGetUserAvailability = `"http://schemas.microsoft.com/exchange/services/2006/messages/GetUserAvailability"`

// EWS response types for GetUserAvailability.

type getAvailabilityResponseEnvelope struct {
	XMLName struct{}                `xml:"Envelope"`
	Body    getAvailabilityRespBody `xml:"Body"`
}

type getAvailabilityRespBody struct {
	Response getAvailabilityResponse `xml:"GetUserAvailabilityResponse"`
}

type getAvailabilityResponse struct {
	Responses []freeBusyResponse `xml:"FreeBusyResponseArray>FreeBusyResponse"`
}

type freeBusyResponse struct {
	ResponseMessage freeBusyResponseMessage `xml:"ResponseMessage"`
	FreeBusyView    freeBusyView            `xml:"FreeBusyView"`
}

type freeBusyResponseMessage struct {
	ResponseClass string `xml:"ResponseClass,attr"`
	ResponseCode  string `xml:"ResponseCode"`
	MessageText   string `xml:"MessageText"`
}

type freeBusyView struct {
	MergedFreeBusy string               `xml:"MergedFreeBusy"`
	CalendarEvents []ewsCalendarFBEvent `xml:"CalendarEventArray>CalendarEvent"`
}

// ewsCalendarFBEvent is a single busy block in the detailed FreeBusy view.
type ewsCalendarFBEvent struct {
	StartTime string `xml:"StartTime"`
	EndTime   string `xml:"EndTime"`
	BusyType  string `xml:"BusyType"`
}

// getSchedule checks a recipient's merged free/busy view over the configured
// time window (default: the next 24 hours) via GetUserAvailability.
func getSchedule(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	start, end, err := resolveEventWindow(config, 24*time.Hour)
	if err != nil {
		return err
	}
	recipient := config.To[0]

	fmt.Printf("Checking availability of %s via https://%s:%d%s (%s to %s)...\n\n",
		recipient, config.Host, config.Port, config.EWSPath, ewsDateTime(start), ewsDateTime(end))

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Recipient", "Start", "End", "MergedFreeBusy", "First_Slot", "Response_Time_ms", "Error",
		})
	}

	writeRow := func(status, merged, firstSlot string, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			recipient, ewsDateTime(start), ewsDateTime(end), merged, firstSlot,
			fmt.Sprintf("%d", elapsed), errStr,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeRow("FAILURE", "", "", 0, err.Error())
		return err
	}

	soapBody := fmt.Sprintf(getUserAvailabilitySOAPBodyFmt,
		xmlEscape(recipient), ewsDateTime(start), ewsDateTime(end))

	reqStart := time.Now()
	respBytes, err := ewsClient.SendSOAPAction(ctx, soapBody, soapActionGetUserAvailability)
	elapsed := time.Since(reqStart).Milliseconds()

	if err != nil {
		fmt.Printf("✗ GetUserAvailability failed: %s\n", err)
		writeRow("FAILURE", "", "", elapsed, err.Error())
		logger.LogError(slogLogger, "getschedule failed", "error", err)
		return err
	}

	var envelope getAvailabilityResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeRow("FAILURE", "", "", elapsed, xmlErr.Error())
		return xmlErr
	}

	responses := envelope.Body.Response.Responses
	if len(responses) == 0 {
		msg := "EWS returned no FreeBusyResponse"
		fmt.Printf("✗ %s\n", msg)
		writeRow("FAILURE", "", "", elapsed, msg)
		return errors.New(msg)
	}

	fb := responses[0]
	if fb.ResponseMessage.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", fb.ResponseMessage.ResponseCode, fb.ResponseMessage.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeRow("FAILURE", "", "", elapsed, errMsg)
		return errors.New(errMsg)
	}

	merged := fb.FreeBusyView.MergedFreeBusy
	firstSlot := interpretAvailability(merged)

	fmt.Printf("✓ Availability retrieved (response time: %d ms)\n", elapsed)
	fmt.Printf("  Recipient:       %s\n", recipient)
	fmt.Printf("  Merged view:     %s (one digit per hour)\n", merged)
	fmt.Printf("  First slot:      %s\n", firstSlot)
	fmt.Println("  Legend:          0=Free 1=Tentative 2=Busy 3=Out of Office 4=Working Elsewhere")

	writeRow("SUCCESS", merged, firstSlot, elapsed, "")
	logger.LogInfo(slogLogger, "getschedule completed", "recipient", recipient, "merged", merged)
	return nil
}

// interpretAvailability maps the first digit of a merged free/busy view to a
// human-readable status.
func interpretAvailability(view string) string {
	if len(view) == 0 {
		return "Unknown (empty response)"
	}
	switch string(view[0]) {
	case "0":
		return "Free"
	case "1":
		return "Tentative"
	case "2":
		return "Busy"
	case "3":
		return "Out of Office"
	case "4":
		return "Working Elsewhere"
	default:
		return fmt.Sprintf("Unknown (%s)", string(view[0]))
	}
}
