package smtp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/smtp/exchange"
)

// testConnect performs basic SMTP connectivity and capability testing.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.SMTPS {
		if config.ConnectAddress != "" {
			logger.Tprintf("Testing SMTPS connectivity to %s:%d (connecting via %s)...\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			logger.Tprintf("Testing SMTPS connectivity to %s:%d...\n", config.Host, config.Port)
		}
	} else {
		if config.ConnectAddress != "" {
			logger.Tprintf("Testing SMTP connectivity to %s:%d (connecting via %s)...\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			logger.Tprintf("Testing SMTP connectivity to %s:%d...\n", config.Host, config.Port)
		}
	}
	fmt.Println()

	// Write CSV header
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Connect_Address", "Connected", "Banner", "Capabilities", "Exchange_Detected",
			"TLS_Version", "Cipher_Suite", "Cipher_Strength",
			"Cert_Subject", "Cert_Issuer", "Cert_SANs",
			"Cert_Valid_From", "Cert_Valid_To", "Cert_Verification_Status",
			"Error",
		}); err != nil {
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
			fmt.Sprintf("%d", config.Port), config.ConnectAddress, "false", "", "", "false",
			"", "", "", "", "", "", "", "", "", // No TLS info on connection failure
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}
	defer func() { _ = client.Close() }()

	if config.SMTPS {
		if config.ConnectAddress != "" {
			logger.Tprintf("✓ Connected successfully with SMTPS (implicit TLS) via %s\n", config.ConnectAddress)
		} else {
			logger.Tprintf("✓ Connected successfully with SMTPS (implicit TLS)\n")
		}
	} else {
		if config.ConnectAddress != "" {
			logger.Tprintf("✓ Connected successfully via %s\n", config.ConnectAddress)
		} else {
			logger.Tprintf("✓ Connected successfully\n")
		}
	}
	logger.Tprintf("  Banner: %s\n", client.GetBanner())
	fmt.Println()

	// Get TLS state for SMTPS connections
	var tlsState = client.GetTLSState()

	// Display TLS information for SMTPS in verbose mode
	if config.SMTPS && config.VerboseMode && tlsState != nil {
		displayComprehensiveTLSInfo(tlsState, config.Host, config.VerboseMode)
	}

	// Send EHLO
	logger.LogDebug(slogLogger, "Sending EHLO command")
	caps, err := client.EHLO("smtptool.local")
	if err != nil {
		logger.LogError(slogLogger, "EHLO failed", "error", err)
		tlsData := formatTLSInfoForCSV(tlsState, config.Host)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host,
			fmt.Sprintf("%d", config.Port), config.ConnectAddress, "true", client.GetBanner(), "", "false",
			tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
			tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
			tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}

	// Display capabilities
	logger.Tprintln("Server Capabilities:")
	for cap, params := range caps {
		if len(params) > 0 {
			logger.Tprintf("  • %s: %s\n", cap, strings.Join(params, ", "))
		} else {
			logger.Tprintf("  • %s\n", cap)
		}
	}
	fmt.Println()

	// Detect Exchange
	exchangeInfo := exchange.DetectExchange(client.GetBanner(), caps)
	if exchangeInfo.IsExchange {
		logger.Tprintf("%s", exchange.FormatExchangeInfo(exchangeInfo, caps))
	}

	// Log to CSV
	capsStr := caps.String()
	tlsData := formatTLSInfoForCSV(tlsState, config.Host)
	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host,
		fmt.Sprintf("%d", config.Port), config.ConnectAddress, "true", client.GetBanner(),
		capsStr, fmt.Sprintf("%t", exchangeInfo.IsExchange),
		tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
		tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
		tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
		"",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	if config.SMTPS {
		logger.Tprintln("✓ SMTPS connectivity test completed successfully")
	} else {
		logger.Tprintln("✓ Connectivity test completed successfully")
	}
	logger.LogInfo(slogLogger, "testconnect completed successfully")

	return nil
}
