package pop3

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// testConnect tests basic POP3 connectivity.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	logger.Tprintf("Testing POP3 connection to %s:%d...\n", config.Host, config.Port)

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

	logger.Tprintf("✓ Connected to %s:%d\n", config.Host, config.Port)
	logger.Tprintf("  Greeting: %s\n", client.GetGreeting())

	// Get TLS info if connected via POP3S
	tlsVersion := ""
	if state := client.GetTLSState(); state != nil {
		tlsVersion = getTLSVersionString(state.Version)
		logger.Tprintf("  TLS: %s\n", tlsVersion)
	}

	// Try STLS if not already using TLS and STARTTLS is requested
	if config.StartTLS && client.GetTLSState() == nil {
		logger.Tprintln("Attempting STLS upgrade...")
		if err := client.StartTLS(nil); err != nil {
			logger.LogWarn(slogLogger, "STLS upgrade failed", "error", err)
			logger.Tprintf("  ✗ STLS failed: %v\n", err)
		} else {
			if state := client.GetTLSState(); state != nil {
				tlsVersion = getTLSVersionString(state.Version)
				logger.Tprintf("  ✓ STLS upgrade successful (TLS %s)\n", tlsVersion)
			}
		}
	}

	// Get capabilities
	caps, err := client.Capabilities(ctx)
	capsStr := ""
	if err != nil {
		logger.LogWarn(slogLogger, "CAPA command failed", "error", err)
		logger.Tprintf("  CAPA: not supported or failed\n")
	} else {
		capsStr = caps.String()
		logger.Tprintf("  Capabilities: %s\n", capsStr)

		// Show interesting capabilities
		if caps.SupportsSTLS() {
			logger.Tprintln("    - STLS (STARTTLS) supported")
		}
		if caps.SupportsUIDL() {
			logger.Tprintln("    - UIDL supported")
		}
		if caps.SupportsTOP() {
			logger.Tprintln("    - TOP supported")
		}
		if caps.SupportsUSER() {
			logger.Tprintln("    - USER/PASS supported")
		}
		if mechanisms := caps.GetAuthMechanisms(); len(mechanisms) > 0 {
			logger.Tprintf("    - SASL mechanisms: %s\n", strings.Join(mechanisms, ", "))
		}
		if impl := caps.GetImplementation(); impl != "" {
			logger.Tprintf("    - Implementation: %s\n", impl)
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

	fmt.Println()
	logger.Tprintln("✓ Connection test successful")
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
