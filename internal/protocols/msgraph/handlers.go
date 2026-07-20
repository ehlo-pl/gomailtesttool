package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	tmpl "github.com/ehlo-pl/gomailtesttool/internal/common/template"
	"github.com/ehlo-pl/gomailtesttool/internal/common/timeslot"
)

// Status constants
const (
	StatusSuccess = "Success"
	StatusError   = "Error"
)

func listEvents(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, count int, config *Config, logger logger.Logger) error {
	// Configure request to get top N events
	requestConfig := &users.ItemEventsRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemEventsRequestBuilderGetQueryParameters{
			Top: Int32Ptr(int32(count)),
		},
	}

	logVerbose(config.VerboseMode, "Calling Graph API: GET /users/%s/events?$top=%d", mailbox, count)

	// Execute API call with retry logic
	var getValueFunc func() []models.Eventable
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		apiResult, apiErr := client.Users().ByUserId(mailbox).Events().Get(ctx, requestConfig)
		if apiErr == nil {
			getValueFunc = apiResult.GetValue
		}
		return apiErr
	})

	if err != nil {
		// Enrich error with rate limit and service error details
		enrichedErr := enrichGraphAPIError(err, logger, "listEvents")
		return fmt.Errorf("error fetching calendar for %s: %w", mailbox, enrichedErr)
	}

	events := getValueFunc()
	eventCount := len(events)

	logVerbose(config.VerboseMode, "API response received: %d events", eventCount)

	if config.OutputFormat == "json" {
		printJSON(formatEventsOutput(events))
	} else {
		fmt.Printf("Upcoming events for %s:\n", mailbox)

		if eventCount == 0 {
			fmt.Println("No events found.")
		} else {
			for _, event := range events {
				subject := "N/A"
				if event.GetSubject() != nil {
					subject = *event.GetSubject()
				}

				id := "N/A"
				if event.GetId() != nil {
					id = *event.GetId()
				}

				fmt.Printf("- %s (ID: %s)\n", subject, id)
			}
			// Log summary entry after all events
			fmt.Printf("\nTotal events retrieved: %d\n", eventCount)
		}
	}

	// Always write to CSV logger regardless of output format
	if logger != nil {
		if eventCount == 0 {
			_ = logger.WriteRow([]string{ActionGetEvents, StatusSuccess, mailbox, "No events found (0 events)", "N/A"})
		} else {
			for _, event := range events {
				subject := "N/A"
				if event.GetSubject() != nil {
					subject = *event.GetSubject()
				}
				id := "N/A"
				if event.GetId() != nil {
					id = *event.GetId()
				}
				_ = logger.WriteRow([]string{ActionGetEvents, StatusSuccess, mailbox, subject, id})
			}
			_ = logger.WriteRow([]string{ActionGetEvents, StatusSuccess, mailbox, fmt.Sprintf("Retrieved %d event(s)", eventCount), "SUMMARY"})
		}
	}

	return nil
}

// resolveTemplate renders --template (if set) and applies it to the config.
// HTML mode (non-.eml extension) fills BodyHTML with the rendered content;
// EML mode parses the rendered message and maps its recognised fields onto
// the config consumed by the Graph send API: recipients fall back to the EML
// headers where flags were not provided, subject and bodies come from the
// EML. The From header is ignored (Graph always sends as --mailbox) and any
// unmapped headers are reported in verbose mode.
func resolveTemplate(config *Config) error {
	if config.Template == "" {
		return nil
	}

	vars, err := tmpl.ParseVars(config.TemplateVars)
	if err != nil {
		return fmt.Errorf("invalid --template-vars: %w", err)
	}
	rendered, err := tmpl.Render(config.Template, vars)
	if err != nil {
		return err
	}

	if !tmpl.IsEML(config.Template) {
		config.BodyHTML = rendered
		return nil
	}

	parsed, err := tmpl.ParseEML(rendered)
	if err != nil {
		return fmt.Errorf("template %s: %w", config.Template, err)
	}
	if parsed.TextBody == "" && parsed.HTMLBody == "" {
		return fmt.Errorf("template %s has no text or HTML body", config.Template)
	}
	if len(config.To) == 0 {
		config.To = stringSlice(parsed.To)
	}
	if len(config.Cc) == 0 {
		config.Cc = stringSlice(parsed.Cc)
	}
	if len(config.Bcc) == 0 {
		config.Bcc = stringSlice(parsed.Bcc)
	}
	if parsed.Subject != "" {
		config.Subject = parsed.Subject
	}
	config.Body = parsed.TextBody
	config.BodyHTML = parsed.HTMLBody

	if parsed.From != "" && !strings.EqualFold(parsed.From, config.Mailbox) {
		logVerbose(config.VerboseMode, "Template From header %s is ignored; Graph sends as %s", parsed.From, config.Mailbox)
	}
	if len(parsed.UnmappedHeaders) > 0 {
		logVerbose(config.VerboseMode, "Template headers not mapped to the Graph API: %s", strings.Join(parsed.UnmappedHeaders, ", "))
	}
	return nil
}

