package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	calendarapi "google.golang.org/api/calendar/v3"
	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	mimebuilder "github.com/ehlo-pl/gomailtesttool/internal/common/mime"
	"github.com/ehlo-pl/gomailtesttool/internal/common/timeslot"
)

// CSV status values.
const (
	StatusSuccess = "Success"
	StatusError   = "Error"
)

// SendEmail builds an RFC 5322 message, base64url-encodes it, and sends it via
// the Gmail API as the impersonated user ("me").
func SendEmail(ctx context.Context, svc *gmailapi.Service, config *Config, csv logger.Logger) error {
	customHeaders, err := email.ParseHeaders(config.Headers)
	if err != nil {
		return fmt.Errorf("invalid headers: %w", err)
	}

	onSkip := func(path string, loadErr error) {
		log.Printf("Could not read attachment file %s: %v", path, loadErr)
	}
	attachments, err := email.LoadAttachments(config.AttachmentFiles, onSkip)
	if err != nil {
		return fmt.Errorf("attachments: %w", err)
	}
	inline, err := email.LoadInlineAttachments(config.InlineAttachmentFiles, onSkip)
	if err != nil {
		return fmt.Errorf("inline attachments: %w", err)
	}

	// Unlike SMTP, Gmail derives recipients from the message headers, so Bcc
	// must be included in the raw message.
	raw, err := mimebuilder.Build(mimebuilder.Message{
		From:             config.Mailbox,
		To:               config.To,
		Cc:               config.Cc,
		Bcc:              config.Bcc,
		Subject:          config.Subject,
		TextBody:         config.Body,
		HTMLBody:         config.BodyHTML,
		Priority:         config.Priority,
		Headers:          customHeaders,
		Attachments:      attachments,
		Inline:           inline,
		IncludeBccHeader: true,
		HostID:           hostFromEmail(config.Mailbox),
	})
	if err != nil {
		return fmt.Errorf("failed to build message: %w", err)
	}

	// Gmail requires the message base64url-encoded into Raw; the SDK does not
	// encode it for you (standard base64 silently fails).
	gmsg := &gmailapi.Message{Raw: base64.URLEncoding.EncodeToString(raw)}

	var sent *gmailapi.Message
	err = retryGmail(ctx, config, func() error {
		var apiErr error
		sent, apiErr = svc.Users.Messages.Send("me", gmsg).Context(ctx).Do()
		return apiErr
	})

	status := StatusSuccess
	var returnErr error
	if err != nil {
		enriched := enrichGmailAPIError(err, "sendmail")
		log.Printf("Error sending mail: %v", enriched)
		status = fmt.Sprintf("%s: %v", StatusError, enriched)
		returnErr = enriched
	} else {
		fmt.Printf("Email sent successfully from %s.\n", config.Mailbox)
		fmt.Printf("To: %s\n", strings.Join(config.To, ", "))
		if len(config.Cc) > 0 {
			fmt.Printf("Cc: %s\n", strings.Join(config.Cc, ", "))
		}
		if len(config.Bcc) > 0 {
			fmt.Printf("Bcc: %s\n", strings.Join(config.Bcc, ", "))
		}
		fmt.Printf("Subject: %s\n", config.Subject)
		fmt.Printf("Message ID: %s\n", sent.Id)
	}

	writeCSV(csv, []string{ActionSendMail, status, config.Mailbox, strings.Join(config.To, "; "), config.Subject})
	return returnErr
}

// listLabels lists all Gmail labels (system + user) with message counts.
func listLabels(ctx context.Context, svc *gmailapi.Service, config *Config, csv logger.Logger) error {
	var res *gmailapi.ListLabelsResponse
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		res, apiErr = svc.Users.Labels.List("me").Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("error listing labels: %w", enrichGmailAPIError(err, "listfolders"))
	}

	if config.OutputFormat == "json" {
		printJSON(res.Labels)
	} else {
		fmt.Printf("Gmail labels for %s (%d):\n\n", config.Mailbox, len(res.Labels))
		for i, l := range res.Labels {
			fmt.Printf("%d. %-30s  type: %-6s  total: %d  unread: %d\n",
				i+1, l.Name, l.Type, l.MessagesTotal, l.MessagesUnread)
		}
	}

	for _, l := range res.Labels {
		writeCSV(csv, []string{
			ActionListFolders, StatusSuccess, config.Mailbox,
			l.Id, l.Name, l.Type,
			fmt.Sprintf("%d", l.MessagesTotal),
			fmt.Sprintf("%d", l.MessagesUnread),
		})
	}
	return nil
}

