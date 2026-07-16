package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// listMail queries the inbox and displays recent messages.
func listMail(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing messages from %s inbox...\n\n", config.Host)

	columns := []string{"Action", "Status", "Server", "Subject", "From", "ReceivedAt", "Preview", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewJMAPClient(config)

	if _, err := client.Discover(ctx); err != nil {
		logger.LogError(slogLogger, "JMAP discovery failed", "error", err)
		_ = csvLogger.WriteRow([]string{config.Action, "FAILURE", config.Host, "", "", "", "", err.Error()})
		return fmt.Errorf("JMAP discovery failed: %w", err)
	}

	emails, err := client.QueryInboxEmails(ctx, uint32(config.Count))
	if err != nil {
		logger.LogError(slogLogger, "Failed to query inbox emails", "error", err)
		_ = csvLogger.WriteRow([]string{config.Action, "FAILURE", config.Host, "", "", "", "", err.Error()})
		return fmt.Errorf("failed to list mail: %w", err)
	}

	if len(emails) == 0 {
		fmt.Println("No messages found in inbox.")
		_ = csvLogger.WriteRow([]string{config.Action, "SUCCESS", config.Host, "No messages found", "", "", "", ""})
		return nil
	}

	fmt.Printf("Inbox messages (%d):\n\n", len(emails))
	for i, email := range emails {
		from := "N/A"
		if len(email.From) > 0 {
			if email.From[0].Name != "" {
				from = fmt.Sprintf("%s <%s>", email.From[0].Name, email.From[0].Email)
			} else {
				from = email.From[0].Email
			}
		}
		subject := email.Subject
		if subject == "" {
			subject = "(no subject)"
		}
		preview := strings.TrimSpace(email.Preview)
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}

		fmt.Printf("%d. %s\n   From: %s\n   Received: %s\n   Preview: %s\n\n",
			i+1, subject, from, email.ReceivedAt, preview)

		_ = csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host,
			subject, from, email.ReceivedAt, preview, "",
		})
	}

	logger.LogInfo(slogLogger, "listmail completed", "host", config.Host, "count", len(emails))
	return nil
}
