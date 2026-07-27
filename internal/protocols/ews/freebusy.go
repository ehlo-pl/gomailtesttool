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

// Normalized free/busy status values returned by mapFreeBusyStatus.
const (
	FreeBusyStatusFree             = "Free"
	FreeBusyStatusTentative        = "Tentative"
	FreeBusyStatusBusy             = "Busy"
	FreeBusyStatusOOF              = "OOF"
	FreeBusyStatusWorkingElsewhere = "WorkingElsewhere"
	FreeBusyStatusNoData           = "NoData"
)

// freeBusySlot is a single normalized free/busy event from the EWS response.
type freeBusySlot struct {
	StartUTC time.Time
	EndUTC   time.Time
	Status   string // one of the FreeBusyStatus* constants
}

// freeBusyResult holds the parsed result for one mailbox.
type freeBusyResult struct {
	Mailbox    string
	Slots      []freeBusySlot
	EventCount int
	Success    bool
	ErrMsg     string
}

// mapFreeBusyStatus maps an EWS BusyType string to a normalized status constant.
func mapFreeBusyStatus(busyType string) string {
	switch strings.ToLower(busyType) {
	case "free":
		return FreeBusyStatusFree
	case "tentative":
		return FreeBusyStatusTentative
	case "busy":
		return FreeBusyStatusBusy
	case "oof":
		return FreeBusyStatusOOF
	case "workingelsewhere":
		return FreeBusyStatusWorkingElsewhere
	default:
		return FreeBusyStatusNoData
	}
}

// buildMailboxDataXML builds the <t:MailboxData> XML blocks for the given mailboxes.
func buildMailboxDataXML(mailboxes []string) string {
	if len(mailboxes) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, mb := range mailboxes {
		fmt.Fprintf(&b, "        <t:MailboxData>\n")
		fmt.Fprintf(&b, "          <t:Email>\n")
		fmt.Fprintf(&b, "            <t:Address>%s</t:Address>\n", xmlEscape(mb))
		fmt.Fprintf(&b, "          </t:Email>\n")
		fmt.Fprintf(&b, "          <t:AttendeeType>Required</t:AttendeeType>\n")
		fmt.Fprintf(&b, "          <t:ExcludeConflicts>false</t:ExcludeConflicts>\n")
		fmt.Fprintf(&b, "        </t:MailboxData>\n")
	}
	return b.String()
}

// parseFreeBusyResponses parses the EWS GetUserAvailability response into a
// slice of freeBusyResult, one per mailbox in the same order as the request.
func parseFreeBusyResponses(respBytes []byte, mailboxes []string, loc *time.Location, slogLogger *slog.Logger) ([]freeBusyResult, error) {
	var envelope getAvailabilityResponseEnvelope
	if err := xml.Unmarshal(respBytes, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse EWS response: %w", err)
	}

	responses := envelope.Body.Response.Responses
	if len(responses) == 0 {
		return nil, errors.New("EWS returned no FreeBusyResponse")
	}

	results := make([]freeBusyResult, 0, len(responses))
	for i, resp := range responses {
		mailbox := ""
		if i < len(mailboxes) {
			mailbox = mailboxes[i]
		}

		r := freeBusyResult{Mailbox: mailbox}

		if resp.ResponseMessage.ResponseClass != "Success" {
			r.ErrMsg = fmt.Sprintf("EWS error %s: %s",
				resp.ResponseMessage.ResponseCode,
				resp.ResponseMessage.MessageText)
			r.Success = false
			results = append(results, r)
			continue
		}

		r.Success = true
		for _, event := range resp.FreeBusyView.CalendarEvents {
			start, err1 := parseFlexibleTime(event.StartTime)
			end, err2 := parseFlexibleTime(event.EndTime)
			if err1 != nil || err2 != nil {
				logger.LogDebug(slogLogger, "Skipping calendar event with unparsable time",
					"start", event.StartTime, "end", event.EndTime)
				continue
			}
			r.Slots = append(r.Slots, freeBusySlot{
				StartUTC: start.UTC(),
				EndUTC:   end.UTC(),
				Status:   mapFreeBusyStatus(event.BusyType),
			})
		}
		r.EventCount = len(r.Slots)
		results = append(results, r)
	}
	return results, nil
}