// SendEmail sends an email via the Microsoft Graph API.
// Returns a non-nil error if the send fails; logging to CSV and console occurs regardless.
func SendEmail(ctx context.Context, client *msgraphsdk.GraphServiceClient, senderMailbox string, to, cc, bcc []string, subject, textContent, htmlContent string, attachmentPaths []string, config *Config, logger logger.Logger) error {
	message := models.NewMessage()

	// Set Subject
	message.SetSubject(&subject)

	// Set Importance (high/normal/low); defaults to normal
	if importance, err := models.ParseImportance(config.Priority); err == nil && importance != nil {
		imp := importance.(*models.Importance)
		message.SetImportance(imp)
		logVerbose(config.VerboseMode, "Email importance: %s", config.Priority)
	}

	// Set body - prefer HTML if provided, otherwise use text
	body := models.NewItemBody()
	if htmlContent != "" {
		body.SetContent(&htmlContent)
		contentType := models.HTML_BODYTYPE
		body.SetContentType(&contentType)
		logVerbose(config.VerboseMode, "Email body type: HTML")
	} else {
		body.SetContent(&textContent)
		contentType := models.TEXT_BODYTYPE
		body.SetContentType(&contentType)
		logVerbose(config.VerboseMode, "Email body type: Text")
	}
	message.SetBody(body)

	// Add Recipients
	if len(to) > 0 {
		message.SetToRecipients(createRecipients(to))
	}
	if len(cc) > 0 {
		message.SetCcRecipients(createRecipients(cc))
	}
	if len(bcc) > 0 {
		message.SetBccRecipients(createRecipients(bcc))
	}

	// Add Attachments (regular + inline)
	if len(attachmentPaths) > 0 || len(config.InlineAttachmentFiles) > 0 {
		fileAttachments, err := createFileAttachments(attachmentPaths, config.InlineAttachmentFiles, config)
		if err != nil {
			log.Printf("Error creating attachments: %v", err)
		} else if len(fileAttachments) > 0 {
			message.SetAttachments(fileAttachments)
			logVerbose(config.VerboseMode, "Attachments added: %d file(s)", len(fileAttachments))
		}
	}

	// Add custom headers
	if len(config.Headers) > 0 {
		if headers, err := email.ParseHeaders(config.Headers); err != nil {
			log.Printf("Error parsing custom headers: %v", err)
		} else if len(headers) > 0 {
			msgHeaders := make([]models.InternetMessageHeaderable, 0, len(headers))
			for _, h := range headers {
				header := models.NewInternetMessageHeader()
				name, value := h.Name, h.Value
				header.SetName(&name)
				header.SetValue(&value)
				msgHeaders = append(msgHeaders, header)
			}
			message.SetInternetMessageHeaders(msgHeaders)
			logVerbose(config.VerboseMode, "Custom headers added: %d", len(msgHeaders))
		}
	}

	requestBody := users.NewItemSendMailPostRequestBody()
	requestBody.SetMessage(message)
	saveToSentItems := config.SaveToSent
	requestBody.SetSaveToSentItems(&saveToSentItems)

	logVerbose(config.VerboseMode, "Calling Graph API: POST /users/%s/sendMail", senderMailbox)
	logVerbose(config.VerboseMode, "Email details - To: %v, CC: %v, BCC: %v", to, cc, bcc)
	err := client.Users().ByUserId(senderMailbox).SendMail().Post(ctx, requestBody, nil)

	status := StatusSuccess
	attachmentCount := len(attachmentPaths) + len(config.InlineAttachmentFiles)
	var returnErr error
	if err != nil {
		// Enrich error with rate limit and service error details
		enrichedErr := enrichGraphAPIError(err, logger, "sendEmail")
		log.Printf("Error sending mail: %v", enrichedErr)
		status = fmt.Sprintf("%s: %v", StatusError, enrichedErr)
		returnErr = enrichedErr
	} else {
		logVerbose(config.VerboseMode, "Email sent successfully via Graph API")
		fmt.Printf("Email sent successfully from %s.\n", senderMailbox)
		fmt.Printf("To: %v\n", to)
		fmt.Printf("Cc: %v\n", cc)
		fmt.Printf("Bcc: %v\n", bcc)
		fmt.Printf("Subject: %s\n", subject)
		if htmlContent != "" {
			fmt.Println("Body Type: HTML")
		} else {
			fmt.Println("Body Type: Text")
		}
		if attachmentCount > 0 {
			fmt.Printf("Attachments: %d file(s)\n", attachmentCount)
		}
	}

	// Write to CSV
	if logger != nil {
		toStr := strings.Join(to, "; ")
		ccStr := strings.Join(cc, "; ")
		bccStr := strings.Join(bcc, "; ")
		bodyType := "Text"
		if htmlContent != "" {
			bodyType = "HTML"
		}
		_ = logger.WriteRow([]string{ActionSendMail, status, senderMailbox, toStr, ccStr, bccStr, subject, bodyType, fmt.Sprintf("%d", attachmentCount)})
	}

	return returnErr
}

