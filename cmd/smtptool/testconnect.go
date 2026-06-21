package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gomailtesttool/internal/common/logger"
	"gomailtesttool/internal/smtp/exchange"
)

// testConnect performs basic SMTP connectivity and capability testing.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.SMTPS {
		fmt.Printf("Testing SMTPS connectivity to %s:%d...\n\n", config.Host, config.Port)
	} else {
		fmt.Printf("Testing SMTP connectivity to %s:%d...\n\n", config.Host, config.Port)
	}

	// Write CSV header
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{"Action", "Status", "Server", "Port", "Connected", "Banner", "Capabilities", "Exchange_Detected", "Error"}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	// Create client
	client := NewSMTPClient(config.Host, config.Port, config)

	// Connect
	logger.LogDebug(slogLogger, "Connecting to SMTP server", "host", config.Host, "port", config.Port)
	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host,
			fmt.Sprintf("%d", config.Port), "false", "", "", "false", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}
	defer client.Close()

	if config.SMTPS {
		fmt.Printf("✓ Connected successfully with SMTPS (implicit TLS)\n")
	} else {
		fmt.Printf("✓ Connected successfully\n")
	}
	fmt.Printf("  Banner: %s\n\n", client.GetBanner())

	// Send EHLO
	logger.LogDebug(slogLogger, "Sending EHLO command")
	caps, err := client.EHLO("smtptool.local")
	if err != nil {
		logger.LogError(slogLogger, "EHLO failed", "error", err)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host,
			fmt.Sprintf("%d", config.Port), "true", client.GetBanner(), "", "false", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}

	// Display capabilities
	fmt.Println("Server Capabilities:")
	for cap, params := range caps {
		if len(params) > 0 {
			fmt.Printf("  • %s: %s\n", cap, strings.Join(params, ", "))
		} else {
			fmt.Printf("  • %s\n", cap)
		}
	}
	fmt.Println()

	// Detect Exchange
	exchangeInfo := exchange.DetectExchange(client.GetBanner(), caps)
	if exchangeInfo.IsExchange {
		fmt.Print(exchange.FormatExchangeInfo(exchangeInfo, caps))
	}

	// Log to CSV
	capsStr := caps.String()
	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host,
		fmt.Sprintf("%d", config.Port), "true", client.GetBanner(),
		capsStr, fmt.Sprintf("%t", exchangeInfo.IsExchange), "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	if config.SMTPS {
		fmt.Println("✓ SMTPS connectivity test completed successfully")
	} else {
		fmt.Println("✓ Connectivity test completed successfully")
	}
	logger.LogInfo(slogLogger, "testconnect completed successfully")

	return nil
}
