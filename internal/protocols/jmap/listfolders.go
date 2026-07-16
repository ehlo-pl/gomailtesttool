package jmap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// listFolders lists JMAP mailboxes. It shares the underlying GetMailboxes call
// with getmailboxes but uses the ActionListFolders constant for logging.
func listFolders(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing JMAP folders from %s...\n\n", config.Host)

	columns := []string{"Action", "Status", "Server", "Mailbox_Id", "Mailbox_Name", "Role", "Total_Emails", "Unread_Emails", "Parent_Id", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewJMAPClient(config)

	session, err := client.Discover(ctx)
	if err != nil {
		logger.LogError(slogLogger, "JMAP discovery failed", "error", err)
		_ = csvLogger.WriteRow([]string{config.Action, "FAILURE", config.Host, "", "", "", "", "", "", err.Error()})
		return fmt.Errorf("JMAP discovery failed: %w", err)
	}

	logger.LogDebug(slogLogger, "Session discovered", "api_url", session.APIURL)

	mailboxes, err := client.GetMailboxes(ctx)
	if err != nil {
		logger.LogError(slogLogger, "Failed to list folders", "error", err)
		_ = csvLogger.WriteRow([]string{config.Action, "FAILURE", config.Host, "", "", "", "", "", "", err.Error()})
		return fmt.Errorf("failed to list folders: %w", err)
	}

	fmt.Printf("Folders (%d):\n", len(mailboxes))
	fmt.Println("  Name                              Role            Total   Unread")
	fmt.Println("  ----                              ----            -----   ------")

	for _, mb := range mailboxes {
		role := "-"
		if mb.Role != nil && *mb.Role != "" {
			role = *mb.Role
		}
		fmt.Printf("  %-34s %-14s %6d   %6d\n", mb.Name, role, mb.TotalEmails, mb.UnreadEmails)

		parentId := ""
		if mb.ParentId != nil {
			parentId = string(*mb.ParentId)
		}
		_ = csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host,
			string(mb.Id), mb.Name, role,
			fmt.Sprintf("%d", mb.TotalEmails), fmt.Sprintf("%d", mb.UnreadEmails),
			parentId, "",
		})
	}

	logger.LogInfo(slogLogger, "listfolders completed", "host", config.Host, "count", len(mailboxes))
	fmt.Printf("\n✓ Listed %d folders\n", len(mailboxes))
	return nil
}

// findMailboxByRole returns the first mailbox with the given role, or nil.
func findMailboxByRole(mailboxes []protocol.Mailbox, role string) *protocol.Mailbox {
	for i := range mailboxes {
		if mailboxes[i].Role != nil && *mailboxes[i].Role == role {
			return &mailboxes[i]
		}
	}
	return nil
}