func createInvite(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox, subject, startTimeStr, endTimeStr string, config *Config, logger logger.Logger) {
	event := models.NewEvent()
	event.SetSubject(&subject)

	// Parse start time, default to now if not provided
	var startTime time.Time
	var err error
	if startTimeStr == "" {
		startTime = time.Now()
	} else {
		startTime, err = parseFlexibleTime(startTimeStr)
		if err != nil {
			log.Printf("Error parsing start time: %v. Using current time instead.", err)
			startTime = time.Now()
		}
	}

	// Parse end time, default to 1 hour after start if not provided
	var endTime time.Time
	if endTimeStr == "" {
		endTime = startTime.Add(1 * time.Hour)
	} else {
		endTime, err = parseFlexibleTime(endTimeStr)
		if err != nil {
			log.Printf("Error parsing end time: %v. Using start + 1 hour instead.", err)
			endTime = startTime.Add(1 * time.Hour)
		}
	}

	// Set start time
	startDateTime := models.NewDateTimeTimeZone()
	startTimeFormatted := startTime.Format(time.RFC3339)
	startDateTime.SetDateTime(&startTimeFormatted)
	timezone := "UTC"
	startDateTime.SetTimeZone(&timezone)
	event.SetStart(startDateTime)

	// Set end time
	endDateTime := models.NewDateTimeTimeZone()
	endTimeFormatted := endTime.Format(time.RFC3339)
	endDateTime.SetDateTime(&endTimeFormatted)
	endDateTime.SetTimeZone(&timezone)
	event.SetEnd(endDateTime)

	// Create the event
	logVerbose(config.VerboseMode, "Calling Graph API: POST /users/%s/events", mailbox)
	logVerbose(config.VerboseMode, "Calendar invite - Subject: %s, Start: %s, End: %s", subject, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	createdEvent, err := client.Users().ByUserId(mailbox).Events().Post(ctx, event, nil)

	status := StatusSuccess
	eventID := "N/A"
	if err != nil {
		// Enrich error with rate limit and service error details
		enrichedErr := enrichGraphAPIError(err, logger, "createInvite")
		log.Printf("Error creating invite: %v", enrichedErr)
		status = fmt.Sprintf("%s: %v", StatusError, enrichedErr)
	} else {
		if createdEvent.GetId() != nil {
			eventID = *createdEvent.GetId()
		}
		logVerbose(config.VerboseMode, "Calendar event created successfully via Graph API")
		logVerbose(config.VerboseMode, "Event ID: %s", eventID)
		fmt.Printf("Calendar invitation created in mailbox: %s\n", mailbox)
		fmt.Printf("Subject: %s\n", subject)
		fmt.Printf("Start: %s\n", startTime.Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("End: %s\n", endTime.Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("Event ID: %s\n", eventID)
	}

	// Write to CSV
	if logger != nil {
		_ = logger.WriteRow([]string{ActionSendInvite, status, mailbox, subject, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339), eventID})
	}
}

func listMailFolders(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, config *Config, csvLogger logger.Logger) error {
	requestConfig := &users.ItemMailFoldersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersRequestBuilderGetQueryParameters{
			Select: []string{"displayName", "totalItemCount", "unreadItemCount", "childFolderCount"},
		},
	}

	logVerbose(config.VerboseMode, "Calling Graph API: GET /users/%s/mailFolders", mailbox)

	var getValueFunc func() []models.MailFolderable
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		result, apiErr := client.Users().ByUserId(mailbox).MailFolders().Get(ctx, requestConfig)
		if apiErr == nil {
			getValueFunc = result.GetValue
		}
		return apiErr
	})
	if err != nil {
		enrichedErr := enrichGraphAPIError(err, csvLogger, "listfolders")
		return fmt.Errorf("error listing folders for %s: %w", mailbox, enrichedErr)
	}

	folders := getValueFunc()

	if config.OutputFormat == "json" {
		type folderJSON struct {
			Name         string `json:"displayName"`
			TotalItems   int32  `json:"totalItemCount"`
			UnreadItems  int32  `json:"unreadItemCount"`
			ChildFolders int32  `json:"childFolderCount"`
		}
		out := make([]folderJSON, 0, len(folders))
		for _, f := range folders {
			out = append(out, folderJSON{
				Name:         derefOr(f.GetDisplayName(), ""),
				TotalItems:   derefInt32(f.GetTotalItemCount()),
				UnreadItems:  derefInt32(f.GetUnreadItemCount()),
				ChildFolders: derefInt32(f.GetChildFolderCount()),
			})
		}
		printJSON(out)
	} else {
		fmt.Printf("Mail folders for %s (%d):\n\n", mailbox, len(folders))
		for i, f := range folders {
			fmt.Printf("%d. %s  (total: %d, unread: %d, child folders: %d)\n",
				i+1,
				derefOr(f.GetDisplayName(), ""),
				derefInt32(f.GetTotalItemCount()),
				derefInt32(f.GetUnreadItemCount()),
				derefInt32(f.GetChildFolderCount()),
			)
		}
	}

	if csvLogger != nil {
		for _, f := range folders {
			_ = csvLogger.WriteRow([]string{
				ActionListFolders, StatusSuccess, mailbox,
				derefOr(f.GetDisplayName(), ""),
				fmt.Sprintf("%d", derefInt32(f.GetTotalItemCount())),
				fmt.Sprintf("%d", derefInt32(f.GetUnreadItemCount())),
				fmt.Sprintf("%d", derefInt32(f.GetChildFolderCount())),
				"",
			})
		}
	}

	return nil
}

func listMailInFolder(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, folder string, count int, config *Config, csvLogger logger.Logger) error {
	requestConfig := &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
			Top:     Int32Ptr(int32(count)),
			Orderby: []string{"receivedDateTime DESC"},
			Select:  []string{"subject", "receivedDateTime", "from", "toRecipients"},
		},
	}

	logVerbose(config.VerboseMode, "Calling Graph API: GET /users/%s/mailFolders/%s/messages?$top=%d", mailbox, folder, count)

	var getValueFunc func() []models.Messageable
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		result, apiErr := client.Users().ByUserId(mailbox).MailFolders().ByMailFolderId(folder).Messages().Get(ctx, requestConfig)
		if apiErr == nil {
			getValueFunc = result.GetValue
		}
		return apiErr
	})
	if err != nil {
		enrichedErr := enrichGraphAPIError(err, csvLogger, "listmail")
		return fmt.Errorf("error listing mail in folder %q for %s: %w", folder, mailbox, enrichedErr)
	}

	messages := getValueFunc()
	messageCount := len(messages)

	if config.OutputFormat == "json" {
		printJSON(formatMessagesOutput(messages))
	} else {
		fmt.Printf("Messages in %q for %s (%d):\n\n", folder, mailbox, messageCount)
		if messageCount == 0 {
			fmt.Println("No messages found.")
		}
		for i, msg := range messages {
			sender := "N/A"
			if msg.GetFrom() != nil && msg.GetFrom().GetEmailAddress() != nil && msg.GetFrom().GetEmailAddress().GetAddress() != nil {
				sender = *msg.GetFrom().GetEmailAddress().GetAddress()
			}
			subject := derefOr(msg.GetSubject(), "N/A")
			receivedDate := "N/A"
			if msg.GetReceivedDateTime() != nil {
				receivedDate = msg.GetReceivedDateTime().Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%d. Subject: %s\n   From: %s\n   Received: %s\n\n", i+1, subject, sender, receivedDate)
		}
	}

	if csvLogger != nil {
		if messageCount == 0 {
			_ = csvLogger.WriteRow([]string{ActionListMail, StatusSuccess, mailbox, folder, "No messages found", "N/A", "N/A"})
		}
		for _, msg := range messages {
			sender := "N/A"
			if msg.GetFrom() != nil && msg.GetFrom().GetEmailAddress() != nil && msg.GetFrom().GetEmailAddress().GetAddress() != nil {
				sender = *msg.GetFrom().GetEmailAddress().GetAddress()
			}
			subject := derefOr(msg.GetSubject(), "N/A")
			receivedDate := "N/A"
			if msg.GetReceivedDateTime() != nil {
				receivedDate = msg.GetReceivedDateTime().Format("2006-01-02 15:04:05")
			}
			_ = csvLogger.WriteRow([]string{ActionListMail, StatusSuccess, mailbox, folder, subject, sender, receivedDate})
		}
	}

	return nil
}

