package msgraph

import (
	"context"
	"fmt"
	"log"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// respondMeeting sends an accept, decline, or tentative response to a calendar
// event via the Microsoft Graph API. The response is sent to the organizer when
// sendResponse is true (the default).
func respondMeeting(ctx context.Context, client *msgraphsdk.GraphServiceClient, mailbox, eventID, response, comment string, sendResponse bool, config *Config, csvLogger logger.Logger) error {
	logVerbose(config.VerboseMode, "Calling Graph API: POST /users/%s/events/%s/%s", mailbox, eventID, response)

	var err error
	switch response {
	case "accept":
		reqBody := users.NewItemEventsItemAcceptPostRequestBody()
		if comment != "" {
			reqBody.SetComment(&comment)
		}
		reqBody.SetSendResponse(&sendResponse)
		err = retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
			return client.Users().ByUserId(mailbox).Events().ByEventId(eventID).Accept().Post(ctx, reqBody, nil)
		})
	case "decline":
		reqBody := users.NewItemEventsItemDeclinePostRequestBody()
		if comment != "" {
			reqBody.SetComment(&comment)
		}
		reqBody.SetSendResponse(&sendResponse)
		err = retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
			return client.Users().ByUserId(mailbox).Events().ByEventId(eventID).Decline().Post(ctx, reqBody, nil)
		})
	case "tentative":
		reqBody := users.NewItemEventsItemTentativelyAcceptPostRequestBody()
		if comment != "" {
			reqBody.SetComment(&comment)
		}
		reqBody.SetSendResponse(&sendResponse)
		err = retryWithBackoff(ctx, config.MaxRetries, config.RetryDelay, func() error {
			return client.Users().ByUserId(mailbox).Events().ByEventId(eventID).TentativelyAccept().Post(ctx, reqBody, nil)
		})
	}

	status := StatusSuccess
	var returnErr error
	if err != nil {
		enrichedErr := enrichGraphAPIError(err, csvLogger, "respondMeeting")
		log.Printf("Error responding to meeting: %v", enrichedErr)
		status = fmt.Sprintf("%s: %v", StatusError, enrichedErr)
		returnErr = enrichedErr
	} else {
		logVerbose(config.VerboseMode, "Meeting response sent successfully via Graph API")
		fmt.Printf("Meeting response '%s' sent for event %s in mailbox %s.\n", response, eventID, mailbox)
		if comment != "" {
			fmt.Printf("Comment: %s\n", comment)
		}
		if sendResponse {
			fmt.Println("Response sent to organizer.")
		} else {
			fmt.Println("Response saved locally (not sent to organizer).")
		}
	}

	if csvLogger != nil {
		_ = csvLogger.WriteRow([]string{ActionRespondMeeting, status, mailbox, eventID, response})
	}

	return returnErr
}
