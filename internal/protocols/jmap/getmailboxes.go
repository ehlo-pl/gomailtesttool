package jmap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// getMailboxes retrieves and displays the list of mailboxes.
func getMailboxes(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	discoveryURL := protocol.DiscoveryURL(config.Host)
	fmt.Printf("Getting mailboxes from %s...\n", config.Host)
	fmt.Printf("Discovery URL: %s\n", discoveryURL)

	// CSV columns for getmailboxes
	columns := []string{"Action", "Status", "Server", "Mailbox_Id", "Mailbox_Name", "Role", "Total_Emails", "Unread_Emails", "Parent_Id", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewJMAPClient(config)

	// First discover the session
	session, err := client.Discover(ctx)
	if err != nil {
		logger.LogError(slogLogger, "JMAP discovery failed",
			"error", err,
			"host", config.Host)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, "", "", "", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("JMAP discovery failed: %w", err)
	}

	fmt.Println("✓ Session discovered")
	fmt.Printf("  API URL: %s\n", session.APIURL)

	// Get mailboxes
	mailboxes, err := client.GetMailboxes(ctx)
	if err != nil {
		logger.LogError(slogLogger, "Failed to get mailboxes",
			"error", err,
			"host", config.Host)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, "", "", "", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("failed to get mailboxes: %w", err)
	}

	fmt.Printf("\nFound %d mailboxes:\n", len(mailboxes))
	fmt.Println("  Name                              Role            Total   Unread")
	fmt.Println("  ----                              ----            -----   ------")

	for _, mb := range mailboxes {
		role := "-"
		if mb.Role != nil && *mb.Role != "" {
			role = *mb.Role
		}
		fmt.Printf("  %-34s %-14s %6d   %6d\n", mb.Name, role, mb.TotalEmails, mb.UnreadEmails)

		// Log each mailbox to CSV
		parentId := ""
		if mb.ParentId != nil {
			parentId = string(*mb.ParentId)
		}
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host,
			string(mb.Id), mb.Name, role,
			fmt.Sprintf("%d", mb.TotalEmails), fmt.Sprintf("%d", mb.UnreadEmails),
			parentId, "",
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	logger.LogInfo(slogLogger, "Get mailboxes completed",
		"host", config.Host,
		"mailbox_count", len(mailboxes))

	fmt.Println("\n✓ Get mailboxes completed")
	return nil
}