// checkAvailability checks the recipient's availability for the next working day at 12:00 UTC.
func checkAvailability(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, recipient string, config *Config, logger logger.Logger) error {
	// Calculate next working day
	now := time.Now().UTC()
	nextWorkingDay := addWorkingDays(now, 1)

	// Set time to 12:00 UTC (noon)
	checkDateTime := time.Date(
		nextWorkingDay.Year(),
		nextWorkingDay.Month(),
		nextWorkingDay.Day(),
		12, 0, 0, 0,
		time.UTC,
	)

	// End time is 1 hour later (13:00 UTC)
	endDateTime := checkDateTime.Add(1 * time.Hour)

	logVerbose(config.VerboseMode, "Checking availability for %s on %s (12:00-13:00 UTC)", recipient, checkDateTime.Format("2006-01-02"))

	// Create DateTimeTimeZone objects for Graph API
	startTimeZone := models.NewDateTimeTimeZone()
	startTimeZone.SetDateTime(pointerTo(checkDateTime.Format(time.RFC3339)))
	startTimeZone.SetTimeZone(pointerTo("UTC"))

	endTimeZone := models.NewDateTimeTimeZone()
	endTimeZone.SetDateTime(pointerTo(endDateTime.Format(time.RFC3339)))
	endTimeZone.SetTimeZone(pointerTo("UTC"))

	// Create request body
	requestBody := users.NewItemCalendarGetSchedulePostRequestBody()
	requestBody.SetSchedules([]string{recipient})
	requestBody.SetStartTime(startTimeZone)
	requestBody.SetEndTime(endTimeZone)
	interval := int32(60) // 60-minute intervals
	requestBody.SetAvailabilityViewInterval(&interval)

	logVerbose(config.VerboseMode, "Calling Graph API: POST /users/%s/calendar/getSchedule", mailbox)

	// Execute API call with retry logic
	var scheduleInfo []models.ScheduleInformationable
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		response, apiErr := client.Users().ByUserId(mailbox).Calendar().GetSchedule().Post(ctx, requestBody, nil)
		if apiErr == nil && response != nil {
			scheduleInfo = response.GetValue()
		}
		return apiErr
	})

	if err != nil {
		// Enrich error with rate limit and service error details
		enrichedErr := enrichGraphAPIError(err, logger, "checkAvailability")
		csvRow := []string{ActionGetSchedule, fmt.Sprintf("Error: %v", enrichedErr), mailbox, recipient, checkDateTime.Format(time.RFC3339), "N/A"}
		if logger != nil {
			_ = logger.WriteRow(csvRow)
		}
		return fmt.Errorf("error checking availability for %s: %w", recipient, enrichedErr)
	}

	logVerbose(config.VerboseMode, "API response received: %d schedule(s)", len(scheduleInfo))

	// Parse availability view
	if len(scheduleInfo) == 0 {
		errMsg := "no schedule information returned"
		csvRow := []string{ActionGetSchedule, fmt.Sprintf("Error: %s", errMsg), mailbox, recipient, checkDateTime.Format(time.RFC3339), "N/A"}
		if logger != nil {
			_ = logger.WriteRow(csvRow)
		}
		return fmt.Errorf("no schedule information returned")
	}

	// Get availability view from first schedule
	info := scheduleInfo[0]
	availabilityView := ""
	if info.GetAvailabilityView() != nil {
		availabilityView = *info.GetAvailabilityView()
	}

	if availabilityView == "" {
		errMsg := "empty availability view returned"
		csvRow := []string{ActionGetSchedule, fmt.Sprintf("Error: %s", errMsg), mailbox, recipient, checkDateTime.Format(time.RFC3339), "N/A"}
		if logger != nil {
			_ = logger.WriteRow(csvRow)
		}
		return fmt.Errorf("empty availability view returned")
	}

	// Interpret availability
	status := interpretAvailability(availabilityView)

	if config.OutputFormat == "json" {
		printJSON(formatScheduleOutput(scheduleInfo))
	} else {
		// Display results
		fmt.Printf("Availability Check Results:\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Organizer:     %s\n", mailbox)
		fmt.Printf("Recipient:     %s\n", recipient)
		fmt.Printf("Check Date:    %s\n", checkDateTime.Format("2006-01-02"))
		fmt.Printf("Check Time:    12:00-13:00 UTC\n")
		fmt.Printf("Status:        %s\n", status)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	}

	logVerbose(config.VerboseMode, "Availability view: %s → %s", availabilityView, status)

	// Log to CSV
	if logger != nil {
		csvRow := []string{ActionGetSchedule, StatusSuccess, mailbox, recipient, checkDateTime.Format(time.RFC3339), availabilityView}
		_ = logger.WriteRow(csvRow)
	}

	return nil
}

