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

// EWS response types for FindItem with CalendarView.

type findCalendarResponseEnvelope struct {
	XMLName struct{}             `xml:"Envelope"`
	Body    findCalendarRespBody `xml:"Body"`
}

type findCalendarRespBody struct {
	Response findCalendarResponse `xml:"FindItemResponse"`
}

type findCalendarResponse struct {
	ResponseMessages findCalendarRespMessages `xml:"ResponseMessages"`
}

type findCalendarRespMessages struct {
	Message findCalendarRespMessage `xml:"FindItemResponseMessage"`
}

type findCalendarRespMessage struct {
	ResponseClass string           `xml:"ResponseClass,attr"`
	ResponseCode  string           `xml:"ResponseCode"`
	MessageText   string           `xml:"MessageText"`
	RootFolder    findCalendarRoot `xml:"RootFolder"`
}

type findCalendarRoot struct {
	TotalItemsInView int               `xml:"TotalItemsInView,attr"`
	Items            []ewsCalendarItem `xml:"Items>CalendarItem"`
}

type ewsCalendarItem struct {
	ItemId    ewsItemId    `xml:"ItemId"`
	Subject   string       `xml:"Subject"`
	Start     string       `xml:"Start"`
	End       string       `xml:"End"`
	Organizer ewsFromField `xml:"Organizer"`
}

// getEvents lists calendar events in the configured time window (default: the
// next 7 days) using FindItem with a CalendarView.
func getEvents(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	start, end, err := resolveEventWindow(config, 7*24*time.Hour)
	if err != nil {
		return err
	}

	fmt.Printf("Listing calendar events from https://%s:%d%s (%s to %s)...\n\n",
		config.Host, config.Port, config.EWSPath, ewsDateTime(start), ewsDateTime(end))

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Subject", "Organizer", "Start", "End", "Response_Time_ms", "Error",
		})
	}

	writeRow := func(status, subject, organizer, evStart, evEnd string, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			subject, organizer, evStart, evEnd, fmt.Sprintf("%d", elapsed), errStr,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeRow("FAILURE", "", "", "", "", 0, err.Error())
		return err
	}

	body := fmt.Sprintf(findItemCalendarSOAPBodyFmt, config.Count, ewsDateTime(start), ewsDateTime(end))

	reqStart := time.Now()
	respBytes, err := ewsClient.SendSOAP(ctx, body)
	elapsed := time.Since(reqStart).Milliseconds()

	if err != nil {
		fmt.Printf("✗ FindItem (CalendarView) failed: %s\n", err)
		writeRow("FAILURE", "", "", "", "", elapsed, err.Error())
		logger.LogError(slogLogger, "getevents failed", "error", err)
		return err
	}

	var envelope findCalendarResponseEnvelope
	if xmlErr := xml.Unmarshal(respBytes, &envelope); xmlErr != nil {
		fmt.Printf("✗ Failed to parse EWS response: %s\n", xmlErr)
		writeRow("FAILURE", "", "", "", "", elapsed, xmlErr.Error())
		return xmlErr
	}

	msg := envelope.Body.Response.ResponseMessages.Message
	if msg.ResponseClass != "Success" {
		errMsg := fmt.Sprintf("EWS error: %s — %s", msg.ResponseCode, msg.MessageText)
		fmt.Printf("✗ %s\n", errMsg)
		writeRow("FAILURE", "", "", "", "", elapsed, errMsg)
		return errors.New(errMsg)
	}

	items := msg.RootFolder.Items
	fmt.Printf("Calendar events (%d):  (response time: %d ms)\n\n", len(items), elapsed)

	if len(items) == 0 {
		fmt.Println("No events found in the given time window.")
		writeRow("SUCCESS", "No events found", "", "", "", elapsed, "")
		return nil
	}

	for i, item := range items {
		organizer := item.Organizer.Mailbox.EmailAddress
		if item.Organizer.Mailbox.Name != "" {
			organizer = fmt.Sprintf("%s <%s>", item.Organizer.Mailbox.Name, item.Organizer.Mailbox.EmailAddress)
		}
		fmt.Printf("%d. %s\n   Organizer: %s\n   Start: %s\n   End:   %s\n\n", i+1, item.Subject, organizer, item.Start, item.End)
		writeRow("SUCCESS", item.Subject, organizer, item.Start, item.End, elapsed, "")
	}

	logger.LogInfo(slogLogger, "getevents completed", "count", len(items))
	return nil
}

// resolveEventWindow returns the [start, end) window from config.StartTime and
// config.EndTime, defaulting to [now, now+defaultSpan).
func resolveEventWindow(config *Config, defaultSpan time.Duration) (time.Time, time.Time, error) {
	start := time.Now().UTC()
	if config.StartTime != "" {
		t, err := parseFlexibleTime(config.StartTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start time: %w", err)
		}
		start = t
	}

	end := start.Add(defaultSpan)
	if config.EndTime != "" {
		t, err := parseFlexibleTime(config.EndTime)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end time: %w", err)
		}
		end = t
	}

	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end time must be after start time")
	}
	return start, end, nil
}
