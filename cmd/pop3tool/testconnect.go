package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gomailtesttool/internal/common/logger"
)

// testConnect tests basic POP3 connectivity.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Testing POP3 connection to %s:%d...\n", config.Host, config.Port)

	// CSV columns for testconnect
	columns := []string{"Action", "Status", "Server", "Port", "Connected", "Greeting", "Capabilities", "TLS_Version", "Error"}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader(columns); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	client := NewPOP3Client(config)

	// Connect to server
	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed",
			"error", err,
			"host", config.Host,
			"port", config.Port)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			"false", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Quit() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)
	fmt.Printf("  Greeting: %s\n", client.GetGreeting())

	// Get TLS info if connected via POP3S
	tlsVersion := ""
	if state := client.GetTLSState(); state != nil {
		tlsVersion = getTLSVersionString(state.Version)
		fmt.Printf("  TLS: %s\n", tlsVersion)
	}

	// Try STLS if not already using TLS and STARTTLS is requested
	if config.StartTLS && client.GetTLSState() == nil {
		fmt.Println("Attempting STLS upgrade...")
		if err := client.StartTLS(nil); err != nil {
			logger.LogWarn(slogLogger, "STLS upgrade failed", "error", err)
			fmt.Printf("  ✗ STLS failed: %v\n", err)
		} else {
			if state := client.GetTLSState(); state != nil {
				tlsVersion = getTLSVersionString(state.Version)
				fmt.Printf("  ✓ STLS upgrade successful (TLS %s)\n", tlsVersion)
			}
		}
	}

	// Get capabilities
	caps, err := client.Capabilities(ctx)
	capsStr := ""
	if err != nil {
		logger.LogWarn(slogLogger, "CAPA command failed", "error", err)
		fmt.Printf("  CAPA: not supported or failed\n")
	} else {
		capsStr = caps.String()
		fmt.Printf("  Capabilities: %s\n", capsStr)

		// Show interesting capabilities
		if caps.SupportsSTLS() {
			fmt.Println("    - STLS (STARTTLS) supported")
		}
		if caps.SupportsUIDL() {
			fmt.Println("    - UIDL supported")
		}
		if caps.SupportsTOP() {
			fmt.Println("    - TOP supported")
		}
		if caps.SupportsUSER() {
			fmt.Println("    - USER/PASS supported")
		}
		if mechanisms := caps.GetAuthMechanisms(); len(mechanisms) > 0 {
			fmt.Printf("    - SASL mechanisms: %s\n", strings.Join(mechanisms, ", "))
		}
		if impl := caps.GetImplementation(); impl != "" {
			fmt.Printf("    - Implementation: %s\n", impl)
		}
	}

	logger.LogInfo(slogLogger, "Connection test successful",
		"host", config.Host,
		"port", config.Port,
		"greeting", client.GetGreeting(),
		"capabilities", capsStr)

	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		"true", client.GetGreeting(), capsStr, tlsVersion, "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	fmt.Println("\n✓ Connection test successful")
	return nil
}

// getTLSVersionString converts TLS version constant to string.
func getTLSVersionString(version uint16) string {
	switch version {
	case 0x0304:
		return "1.3"
	case 0x0303:
		return "1.2"
	case 0x0302:
		return "1.1"
	case 0x0301:
		return "1.0"
	default:
		return fmt.Sprintf("unknown (0x%04x)", version)
	}
}