// findTimeSlot searches for free meeting slots of the configured duration in
// the recipient's calendar. It fetches the recipient's busy items via the
// getSchedule API over the whole window and computes free slots client-side
// (working hours 08:00-17:00 UTC, Monday-Friday).
func findTimeSlot(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, recipient string, config *Config, logger logger.Logger) error {
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

	logVerbose(config.VerboseMode, "Searching %d-minute slots for %s between %s and %s",
		config.Duration, recipient, windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339))

	startTimeZone := models.NewDateTimeTimeZone()
	startTimeZone.SetDateTime(pointerTo(windowStart.Format(time.RFC3339)))
	startTimeZone.SetTimeZone(pointerTo("UTC"))

	endTimeZone := models.NewDateTimeTimeZone()
	endTimeZone.SetDateTime(pointerTo(windowEnd.Format(time.RFC3339)))
	endTimeZone.SetTimeZone(pointerTo("UTC"))

	requestBody := users.NewItemCalendarGetSchedulePostRequestBody()
	requestBody.SetSchedules([]string{recipient})
	requestBody.SetStartTime(startTimeZone)
	requestBody.SetEndTime(endTimeZone)
	interval := int32(30)
	requestBody.SetAvailabilityViewInterval(&interval)

	logVerbose(config.VerboseMode, "Calling Graph API: POST /users/%s/calendar/getSchedule", mailbox)

	var scheduleInfo []models.ScheduleInformationable
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		response, apiErr := client.Users().ByUserId(mailbox).Calendar().GetSchedule().Post(ctx, requestBody, nil)
		if apiErr == nil && response != nil {
			scheduleInfo = response.GetValue()
		}
		return apiErr
	})
	if err != nil {
		enrichedErr := enrichGraphAPIError(err, logger, "findTimeSlot")
		if logger != nil {
			_ = logger.WriteRow([]string{ActionFindTimeSlot, fmt.Sprintf("Error: %v", enrichedErr), mailbox, recipient, "", ""})
		}
		return fmt.Errorf("error fetching schedule for %s: %w", recipient, enrichedErr)
	}

	if len(scheduleInfo) == 0 {
		errMsg := "no schedule information returned"
		if logger != nil {
			_ = logger.WriteRow([]string{ActionFindTimeSlot, fmt.Sprintf("Error: %s", errMsg), mailbox, recipient, "", ""})
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Collect busy intervals (everything not explicitly free).
	var busy []timeslot.Interval
	for _, item := range scheduleInfo[0].GetScheduleItems() {
		if status := item.GetStatus(); status != nil && *status == models.FREE_FREEBUSYSTATUS {
			continue
		}
		start, err1 := parseGraphDateTime(item.GetStart())
		end, err2 := parseGraphDateTime(item.GetEnd())
		if err1 != nil || err2 != nil {
			logVerbose(config.VerboseMode, "Skipping schedule item with unparsable time: %v %v", err1, err2)
			continue
		}
		busy = append(busy, timeslot.Interval{Start: start, End: end})
	}

	slots := timeslot.FindFreeSlots(busy, windowStart, windowEnd, timeslot.Options{
		Duration:     time.Duration(config.Duration) * time.Minute,
		Count:        config.Count,
		WorkDayStart: 8,
		WorkDayEnd:   17,
		WorkdaysOnly: true,
	})

	printTimeSlots(config.OutputFormat, mailbox, recipient, config.Duration, windowStart, windowEnd, busy, slots)

	if logger != nil {
		if len(slots) == 0 {
			_ = logger.WriteRow([]string{ActionFindTimeSlot, StatusSuccess, mailbox, recipient, "No free slots found", ""})
		}
		for _, slot := range slots {
			_ = logger.WriteRow([]string{ActionFindTimeSlot, StatusSuccess, mailbox, recipient,
				slot.Start.Format(time.RFC3339), slot.End.Format(time.RFC3339)})
		}
	}

	return nil
}

// parseGraphDateTime parses a Graph DateTimeTimeZone value requested in UTC
// (e.g. "2026-07-20T08:00:00.0000000").
func parseGraphDateTime(dt models.DateTimeTimeZoneable) (time.Time, error) {
	if dt == nil || dt.GetDateTime() == nil {
		return time.Time{}, fmt.Errorf("missing dateTime")
	}
	s := *dt.GetDateTime()
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid dateTime %q: %w", s, err)
	}
	return t.UTC(), nil
}

// printTimeSlots renders the findtimeslot result in text or JSON form.
func printTimeSlots(outputFormat, mailbox, recipient string, durationMinutes int, windowStart, windowEnd time.Time, busy []timeslot.Interval, slots []timeslot.Interval) {
	if outputFormat == "json" {
		type slotJSON struct {
			Start string `json:"start"`
			End   string `json:"end"`
		}
		out := make([]slotJSON, 0, len(slots))
		for _, s := range slots {
			out = append(out, slotJSON{Start: s.Start.Format(time.RFC3339), End: s.End.Format(time.RFC3339)})
		}
		printJSON(out)
		return
	}

	fmt.Printf("Free %d-minute slots for a meeting with %s:\n", durationMinutes, recipient)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Organizer:     %s\n", mailbox)
	fmt.Printf("Window:        %s — %s\n", windowStart.Format("2006-01-02 15:04 MST"), windowEnd.Format("2006-01-02 15:04 MST"))
	fmt.Printf("Busy blocks:   %d\n", len(busy))
	fmt.Printf("Working hours: 08:00-17:00 UTC, Monday-Friday\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	if len(slots) == 0 {
		fmt.Println("No free slots found in the window.")
		return
	}
	for i, s := range slots {
		fmt.Printf("%d. %s — %s\n", i+1, s.Start.Format("Mon 2006-01-02 15:04"), s.End.Format("15:04 MST"))
	}
}

// exportInbox exports messages from the inbox to JSON files
func exportInbox(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, count int, config *Config, logger logger.Logger) error {
	// Configure request to get top N messages
	requestConfig := &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
			Top:     Int32Ptr(int32(count)),
			Orderby: []string{"receivedDateTime DESC"},
			Select:  []string{"id", "internetMessageId", "subject", "receivedDateTime", "from", "toRecipients", "ccRecipients", "bccRecipients", "body", "hasAttachments"},
		},
	}

	logVerbose(config.VerboseMode, "Calling Graph API: GET /users/%s/mailFolders/Inbox/messages?$top=%d&$orderby=receivedDateTime DESC", mailbox, count)

	// Execute API call with retry logic
	var getValueFunc func() []models.Messageable
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		// Specifically target Inbox folder
		apiResult, apiErr := client.Users().ByUserId(mailbox).MailFolders().ByMailFolderId("Inbox").Messages().Get(ctx, requestConfig)
		if apiErr == nil {
			getValueFunc = apiResult.GetValue
		}
		return apiErr
	})

	if err != nil {
		enrichedErr := enrichGraphAPIError(err, logger, "exportInbox")
		return fmt.Errorf("error fetching inbox for %s: %w", mailbox, enrichedErr)
	}

	messages := getValueFunc()
	messageCount := len(messages)

	logVerbose(config.VerboseMode, "API response received: %d messages", messageCount)

	if config.OutputFormat != "json" {
		fmt.Printf("Exporting %d messages from inbox for %s...\n", messageCount, mailbox)
	}

	if messageCount == 0 {
		if config.OutputFormat == "json" {
			printJSON([]interface{}{})
		} else {
			fmt.Println("No messages found.")
		}
		if logger != nil {
			_ = logger.WriteRow([]string{ActionExportInbox, StatusSuccess, mailbox, "No messages found (0 messages)", "N/A"})
		}
		return nil
	}

	// Print JSON output if requested
	if config.OutputFormat == "json" {
		printJSON(formatMessagesOutput(messages))
	}

	// Create export directory
	exportDir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}

	if config.OutputFormat != "json" {
		fmt.Printf("Export directory: %s\n", exportDir)
	}

	successCount := 0
	for _, message := range messages {
		if err := exportMessageToJSON(message, exportDir, config); err != nil {
			log.Printf("Error exporting message ID %s: %v", *message.GetId(), err)
			continue
		}
		successCount++
	}

	if config.OutputFormat != "json" {
		fmt.Printf("Successfully exported %d/%d messages.\n", successCount, messageCount)
	}
	if logger != nil {
		_ = logger.WriteRow([]string{ActionExportInbox, StatusSuccess, mailbox, fmt.Sprintf("Exported %d/%d messages", successCount, messageCount), exportDir})
	}

	return nil
}

