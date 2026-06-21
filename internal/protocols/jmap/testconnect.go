package jmap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// testConnect tests JMAP server connectivity by discovering the session.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	discoveryURL := protocol.DiscoveryURL(config.Host)
	fmt.Printf("Testing JMAP connectivity to %s...\n", config.Host)
	fmt.Printf("Discovery URL: %s\n", discoveryURL)

	// CSV columns for testconnect
	columns := []string{"Action", "Status", "Server", "Port", "Discovery_URL", "API_URL", "Capabilities", "Accounts", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewJMAPClient(config)

	// Try to discover the session (without auth for connectivity test)
	session, err := client.Discover(ctx)
	if err != nil {
		logger.LogError(slogLogger, "JMAP discovery failed",
			"error", err,
			"host", config.Host)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			discoveryURL, "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("JMAP discovery failed: %w", err)
	}

	fmt.Println("✓ JMAP session discovered successfully")
	fmt.Printf("\nSession Information:\n")
	fmt.Printf("  API URL:      %s\n", session.APIURL)
	fmt.Printf("  Username:     %s\n", session.Username)
	fmt.Printf("  Accounts:     %d\n", session.GetAccountCount())

	// Display capabilities
	caps := session.GetCapabilityNames()
	fmt.Printf("  Capabilities: %d\n", len(caps))
	for _, cap := range caps {
		fmt.Printf("    - %s\n", cap)
	}

	// Display accounts
	if session.GetAccountCount() > 0 {
		fmt.Printf("\nAccounts:\n")
		for id, account := range session.Accounts {
			fmt.Printf("  %s: %s\n", id, account.Name)
		}
	}

	// Log success to CSV
	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		discoveryURL, session.APIURL, strings.Join(caps, "; "),
		fmt.Sprintf("%d", session.GetAccountCount()), "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	logger.LogInfo(slogLogger, "JMAP connectivity test completed",
		"host", config.Host,
		"api_url", session.APIURL,
		"capabilities", len(caps),
		"accounts", session.GetAccountCount())

	fmt.Println("\n✓ JMAP connectivity test completed")
	return nil
}