// freeBusy executes the EWS Free/Busy test for the configured mailbox(es).
// It sends a GetUserAvailability SOAP request and reports a deterministic
// PASS/FAIL outcome with diagnostics.
func freeBusy(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return fmt.Errorf("invalid timezone %q: %w", config.Timezone, err)
	}

	start, err := parseFlexibleTime(config.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}
	end, err := parseFlexibleTime(config.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}

	// Collect mailboxes: primary (--mailbox) plus optional extra (--to).
	mailboxes := []string{config.Mailbox}
	mailboxes = append(mailboxes, config.To...)

	fmt.Printf("Testing EWS Free/Busy for %d mailbox(es) via https://%s:%d%s\n",
		len(mailboxes), config.Host, config.Port, config.EWSPath)
	fmt.Printf("  Time window: %s — %s (UTC)\n", ewsDateTime(start), ewsDateTime(end))
	fmt.Printf("  Timezone:    %s\n", config.Timezone)
	fmt.Printf("  Interval:    %d minutes\n\n", config.Interval)

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Username",
			"Mailbox", "Start", "End", "Timezone", "Interval_Minutes",
			"Event_Count", "Response_Time_ms", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	writeRow := func(mailbox, status string, eventCount int, elapsed int64, errStr string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, status, config.Host,
			fmt.Sprintf("%d", config.Port), config.Username,
			mailbox, start.In(loc).Format(time.RFC3339), end.In(loc).Format(time.RFC3339),
			config.Timezone, fmt.Sprintf("%d", config.Interval),
			fmt.Sprintf("%d", eventCount),
			fmt.Sprintf("%d", elapsed), errStr,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		for _, mb := range mailboxes {
			writeRow(mb, "FAILURE", 0, 0, err.Error())
		}
		return err
	}

	mailboxDataXML := buildMailboxDataXML(mailboxes)
	soapBody := fmt.Sprintf(getFreeBusySOAPBodyFmt,
		mailboxDataXML, ewsDateTime(start), ewsDateTime(end), config.Interval)

	reqStart := time.Now()
	respBytes, err := ewsClient.SendSOAPAction(ctx, soapBody, soapActionGetUserAvailability)
	elapsed := time.Since(reqStart).Milliseconds()

	if err != nil {
		fmt.Printf("✗ GetUserAvailability failed: %s\n", err)
		for _, mb := range mailboxes {
			writeRow(mb, "FAILURE", 0, elapsed, err.Error())
		}
		logger.LogError(slogLogger, "freebusy failed", "error", err)
		return err
	}

	results, parseErr := parseFreeBusyResponses(respBytes, mailboxes, loc, slogLogger)
	if parseErr != nil {
		fmt.Printf("✗ %s\n", parseErr)
		for _, mb := range mailboxes {
			writeRow(mb, "FAILURE", 0, elapsed, parseErr.Error())
		}
		return parseErr
	}

	anyFailure := false
	for _, r := range results {
		if !r.Success {
			anyFailure = true
			fmt.Printf("✗ FAIL  %s — %s\n", r.Mailbox, r.ErrMsg)
			writeRow(r.Mailbox, "FAILURE", 0, elapsed, r.ErrMsg)
			logger.LogError(slogLogger, "freebusy mailbox failed",
				"mailbox", r.Mailbox, "error", r.ErrMsg)
			continue
		}

		fmt.Printf("✓ PASS  %s (%d event(s), response time: %d ms)\n",
			r.Mailbox, r.EventCount, elapsed)
		printFreeBusySlots(r.Slots, loc)
		writeRow(r.Mailbox, "SUCCESS", r.EventCount, elapsed, "")
		logger.LogInfo(slogLogger, "freebusy mailbox ok",
			"mailbox", r.Mailbox, "events", r.EventCount)
	}

	if anyFailure {
		return errors.New("freebusy test failed for one or more mailboxes")
	}
	return nil
}

// printFreeBusySlots prints a summary table of free/busy slots to stdout.
func printFreeBusySlots(slots []freeBusySlot, loc *time.Location) {
	if len(slots) == 0 {
		fmt.Println("  No calendar events in the requested window.")
		return
	}

	// Count statuses.
	counts := make(map[string]int)
	for _, s := range slots {
		counts[s.Status]++
	}

	fmt.Printf("  Events: %d total", len(slots))
	for _, status := range []string{FreeBusyStatusBusy, FreeBusyStatusTentative, FreeBusyStatusOOF, FreeBusyStatusWorkingElsewhere, FreeBusyStatusFree} {
		if n := counts[status]; n > 0 {
			fmt.Printf("  %s:%d", status, n)
		}
	}
	fmt.Println()

	// Print up to 5 events in the given timezone.
	limit := len(slots)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		s := slots[i]
		fmt.Printf("  %s  %s — %s\n",
			s.Status,
			s.StartUTC.In(loc).Format("2006-01-02 15:04 MST"),
			s.EndUTC.In(loc).Format("15:04 MST"),
		)
	}
	if len(slots) > 5 {
		fmt.Printf("  ... and %d more event(s)\n", len(slots)-5)
	}
}