// searchAndExport searches for a message by Internet Message ID and exports it
func searchAndExport(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, messageID string, config *Config, logger logger.Logger) error {
	// Configure request with filter
	// Note: We search the whole mailbox (Messages endpoint), not just Inbox
	// SECURITY: Escape single quotes for OData filter (defense-in-depth)
	// Even though validateMessageID() blocks quotes, we escape as an additional safeguard
	escapedMessageID := strings.ReplaceAll(messageID, "'", "''")
	filter := fmt.Sprintf("internetMessageId eq '%s'", escapedMessageID)
	requestConfig := &users.ItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMessagesRequestBuilderGetQueryParameters{
			Filter: &filter,
			Select: []string{"id", "internetMessageId", "subject", "receivedDateTime", "from", "toRecipients", "ccRecipients", "bccRecipients", "body", "hasAttachments"},
		},
	}

	logVerbose(config.VerboseMode, "Calling Graph API: GET /users/%s/messages?$filter=%s", mailbox, filter)

	// Execute API call with retry logic; empty results are also retried
	// because Graph is eventually consistent for just-delivered messages.
	messages, err := fetchMessagesWithRetry(ctx, config.MaxRetries, config.RetryDelay, "searchAndExport", func() ([]models.Messageable, error) {
		apiResult, apiErr := client.Users().ByUserId(mailbox).Messages().Get(ctx, requestConfig)
		if apiErr != nil {
			return nil, apiErr
		}
		return apiResult.GetValue(), nil
	})

	if err != nil {
		enrichedErr := enrichGraphAPIError(err, logger, "searchAndExport")
		return fmt.Errorf("error searching message for %s: %w", mailbox, enrichedErr)
	}

	messageCount := len(messages)

	logVerbose(config.VerboseMode, "API response received: %d messages", messageCount)

	if messageCount == 0 {
		if config.OutputFormat == "json" {
			printJSON([]interface{}{}) // Empty array
		} else {
			fmt.Printf("No message found with Internet Message ID: %s\n", messageID)
		}
		if logger != nil {
			_ = logger.WriteRow([]string{ActionSearchAndExport, StatusSuccess, mailbox, "Message not found", messageID})
		}
		return nil
	}

	// Print JSON output if requested
	if config.OutputFormat == "json" {
		printJSON(formatMessagesOutput(messages))
	}

	// Create export directory
	exportDir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}

	if config.OutputFormat != "json" {
		fmt.Printf("Export directory: %s\n", exportDir)
	}

	// Export found messages (usually 1, but duplicates technically possible in some scenarios)
	for _, message := range messages {
		if err := exportMessageToJSON(message, exportDir, config); err != nil {
			return fmt.Errorf("failed to export message: %w", err)
		}
		if config.OutputFormat != "json" {
			fmt.Printf("Successfully exported message: %s\n", derefOr(message.GetSubject(), "(no subject)"))
		}
		if logger != nil {
			_ = logger.WriteRow([]string{ActionSearchAndExport, StatusSuccess, mailbox, "Exported successfully", derefOr(message.GetId(), "")})
		}
	}

	return nil
}

// exportMessages searches for messages matching the given Internet Message-ID
// and/or subject pattern and exports each match as a raw .eml (RFC822) file.
// When folder is non-empty the search is scoped to that mail folder; with no
// other criteria the newest count messages of the folder are exported.
func exportMessages(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, messageID string, subject string, folder string, count int, config *Config, logger logger.Logger) error {
	// Build OData filter from the supplied criteria.
	// SECURITY: Escape single quotes for OData filter (defense-in-depth);
	// validateMessageID()/validateSearchSubject() already reject control
	// characters and (for messageID) quotes.
	var clauses []string
	if messageID != "" {
		escapedMessageID := strings.ReplaceAll(messageID, "'", "''")
		clauses = append(clauses, fmt.Sprintf("internetMessageId eq '%s'", escapedMessageID))
	}
	if subject != "" {
		escapedSubject := strings.ReplaceAll(subject, "'", "''")
		clauses = append(clauses, fmt.Sprintf("contains(subject,'%s')", escapedSubject))
	}
	filter := strings.Join(clauses, " and ")

	// Graph rejects $orderby combined with a $filter on unrelated properties,
	// so only order (newest first) when exporting a whole folder unfiltered.
	var filterPtr *string
	var orderBy []string
	if filter != "" {
		filterPtr = &filter
	} else {
		orderBy = []string{"receivedDateTime DESC"}
	}
	selectFields := []string{"id", "internetMessageId", "subject", "receivedDateTime", "from", "toRecipients", "ccRecipients", "bccRecipients", "hasAttachments"}

	logVerbose(config.VerboseMode, "Calling Graph API: GET /users/%s/%s?$filter=%s&$top=%d", mailbox, folderPathSegment(folder), filter, count)

	// Execute API call with retry logic; empty results are also retried
	// because Graph is eventually consistent for just-delivered messages.
	messages, err := fetchMessagesWithRetry(ctx, config.MaxRetries, config.RetryDelay, "exportMessages", func() ([]models.Messageable, error) {
		if folder != "" {
			requestConfig := &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
				QueryParameters: &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
					Filter:  filterPtr,
					Top:     Int32Ptr(int32(count)),
					Orderby: orderBy,
					Select:  selectFields,
				},
			}
			apiResult, apiErr := client.Users().ByUserId(mailbox).MailFolders().ByMailFolderId(folder).Messages().Get(ctx, requestConfig)
			if apiErr != nil {
				return nil, apiErr
			}
			return apiResult.GetValue(), nil
		}
		requestConfig := &users.ItemMessagesRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemMessagesRequestBuilderGetQueryParameters{
				Filter:  filterPtr,
				Top:     Int32Ptr(int32(count)),
				Orderby: orderBy,
				Select:  selectFields,
			},
		}
		apiResult, apiErr := client.Users().ByUserId(mailbox).Messages().Get(ctx, requestConfig)
		if apiErr != nil {
			return nil, apiErr
		}
		return apiResult.GetValue(), nil
	})

	if err != nil {
		enrichedErr := enrichGraphAPIError(err, logger, "exportMessages")
		return fmt.Errorf("error searching messages for %s: %w", mailbox, enrichedErr)
	}

	messageCount := len(messages)

	logVerbose(config.VerboseMode, "API response received: %d messages", messageCount)

	if messageCount == 0 {
		if config.OutputFormat == "json" {
			printJSON([]interface{}{}) // Empty array
		} else {
			fmt.Println("No messages found matching the given criteria.")
		}
		if logger != nil {
			_ = logger.WriteRow([]string{ActionExportMessages, StatusSuccess, mailbox, "No messages found (0 messages)", "", ""})
		}
		return nil
	}

	// Print JSON output if requested
	if config.OutputFormat == "json" {
		printJSON(formatMessagesOutput(messages))
	}

	// Create export directory
	exportDir, err := export.CreateExportDir(config.ExportDir)
	if err != nil {
		return err
	}

	if config.OutputFormat != "json" {
		fmt.Printf("Export directory: %s\n", exportDir)
	}

	successCount := 0
	for _, message := range messages {
		messageID := derefOr(message.GetId(), "")
		filePath, err := exportMessageToEML(ctx, client, mailbox, message, exportDir, config)
		if err != nil {
			log.Printf("Error exporting message ID %s: %v", messageID, err)
			if logger != nil {
				_ = logger.WriteRow([]string{ActionExportMessages, StatusError, mailbox, err.Error(), messageID, ""})
			}
			continue
		}

		successCount++
		if config.OutputFormat != "json" {
			fmt.Printf("Successfully exported message: %s -> %s\n", derefOr(message.GetSubject(), "(no subject)"), filePath)
		}
		if logger != nil {
			_ = logger.WriteRow([]string{ActionExportMessages, StatusSuccess, mailbox, "Exported successfully", messageID, filePath})
		}
	}

	if config.OutputFormat != "json" {
		fmt.Printf("Successfully exported %d/%d messages.\n", successCount, messageCount)
	}

	return nil
}