// listMailByLabel lists recent messages under a specific Gmail label.
func listMailByLabel(ctx context.Context, svc *gmailapi.Service, config *Config, csv logger.Logger) error {
	label := config.Label
	if label == "" {
		label = "INBOX"
	}

	var listRes *gmailapi.ListMessagesResponse
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		listRes, apiErr = svc.Users.Messages.List("me").LabelIds(label).MaxResults(int64(config.Count)).Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("error listing mail in label %q: %w", label, enrichGmailAPIError(err, "listmail"))
	}

	if len(listRes.Messages) == 0 {
		if config.OutputFormat == "json" {
			printJSON([]any{})
		} else {
			fmt.Printf("No messages found in label %q.\n", label)
		}
		writeCSV(csv, []string{ActionListMail, StatusSuccess, config.Mailbox, label, "No messages found", "N/A"})
		return nil
	}

	type summary struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
		From    string `json:"from"`
		Date    string `json:"date"`
	}
	var summaries []summary
	for _, m := range listRes.Messages {
		var full *gmailapi.Message
		gErr := retryGmail(ctx, config, func() error {
			var apiErr error
			full, apiErr = svc.Users.Messages.Get("me", m.Id).Format("metadata").
				MetadataHeaders("Subject", "From", "Date").Context(ctx).Do()
			return apiErr
		})
		if gErr != nil {
			log.Printf("Error fetching message %s: %v", m.Id, enrichGmailAPIError(gErr, "listmail"))
			continue
		}
		summaries = append(summaries, summary{
			ID:      m.Id,
			Subject: headerValue(full, "Subject"),
			From:    headerValue(full, "From"),
			Date:    headerValue(full, "Date"),
		})
	}

	if config.OutputFormat == "json" {
		printJSON(summaries)
	} else {
		fmt.Printf("Messages in label %q for %s (%d):\n\n", label, config.Mailbox, len(summaries))
		for i, s := range summaries {
			fmt.Printf("%d. %s\n   From: %s\n   Date: %s\n\n", i+1, s.Subject, s.From, s.Date)
		}
	}
	for _, s := range summaries {
		writeCSV(csv, []string{ActionListMail, StatusSuccess, config.Mailbox, label, s.Subject, s.ID})
	}
	return nil
}

// exportMessages searches by rfc822 Message-ID and/or subject (or a raw Gmail
// search query when --search is given), then writes each matching message to a
// .eml file.
func exportMessages(ctx context.Context, svc *gmailapi.Service, config *Config, csv logger.Logger) error {
	query := config.SearchQuery
	if query == "" {
		query = buildSearchQuery(config.MessageID, config.Subject)
	}

	var listRes *gmailapi.ListMessagesResponse
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		listRes, apiErr = svc.Users.Messages.List("me").Q(query).MaxResults(int64(config.Count)).Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("error searching messages: %w", enrichGmailAPIError(err, "exportmessages"))
	}

	if len(listRes.Messages) == 0 {
		if config.OutputFormat == "json" {
			printJSON([]any{})
		} else {
			fmt.Println("No messages found matching the given criteria.")
		}
		writeCSV(csv, []string{ActionExportMessages, StatusSuccess, config.Mailbox, "No messages found (0 messages)", "N/A"})
		return nil
	}

	dir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}
	if config.OutputFormat != "json" {
		fmt.Printf("Export directory: %s\n", dir)
	}

	success := 0
	for _, m := range listRes.Messages {
		filePath, exportErr := exportMessageToEML(ctx, svc, config, m.Id, dir)
		if exportErr != nil {
			log.Printf("Error exporting message %s: %v", m.Id, exportErr)
			writeCSV(csv, []string{ActionExportMessages, StatusError, config.Mailbox, exportErr.Error(), m.Id})
			continue
		}
		success++
		if config.OutputFormat != "json" {
			fmt.Printf("Successfully exported message %s -> %s\n", m.Id, filePath)
		}
		writeCSV(csv, []string{ActionExportMessages, StatusSuccess, config.Mailbox, "Exported successfully", m.Id, filePath})
	}

	if config.OutputFormat != "json" {
		fmt.Printf("Successfully exported %d/%d messages.\n", success, len(listRes.Messages))
	}
	return nil
}

