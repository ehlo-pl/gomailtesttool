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
	"github.com/ehlo-pl/gomailtesttool/internal/common/timeslot"
)

// findTimeSlot searches for free meeting slots of the configured duration in
// the recipient's calendar. It fetches the recipient's busy blocks via
// GetUserAvailability (detailed FreeBusy view) and computes free slots
// client-side (working hours 08:00-17:00 UTC, Monday-Friday).
func findTimeSlot(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	windowStart := time.Now().UTC()
	if config.StartTime != "" {
		t, err := parseFlexibleTime(config.StartTime)
		if err != nil {
			return fmt.Errorf("invalid start time: %w", err)
		}
		windowStart = t.UTC()
	}
	windowEnd := timeslot.AddWorkingDays(windowStart, 5)
	if config.EndTime != "" {
		t, err := parseFlexibleTime(config.EndTime)
		if err != nil {
			return fmt.Errorf("invalid end time: %w", err)
		}
		windowEnd = t.UTC()
	}
	if !windowEnd.After(windowStart) {
		return fmt.Errorf("end time must be after start time")
	}
	recipient := config.To[0]

	fmt.Printf("Searching %d-minute slots for %s via https://%s:%d%s (%s to %s)...\n\n",
		config.Duration, recipient, config.Host, config.Port, config.EWSPath,
		ewsDateTime(windowStart), ewsDateTime(windowEnd))

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		_ = csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Recipient", "Duration_Minutes", "Slot_Start", "Slot_End", "Response_Time_ms", "Error",
		})
	}

	writeRow := func(status, slotStart, slotEnd string, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			recipient, fmt.Sprintf("%d", config.Duration), slotStart, slotEnd,
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

	soapBody := fmt.Sprintf(getUserAvailabilityDetailedSOAPBodyFmt,
		xmlEscape(recipient), ewsDateTime(windowStart), ewsDateTime(windowEnd))

	reqStart := time.Now()
	respBytes, err := ewsClient.SendSOAPAction(ctx, soapBody, soapActionGetUserAvailability)
	elapsed := time.Since(reqStart).Milliseconds()

	if err != nil {
		fmt.Printf("✗ GetUserAvailability failed: %s\n", err)
		writeRow("FAILURE", "", "", elapsed, err.Error())
		logger.LogError(slogLogger, "findtimeslot failed", "error", err)
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

	// Collect busy intervals (everything not explicitly free).
	var busy []timeslot.Interval
	for _, event := range fb.FreeBusyView.CalendarEvents {
		if strings.EqualFold(event.BusyType, "Free") {
			continue
		}
		start, err1 := parseFlexibleTime(event.StartTime)
		end, err2 := parseFlexibleTime(event.EndTime)
		if err1 != nil || err2 != nil {
			logger.LogDebug(slogLogger, "Skipping calendar event with unparsable time",
				"start", event.StartTime, "end", event.EndTime)
			continue
		}
		busy = append(busy, timeslot.Interval{Start: start.UTC(), End: end.UTC()})
	}

	slots := timeslot.FindFreeSlots(busy, windowStart, windowEnd, timeslot.Options{
		Duration:     time.Duration(config.Duration) * time.Minute,
		Count:        config.Count,
		WorkDayStart: 8,
		WorkDayEnd:   17,
		WorkdaysOnly: true,
	})

	fmt.Printf("✓ Availability retrieved (response time: %d ms)\n", elapsed)
	fmt.Printf("  Recipient:     %s\n", recipient)
	fmt.Printf("  Busy blocks:   %d\n", len(busy))
	fmt.Println("  Working hours: 08:00-17:00 UTC, Monday-Friday")
	fmt.Println()

	if len(slots) == 0 {
		fmt.Println("No free slots found in the window.")
		writeRow("SUCCESS", "No free slots found", "", elapsed, "")
		return nil
	}

	fmt.Printf("Free %d-minute slots:\n", config.Duration)
	for i, slot := range slots {
		fmt.Printf("%d. %s — %s\n", i+1, slot.Start.Format("Mon 2006-01-02 15:04"), slot.End.Format("15:04 MST"))
		writeRow("SUCCESS", slot.Start.Format(time.RFC3339), slot.End.Format(time.RFC3339), elapsed, "")
	}

	logger.LogInfo(slogLogger, "findtimeslot completed", "recipient", recipient, "slots", len(slots))
	return nil
}