// folderPathSegment renders the Graph URL path segment used for message
// queries, for verbose logging only.
func folderPathSegment(folder string) string {
	if folder == "" {
		return "messages"
	}
	return fmt.Sprintf("mailFolders/%s/messages", folder)
}

// exportMessageToEML fetches a message's raw RFC822/MIME content via the
// Graph API "$value" endpoint and writes it to a .eml file in dir. It returns
// the path to the written file.
func exportMessageToEML(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox string, message models.Messageable, dir string, config *Config) (string, error) {
	if message.GetId() == nil {
		return "", fmt.Errorf("message has no ID")
	}
	id := *message.GetId()

	var mimeContent []byte
	err := retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
		content, apiErr := client.Users().ByUserId(mailbox).Messages().ByMessageId(id).Content().Get(ctx, nil)
		if apiErr == nil {
			mimeContent = content
		}
		return apiErr
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch message content: %w", err)
	}

	// Prefer the Internet Message-ID for the filename when available, since
	// it's more useful for cross-referencing than the Graph-internal ID.
	name := id
	if message.GetInternetMessageId() != nil && *message.GetInternetMessageId() != "" {
		name = *message.GetInternetMessageId()
	}


	filename := fmt.Sprintf("msg_%s.eml", export.SanitizeFilename(name))
	filePath := filepath.Join(dir, filename)

	if err := os.WriteFile(filePath, mimeContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write EML file: %w", err)
	}

	logVerbose(config.VerboseMode, "Exported message to %s", filePath)
	return filePath, nil
}

// exportMessageToJSON serializes a message to JSON and saves it to a file
func exportMessageToJSON(message models.Messageable, dir string, config *Config) error {
	// Extract basic info for filename
	id := "unknown_id"
	if message.GetId() != nil {
		id = *message.GetId()
	}

	// Create a simplified structure for export to ensure clean JSON
	exportData := make(map[string]interface{})

	if message.GetId() != nil {
		exportData["id"] = *message.GetId()
	}
	if message.GetInternetMessageId() != nil {
		exportData["internetMessageId"] = *message.GetInternetMessageId()
	}
	if message.GetSubject() != nil {
		exportData["subject"] = *message.GetSubject()
	}
	if message.GetReceivedDateTime() != nil {
		exportData["receivedDateTime"] = message.GetReceivedDateTime().Format(time.RFC3339)
	}

	// From
	if message.GetFrom() != nil && message.GetFrom().GetEmailAddress() != nil {
		exportData["from"] = extractEmailAddress(message.GetFrom().GetEmailAddress())
	}

	// Recipients
	if message.GetToRecipients() != nil {
		exportData["to"] = extractRecipients(message.GetToRecipients())
	}
	if message.GetCcRecipients() != nil {
		exportData["cc"] = extractRecipients(message.GetCcRecipients())
	}
	if message.GetBccRecipients() != nil {
		exportData["bcc"] = extractRecipients(message.GetBccRecipients())
	}

	// Body
	if message.GetBody() != nil {
		bodyData := make(map[string]string)
		if message.GetBody().GetContentType() != nil {
			bodyData["contentType"] = message.GetBody().GetContentType().String()
		}
		if message.GetBody().GetContent() != nil {
			bodyData["content"] = *message.GetBody().GetContent()
		}
		exportData["body"] = bodyData
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// Sanitize filename
	filename := fmt.Sprintf("msg_%s.json", export.SanitizeFilename(id))
	filePath := filepath.Join(dir, filename)

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	logVerbose(config.VerboseMode, "Exported message to %s", filePath)
	return nil
}

// extractEmailAddress helper
func extractEmailAddress(addr models.EmailAddressable) map[string]string {
	res := make(map[string]string)
	if addr.GetName() != nil {
		res["name"] = *addr.GetName()
	}
	if addr.GetAddress() != nil {
		res["address"] = *addr.GetAddress()
	}
	return res
}

// extractRecipients helper
func extractRecipients(recipients []models.Recipientable) []map[string]string {
	var res []map[string]string
	for _, r := range recipients {
		if r.GetEmailAddress() != nil {
			res = append(res, extractEmailAddress(r.GetEmailAddress()))
		}
	}
	return res
}

// interpretAvailability converts Microsoft Graph availability view codes to human-readable status.
func interpretAvailability(view string) string {
	if len(view) == 0 {
		return "Unknown (empty response)"
	}

	// Get the first character (representing the time slot status)
	code := string(view[0])

	switch code {
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
		return fmt.Sprintf("Unknown (%s)", code)
	}
}

// createFileAttachments reads files and creates Graph API attachment objects.
// Files in inlinePaths are embedded inline (IsInline=true, ContentId set to
// their filename) so they can be referenced from HTML bodies via "cid:<filename>".
func createFileAttachments(filePaths, inlinePaths []string, config *Config) ([]models.Attachmentable, error) {
	onSkip := func(path string, err error) {
		log.Printf("Warning: Could not read attachment file %s: %v", path, err)
	}

	regular, err := email.LoadAttachments(filePaths, onSkip)
	if err != nil {
		return nil, err
	}
	inline, err := email.LoadInlineAttachments(inlinePaths, onSkip)
	if err != nil {
		return nil, err
	}

	var attachments []models.Attachmentable
	for _, att := range append(regular, inline...) {
		attachment := models.NewFileAttachment()

		// Set the OData type for file attachment
		odataType := "#microsoft.graph.fileAttachment"
		attachment.SetOdataType(&odataType)

		name := att.Name
		attachment.SetName(&name)

		contentType := att.ContentType
		attachment.SetContentType(&contentType)

		// Set content as base64 encoded bytes
		attachment.SetContentBytes(att.Data)

		if att.Inline {
			isInline := true
			attachment.SetIsInline(&isInline)
			contentID := att.ContentID
			attachment.SetContentId(&contentID)
		}

		logVerbose(config.VerboseMode, "Attachment: %s (%s, %d bytes, inline=%v)", att.Name, att.ContentType, len(att.Data), att.Inline)
		attachments = append(attachments, attachment)
	}

	return attachments, nil
}

func createRecipients(emails []string) []models.Recipientable {
	recipients := make([]models.Recipientable, len(emails))
	for i, email := range emails {
		recipient := models.NewRecipient()
		emailAddress := models.NewEmailAddress()
		// Need to create a new variable for the address pointer
		address := email
		emailAddress.SetAddress(&address)
		recipient.SetEmailAddress(emailAddress)
		recipients[i] = recipient
	}
	return recipients
}

// parseFlexibleTime parses a time string accepting multiple formats
func parseFlexibleTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("time string is empty")
	}

	// Try RFC3339 first (with timezone)
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}

	// Try PowerShell sortable format (without timezone) - assume UTC
	t, err = time.Parse("2006-01-02T15:04:05", timeStr)
	if err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339 like '2026-01-15T14:00:00Z' or PowerShell sortable like '2026-01-15T14:00:00')")
}

