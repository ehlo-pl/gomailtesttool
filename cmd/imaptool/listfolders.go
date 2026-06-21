package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gomailtesttool/internal/common/logger"
)

// listFolders lists all mailbox folders.
func listFolders(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Listing folders on %s:%d...\n", config.Host, config.Port)

	// CSV columns for listfolders
	columns := []string{"Action", "Status", "Server", "Port", "Folder_Name", "Attributes", "Total_Messages", "Unseen", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewIMAPClient(config)

	// Connect to server
	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed",
			"error", err,
			"host", config.Host,
			"port", config.Port)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Logout() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	// Get capabilities
	caps := client.GetCapabilities()

	// Determine auth method
	authMethod := config.AuthMethod
	if strings.EqualFold(authMethod, "auto") {
		if config.AccessToken != "" {
			if caps != nil && caps.SupportsXOAUTH2() {
				authMethod = "XOAUTH2"
			} else {
				authMethod = "PLAIN"
			}
		} else if caps != nil && caps.SupportsPlain() {
			authMethod = "PLAIN"
		} else {
			authMethod = "LOGIN"
		}
	}

	fmt.Printf("Authenticating with method: %s\n", authMethod)

	// Authenticate
	var authErr error
	if config.AccessToken != "" && strings.EqualFold(authMethod, "XOAUTH2") {
		authErr = client.Auth(ctx, config.Username, "", config.AccessToken)
	} else {
		authErr = client.Auth(ctx, config.Username, config.Password, "")
	}

	if authErr != nil {
		logger.LogError(slogLogger, "Authentication failed",
			"error", authErr,
			"username", maskUsername(config.Username))

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"", "", "", "", fmt.Sprintf("Auth failed: %v", authErr),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("authentication failed: %w", authErr)
	}
	fmt.Println("✓ Authentication successful")

	// List mailboxes
	fmt.Println("\nListing mailboxes...")
	mailboxes, err := client.ListMailboxes(ctx)
	if err != nil {
		logger.LogError(slogLogger, "LIST command failed", "error", err)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"", "", "", "", fmt.Sprintf("LIST failed: %v", err),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("LIST failed: %w", err)
	}

	fmt.Printf("\nFound %d mailboxes:\n", len(mailboxes))
	fmt.Println("  Name                              Messages  Unseen  Attributes")
	fmt.Println("  ----                              --------  ------  ----------")

	for _, mb := range mailboxes {
		attrs := strings.Join(mb.Attributes, ", ")
		fmt.Printf("  %-34s %8d  %6d  %s\n", mb.Name, mb.Messages, mb.Unseen, attrs)

		// Log each mailbox to CSV
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
			mb.Name, attrs, fmt.Sprintf("%d", mb.Messages), fmt.Sprintf("%d", mb.Unseen), "",
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	logger.LogInfo(slogLogger, "List folders completed",
		"host", config.Host,
		"mailbox_count", len(mailboxes))

	fmt.Println("\n✓ List folders completed")
	return nil
}