// exportMessageToEML fetches a message's raw RFC822 content, base64url-decodes
// it, and writes it to a .eml file in dir.
func exportMessageToEML(ctx context.Context, svc *gmailapi.Service, config *Config, id, dir string) (string, error) {
	var full *gmailapi.Message
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		full, apiErr = svc.Users.Messages.Get("me", id).Format("raw").Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch message content: %w", enrichGmailAPIError(err, "exportmessages"))
	}

	raw, err := decodeGmailRaw(full.Raw)
	if err != nil {
		return "", fmt.Errorf("failed to decode raw message: %w", err)
	}

	filename := fmt.Sprintf("msg_%s.eml", export.SanitizeFilename(id))
	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, raw, 0o644); err != nil {
		return "", fmt.Errorf("failed to write EML file: %w", err)
	}
	logVerbose(config.VerboseMode, "Exported message to %s", filePath)
	return filePath, nil
}

// listEvents lists upcoming calendar events for the mailbox's calendar.
func listEvents(ctx context.Context, svc *calendarapi.Service, config *Config, csv logger.Logger) error {
	calID := calendarID(config)
	now := time.Now().Format(time.RFC3339)

	var res *calendarapi.Events
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		res, apiErr = svc.Events.List(calID).TimeMin(now).SingleEvents(true).
			OrderBy("startTime").MaxResults(int64(config.Count)).Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("error fetching events: %w", enrichGmailAPIError(err, "getevents"))
	}

	if len(res.Items) == 0 {
		if config.OutputFormat == "json" {
			printJSON([]any{})
		} else {
			fmt.Println("No upcoming events found.")
		}
		writeCSV(csv, []string{ActionGetEvents, StatusSuccess, config.Mailbox, "No events found (0 events)", "N/A"})
		return nil
	}

	if config.OutputFormat == "json" {
		printJSON(res.Items)
	} else {
		for i, e := range res.Items {
			fmt.Printf("%d. %s\n   Start: %s\n   End:   %s\n", i+1, e.Summary, eventTime(e.Start), eventTime(e.End))
		}
	}
	for _, e := range res.Items {
		writeCSV(csv, []string{ActionGetEvents, StatusSuccess, config.Mailbox, e.Summary, e.Id})
	}
	return nil
}

// getSchedule checks a recipient's availability via the Calendar freeBusy
// query over the configured time window (default: the next 24 hours).
func getSchedule(ctx context.Context, svc *calendarapi.Service, config *Config, csv logger.Logger) error {
	recipient := config.To[0]

	start := time.Now().UTC()
	if config.StartTime != "" {
		t, err := parseFlexibleTime(config.StartTime)
		if err != nil {
			return fmt.Errorf("invalid start time: %w", err)
		}
		start = t
	}
	end := start.Add(24 * time.Hour)
	if config.EndTime != "" {
		t, err := parseFlexibleTime(config.EndTime)
		if err != nil {
			return fmt.Errorf("invalid end time: %w", err)
		}
		end = t
	}
	if !end.After(start) {
		return fmt.Errorf("end time must be after start time")
	}

	req := &calendarapi.FreeBusyRequest{
		TimeMin: start.Format(time.RFC3339),
		TimeMax: end.Format(time.RFC3339),
		Items:   []*calendarapi.FreeBusyRequestItem{{Id: recipient}},
	}

	var res *calendarapi.FreeBusyResponse
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		res, apiErr = svc.Freebusy.Query(req).Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("error querying free/busy: %w", enrichGmailAPIError(err, "getschedule"))
	}

	cal, ok := res.Calendars[recipient]
	if !ok {
		msg := fmt.Sprintf("no free/busy data returned for %s", recipient)
		writeCSV(csv, []string{ActionGetSchedule, StatusError, config.Mailbox, recipient, msg})
		return fmt.Errorf("%s", msg)
	}
	if len(cal.Errors) > 0 {
		msg := fmt.Sprintf("free/busy lookup failed for %s: %s", recipient, cal.Errors[0].Reason)
		writeCSV(csv, []string{ActionGetSchedule, StatusError, config.Mailbox, recipient, msg})
		return fmt.Errorf("%s", msg)
	}

	if config.OutputFormat == "json" {
		printJSON(cal.Busy)
	} else {
		fmt.Printf("Availability of %s from %s to %s:\n\n",
			recipient, start.Format("2006-01-02 15:04 MST"), end.Format("2006-01-02 15:04 MST"))
		if len(cal.Busy) == 0 {
			fmt.Println("Free for the entire window (no busy blocks).")
		} else {
			for i, b := range cal.Busy {
				fmt.Printf("%d. Busy: %s — %s\n", i+1, b.Start, b.End)
			}
		}
	}

	if len(cal.Busy) == 0 {
		writeCSV(csv, []string{ActionGetSchedule, StatusSuccess, config.Mailbox, recipient, "Free (no busy blocks)"})
	}
	for _, b := range cal.Busy {
		writeCSV(csv, []string{ActionGetSchedule, StatusSuccess, config.Mailbox, recipient, fmt.Sprintf("Busy %s — %s", b.Start, b.End)})
	}
	return nil
}