// addWorkingDays adds a specified number of working days (Monday-Friday) to the given time.
func addWorkingDays(t time.Time, days int) time.Time {
	if days <= 0 {
		return t
	}

	result := t
	daysAdded := 0

	for daysAdded < days {
		result = result.Add(24 * time.Hour)

		// Check if this is a working day (Monday=1, Friday=5)
		weekday := result.Weekday()
		if weekday != time.Saturday && weekday != time.Sunday {
			daysAdded++
		}
	}

	return result
}

// Output helper functions

// printJSON marshals the data to JSON and prints it to stdout
func printJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON output: %v\n", err)
	}
}

// formatEventsOutput converts a list of Eventable items to a JSON-friendly slice of maps
func formatEventsOutput(events []models.Eventable) []map[string]interface{} {
	var output []map[string]interface{}
	for _, event := range events {
		eventMap := make(map[string]interface{})
		if event.GetId() != nil {
			eventMap["id"] = *event.GetId()
		}
		if event.GetSubject() != nil {
			eventMap["subject"] = *event.GetSubject()
		}
		if event.GetStart() != nil && event.GetStart().GetDateTime() != nil {
			eventMap["start"] = *event.GetStart().GetDateTime()
		}
		if event.GetEnd() != nil && event.GetEnd().GetDateTime() != nil {
			eventMap["end"] = *event.GetEnd().GetDateTime()
		}
		if event.GetOrganizer() != nil && event.GetOrganizer().GetEmailAddress() != nil {
			eventMap["organizer"] = extractEmailAddress(event.GetOrganizer().GetEmailAddress())
		}
		output = append(output, eventMap)
	}
	return output
}

// formatMessagesOutput converts a list of Messageable items to a JSON-friendly slice of maps
func formatMessagesOutput(messages []models.Messageable) []map[string]interface{} {
	var output []map[string]interface{}
	for _, message := range messages {
		msgMap := make(map[string]interface{})
		if message.GetId() != nil {
			msgMap["id"] = *message.GetId()
		}
		if message.GetSubject() != nil {
			msgMap["subject"] = *message.GetSubject()
		}
		if message.GetReceivedDateTime() != nil {
			msgMap["receivedDateTime"] = message.GetReceivedDateTime().Format(time.RFC3339)
		}
		if message.GetFrom() != nil && message.GetFrom().GetEmailAddress() != nil {
			msgMap["from"] = extractEmailAddress(message.GetFrom().GetEmailAddress())
		}
		if message.GetToRecipients() != nil {
			msgMap["toRecipients"] = extractRecipients(message.GetToRecipients())
		}
		output = append(output, msgMap)
	}
	return output
}

// formatScheduleOutput converts a list of ScheduleInformationable items to a JSON-friendly structure
func formatScheduleOutput(schedules []models.ScheduleInformationable) []map[string]interface{} {
	var output []map[string]interface{}
	for _, schedule := range schedules {
		schMap := make(map[string]interface{})
		if schedule.GetScheduleId() != nil {
			schMap["scheduleId"] = *schedule.GetScheduleId()
		}
		if schedule.GetAvailabilityView() != nil {
			schMap["availabilityView"] = *schedule.GetAvailabilityView()
			schMap["availabilityStatus"] = interpretAvailability(*schedule.GetAvailabilityView())
		}
		// Include working hours if available
		if schedule.GetWorkingHours() != nil {
			wh := schedule.GetWorkingHours()
			whMap := make(map[string]interface{})
			if wh.GetStartTime() != nil {
				whMap["startTime"] = *wh.GetStartTime()
			}
			if wh.GetEndTime() != nil {
				whMap["endTime"] = *wh.GetEndTime()
			}
			schMap["workingHours"] = whMap
		}
		output = append(output, schMap)
	}
	return output
}
