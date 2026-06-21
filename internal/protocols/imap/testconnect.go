package imap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// testConnect tests basic IMAP connectivity.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Testing IMAP connection to %s:%d...\n", config.Host, config.Port)

	// CSV columns for testconnect
	columns := []string{"Action", "Status", "Server", "Port", "Connected", "Capabilities", "TLS_Version", "Error"}
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
			"false", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Logout() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	// Get TLS info if connected via IMAPS or STARTTLS
	tlsVersion := ""
	if state := client.GetTLSState(); state != nil {
		if config.IMAPS {
			tlsVersion = "IMAPS"
		} else if config.StartTLS {
			tlsVersion = "STARTTLS"
		}
		fmt.Printf("  TLS: %s\n", tlsVersion)
	}

	caps := client.GetCapabilities()

	// Display capabilities
	capsStr := ""
	if caps != nil {
		capsStr = caps.String()
		fmt.Printf("  Capabilities: %s\n", capsStr)

		// Show interesting capabilities
		if caps.SupportsIMAP4rev2() {
			fmt.Println("    - IMAP4rev2 supported")
		} else if caps.SupportsIMAP4rev1() {
			fmt.Println("    - IMAP4rev1 supported")
		}
		if caps.SupportsSTARTTLS() {
			fmt.Println("    - STARTTLS supported")
		}
		if caps.SupportsIDLE() {
			fmt.Println("    - IDLE (push notifications) supported")
		}
		if caps.SupportsNAMESPACE() {
			fmt.Println("    - NAMESPACE supported")
		}
		if caps.SupportsQUOTA() {
			fmt.Println("    - QUOTA supported")
		}
		if mechanisms := caps.GetAuthMechanisms(); len(mechanisms) > 0 {
			fmt.Printf("    - Auth mechanisms: %s\n", strings.Join(mechanisms, ", "))
		}
		if caps.IsLoginDisabled() {
			fmt.Println("    - LOGIN disabled (use STARTTLS first)")
		}
	}

	logger.LogInfo(slogLogger, "Connection test successful",
		"host", config.Host,
		"port", config.Port,
		"capabilities", capsStr)

	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		"true", capsStr, tlsVersion, "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	fmt.Println("\n✓ Connection test successful")
	return nil
}