// findTimeSlot searches for free meeting slots of the configured duration in
// the recipient's calendar. It fetches busy blocks via the Calendar freeBusy
// API and computes free slots client-side (working hours 08:00-17:00 UTC,
// Monday-Friday).
func findTimeSlot(ctx context.Context, svc *calendarapi.Service, config *Config, csv logger.Logger) error {
	recipient := config.To[0]

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

	req := &calendarapi.FreeBusyRequest{
		TimeMin: windowStart.Format(time.RFC3339),
		TimeMax: windowEnd.Format(time.RFC3339),
		Items:   []*calendarapi.FreeBusyRequestItem{{Id: recipient}},
	}

	var res *calendarapi.FreeBusyResponse
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		res, apiErr = svc.Freebusy.Query(req).Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("error querying free/busy: %w", enrichGmailAPIError(err, "findtimeslot"))
	}

	cal, ok := res.Calendars[recipient]
	if !ok {
		msg := fmt.Sprintf("no free/busy data returned for %s", recipient)
		writeCSV(csv, []string{ActionFindTimeSlot, StatusError, config.Mailbox, recipient, msg, ""})
		return fmt.Errorf("%s", msg)
	}
	if len(cal.Errors) > 0 {
		msg := fmt.Sprintf("free/busy lookup failed for %s: %s", recipient, cal.Errors[0].Reason)
		writeCSV(csv, []string{ActionFindTimeSlot, StatusError, config.Mailbox, recipient, msg, ""})
		return fmt.Errorf("%s", msg)
	}

	var busy []timeslot.Interval
	for _, b := range cal.Busy {
		start, err1 := time.Parse(time.RFC3339, b.Start)
		end, err2 := time.Parse(time.RFC3339, b.End)
		if err1 != nil || err2 != nil {
			logVerbose(config.VerboseMode, "Skipping busy block with unparsable time: %s — %s", b.Start, b.End)
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

	if config.OutputFormat == "json" {
		type slotJSON struct {
			Start string `json:"start"`
			End   string `json:"end"`
		}
		out := make([]slotJSON, 0, len(slots))
		for _, s := range slots {
			out = append(out, slotJSON{Start: s.Start.Format(time.RFC3339), End: s.End.Format(time.RFC3339)})
		}
		printJSON(out)
	} else {
		fmt.Printf("Free %d-minute slots for a meeting with %s:\n", config.Duration, recipient)
		fmt.Printf("Window: %s — %s (working hours 08:00-17:00 UTC, Mon-Fri; %d busy blocks)\n\n",
			windowStart.Format("2006-01-02 15:04 MST"), windowEnd.Format("2006-01-02 15:04 MST"), len(busy))
		if len(slots) == 0 {
			fmt.Println("No free slots found in the window.")
		}
		for i, s := range slots {
			fmt.Printf("%d. %s — %s\n", i+1, s.Start.Format("Mon 2006-01-02 15:04"), s.End.Format("15:04 MST"))
		}
	}

	if len(slots) == 0 {
		writeCSV(csv, []string{ActionFindTimeSlot, StatusSuccess, config.Mailbox, recipient, "No free slots found", ""})
	}
	for _, s := range slots {
		writeCSV(csv, []string{ActionFindTimeSlot, StatusSuccess, config.Mailbox, recipient,
			s.Start.Format(time.RFC3339), s.End.Format(time.RFC3339)})
	}
	return nil
}

// createInvite creates a calendar event and invites the --to attendees.
func createInvite(ctx context.Context, svc *calendarapi.Service, config *Config, csv logger.Logger) error {
	start, err := parseFlexibleTime(config.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}
	end, err := parseFlexibleTime(config.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}

	event := &calendarapi.Event{
		Summary: config.Subject,
		Start:   &calendarapi.EventDateTime{DateTime: start.Format(time.RFC3339)},
		End:     &calendarapi.EventDateTime{DateTime: end.Format(time.RFC3339)},
	}
	for _, addr := range config.To {
		event.Attendees = append(event.Attendees, &calendarapi.EventAttendee{Email: addr})
	}

	calID := calendarID(config)
	var created *calendarapi.Event
	err = retryGmail(ctx, config, func() error {
		var apiErr error
		created, apiErr = svc.Events.Insert(calID, event).SendUpdates("all").Context(ctx).Do()
		return apiErr
	})

	status := StatusSuccess
	eventID := "N/A"
	var returnErr error
	if err != nil {
		enriched := enrichGmailAPIError(err, "sendinvite")
		log.Printf("Error creating invite: %v", enriched)
		status = fmt.Sprintf("%s: %v", StatusError, enriched)
		returnErr = enriched
	} else {
		eventID = created.Id
		fmt.Printf("Calendar invitation created in calendar: %s\n", calID)
		fmt.Printf("Subject: %s\n", config.Subject)
		fmt.Printf("Start: %s\n", start.Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("End: %s\n", end.Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("Event ID: %s\n", eventID)
	}

	writeCSV(csv, []string{ActionSendInvite, status, config.Mailbox, config.Subject, start.Format(time.RFC3339), end.Format(time.RFC3339), eventID})
	return returnErr
}

// testAuth verifies impersonation and scopes with the cheapest authenticated
// call (GetProfile).
func testAuth(ctx context.Context, svc *gmailapi.Service, config *Config, csv logger.Logger) error {
	var profile *gmailapi.Profile
	err := retryGmail(ctx, config, func() error {
		var apiErr error
		profile, apiErr = svc.Users.GetProfile("me").Context(ctx).Do()
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("authentication test failed: %w", enrichGmailAPIError(err, "testauth"))
	}

	fmt.Println("Authentication successful.")
	fmt.Printf("Mailbox: %s\n", profile.EmailAddress)
	fmt.Printf("Messages total: %d\n", profile.MessagesTotal)
	writeCSV(csv, []string{ActionTestAuth, StatusSuccess, profile.EmailAddress, fmt.Sprintf("%d messages", profile.MessagesTotal)})
	return nil
}

// exportBearerToken acquires an access token from the configured credential and
// prints it, so it can be reused elsewhere (e.g. as --bearertoken).
func exportBearerToken(ctx context.Context, config *Config, slogger *slog.Logger, csv logger.Logger) error {
	ts, err := newTokenSource(ctx, config, effectiveScopes(config), slogger)
	if err != nil {
		return err
	}
	tok, err := ts.Token()
	if err != nil {
		return fmt.Errorf("failed to acquire token: %w", enrichGmailAPIError(err, "exportbearertoken"))
	}

	if config.OutputFormat == "json" {
		printJSON(map[string]string{"bearertoken": tok.AccessToken})
	} else {
		fmt.Println(tok.AccessToken)
	}
	writeCSV(csv, []string{ActionExportBearerToken, StatusSuccess, config.Mailbox, "token exported"})
	return nil
}

// buildSearchQuery assembles a Gmail search query from the validated Message-ID
// and/or subject criteria.
func buildSearchQuery(messageID, subject string) string {
	var parts []string
	if messageID != "" {
		parts = append(parts, "rfc822msgid:"+strings.Trim(messageID, "<>"))
	}
	if subject != "" {
		parts = append(parts, fmt.Sprintf("subject:(%s)", subject))
	}
	return strings.Join(parts, " ")
}

// decodeGmailRaw decodes Gmail's URL-safe base64 "raw" field, tolerating the
// presence or absence of padding.
func decodeGmailRaw(s string) ([]byte, error) {
	if data, err := base64.URLEncoding.DecodeString(s); err == nil {
		return data, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// headerValue returns the named header value from a metadata-format message.
func headerValue(msg *gmailapi.Message, name string) string {
	if msg == nil || msg.Payload == nil {
		return ""
	}
	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// eventTime returns the best available timestamp for a calendar event endpoint.
func eventTime(dt *calendarapi.EventDateTime) string {
	if dt == nil {
		return ""
	}
	if dt.DateTime != "" {
		return dt.DateTime
	}
	return dt.Date
}

// calendarID returns the calendar to operate on: the mailbox, or "primary".
func calendarID(config *Config) string {
	if config.Mailbox != "" {
		return config.Mailbox
	}
	return "primary"
}

// hostFromEmail extracts the domain part of an email for Message-ID generation,
// falling back to "gmail.com".
func hostFromEmail(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 && i < len(addr)-1 {
		return addr[i+1:]
	}
	return "gmail.com"
}

// writeCSV writes a CSV row if the logger is non-nil.
func writeCSV(l logger.Logger, row []string) {
	if l != nil {
		_ = l.WriteRow(row)
	}
}
