package main

import (
	"context"
	"fmt"
	"log/slog"

	"gomailtesttool/internal/common/logger"
	"gomailtesttool/internal/jmap/protocol"
)

// testAuth tests JMAP authentication.
func testAuth(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	discoveryURL := protocol.DiscoveryURL(config.Host)
	fmt.Printf("Testing JMAP authentication to %s...\n", config.Host)
	fmt.Printf("Discovery URL: %s\n", discoveryURL)

	// CSV columns for testauth
	columns := []string{"Action", "Status", "Server", "Port", "Username", "Auth_Method", "API_URL", "Accounts", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewJMAPClient(config)
	authMethod := client.GetAuthMethod()

	fmt.Printf("Username: %s\n", config.Username)
	fmt.Printf("Auth method: %s\n", authMethod)

	// Try to authenticate by discovering session with credentials
	session, err := client.Discover(ctx)
	if err != nil {
		logger.LogError(slogLogger, "JMAP authentication failed",
			"error", err,
			"host", config.Host,
			"username", maskUsername(config.Username),
			"auth_method", authMethod)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			maskUsername(config.Username), authMethod, "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("JMAP authentication failed: %w", err)
	}

	fmt.Println("✓ Authentication successful")
	fmt.Printf("\nSession Information:\n")
	fmt.Printf("  API URL:      %s\n", session.APIURL)
	fmt.Printf("  Username:     %s\n", session.Username)
	fmt.Printf("  Accounts:     %d\n", session.GetAccountCount())

	// Display capabilities
	caps := session.GetCapabilityNames()
	fmt.Printf("  Capabilities: %d\n", len(caps))

	// Check for mail capability
	if session.HasMailCapability() {
		fmt.Println("  ✓ Mail capability supported")
	}
	if session.HasSubmissionCapability() {
		fmt.Println("  ✓ Submission capability supported")
	}

	// Display accounts
	if session.GetAccountCount() > 0 {
		fmt.Printf("\nAccounts:\n")
		for id, account := range session.Accounts {
			fmt.Printf("  %s: %s", id, account.Name)
			if account.IsPersonal {
				fmt.Printf(" (personal)")
			}
			if account.IsReadOnly {
				fmt.Printf(" (read-only)")
			}
			fmt.Println()
		}
	}

	// Get primary mail account
	if primaryId, ok := session.GetPrimaryMailAccountId(); ok {
		fmt.Printf("\nPrimary mail account: %s\n", primaryId)
	}

	// Log success to CSV
	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		maskUsername(config.Username), authMethod, session.APIURL,
		fmt.Sprintf("%d", session.GetAccountCount()), "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	logger.LogInfo(slogLogger, "JMAP authentication test completed",
		"host", config.Host,
		"username", maskUsername(config.Username),
		"auth_method", authMethod,
		"accounts", session.GetAccountCount(),
		"has_mail", session.HasMailCapability())

	fmt.Println("\n✓ JMAP authentication test completed")
	return nil
}
