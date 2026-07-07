package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
)

// testStartTLS performs comprehensive TLS/SSL testing with detailed diagnostics.
// For SMTPS mode, tests implicit TLS (TLS handshake happens immediately after TCP connect).
func testStartTLS(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.SMTPS {
		if config.ConnectAddress != "" {
			fmt.Printf("Testing SMTPS (implicit TLS) on %s:%d (connecting via %s)...\n\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			fmt.Printf("Testing SMTPS (implicit TLS) on %s:%d...\n\n", config.Host, config.Port)
		}
	} else {
		if config.ConnectAddress != "" {
			fmt.Printf("Testing STARTTLS on %s:%d (connecting via %s)...\n\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			fmt.Printf("Testing STARTTLS on %s:%d...\n\n", config.Host, config.Port)
		}
	}

	// Write CSV header
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Connect_Address", "STARTTLS_Available",
			"TLS_Version", "Cipher_Suite", "Cert_Subject", "Cert_Issuer",
			"Cert_Valid_From", "Cert_Valid_To", "Cert_SANs",
			"Verification_Status", "Warnings", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	// Create and connect client
	client := NewSMTPClient(config.Host, config.Port, config)
	logger.LogDebug(slogLogger, "Connecting to SMTP server")

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, "unknown", "", "", "", "", "", "", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}
	defer func() { _ = client.Close() }()

	if config.SMTPS {
		if config.ConnectAddress != "" {
			fmt.Printf("✓ Connected with SMTPS (implicit TLS) via %s\n", config.ConnectAddress)
		} else {
			fmt.Printf("✓ Connected with SMTPS (implicit TLS)\n")
		}
	} else {
		if config.ConnectAddress != "" {
			fmt.Printf("✓ Connected via %s\n", config.ConnectAddress)
		} else {
			fmt.Printf("✓ Connected\n")
		}
	}

	// Send EHLO
	logger.LogDebug(slogLogger, "Sending EHLO command")
	caps, err := client.EHLO("smtptool.local")
	if err != nil {
		logger.LogError(slogLogger, "EHLO failed", "error", err)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, "unknown", "", "", "", "", "", "", "", "", "", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}

	var connState *tls.ConnectionState

	if config.SMTPS {
		// For SMTPS, TLS handshake already happened during Connect()
		connState = client.GetTLSState()
		if connState == nil {
			msg := "SMTPS connection state not available"
			logger.LogError(slogLogger, msg)
			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, "N/A (SMTPS)", "", "", "", "", "", "", "", "", "", msg,
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return errors.New(msg)
		}
		fmt.Printf("✓ SMTPS TLS handshake completed\n\n")
	} else {
		// Check STARTTLS capability
		if !caps.SupportsSTARTTLS() {
			msg := "STARTTLS not advertised by server"
			fmt.Printf("✗ %s\n", msg)
			logger.LogWarn(slogLogger, msg)
			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, "false", "", "", "", "", "", "", "", "", "", msg,
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return errors.New(msg)
		}

		fmt.Printf("✓ STARTTLS capability available\n\n")

		// Perform STARTTLS handshake
		fmt.Println("Performing TLS handshake...")
		tlsVersion := tlsutil.ParseTLSVersion(config.TLSVersion)
		tlsConfig := &tls.Config{
			ServerName:         client.GetHost(), // resolved MX hostname if --use-mx, otherwise --host
			InsecureSkipVerify: config.SkipVerify,
			MinVersion:         tlsVersion,
			MaxVersion:         tlsVersion, // Force exact TLS version
		}

		logger.LogDebug(slogLogger, "Starting TLS handshake",
			"skipVerify", config.SkipVerify,
			"tlsVersion", config.TLSVersion,
			"minVersion", tlsVersion,
			"maxVersion", tlsVersion)
		connState, err = client.StartTLS(tlsConfig)
		if err != nil {
			logger.LogError(slogLogger, "STARTTLS handshake failed", "error", err)
			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, "true", "", "", "", "", "", "", "", "", "", err.Error(),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("TLS handshake failed: %w", err)
		}

		fmt.Printf("✓ TLS handshake successful\n\n")
	}

	// Analyze TLS connection
	tlsInfo := tlsutil.AnalyzeTLSConnection(connState)
	printTLSInfo(tlsInfo)

	// Analyze certificate chain
	certInfo := tlsutil.AnalyzeCertificateChain(connState.PeerCertificates, client.GetHost())
	printCertificateInfo(certInfo)

	// Check for warnings
	warnings := tlsutil.CheckTLSWarnings(tlsInfo, certInfo, config.SkipVerify)
	if len(warnings) > 0 {
		fmt.Println("\n⚠ TLS Warnings:")
		fmt.Println(strings.Repeat("─", 60))
		for _, w := range warnings {
			fmt.Printf("  • %s\n", w)
		}
		fmt.Println(strings.Repeat("─", 60))
	}

	// Get recommendations
	recommendations := tlsutil.GetTLSRecommendations(tlsInfo)
	if len(recommendations) > 0 {
		fmt.Println("\n💡 Recommendations:")
		fmt.Println(strings.Repeat("─", 60))
		for _, r := range recommendations {
			fmt.Printf("  • %s\n", r)
		}
		fmt.Println(strings.Repeat("─", 60))
	}

	// Test encrypted connection
	fmt.Println("\n✓ Testing encrypted connection...")
	_, err = client.EHLO("smtptool.local")
	if err != nil {
		fmt.Printf("  ⚠ EHLO on encrypted connection failed: %v\n", err)
		logger.LogWarn(slogLogger, "EHLO on encrypted connection failed", "error", err)
	} else {
		fmt.Println("  ✓ Encrypted connection working")
	}

	// Determine STARTTLS availability value for CSV
	starttlsAvailable := "true"
	if config.SMTPS {
		starttlsAvailable = "N/A (SMTPS)"
	}

	// Log to CSV
	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		config.ConnectAddress,
		starttlsAvailable,
		tlsInfo.Version,
		tlsInfo.CipherSuite,
		certInfo.Subject,
		certInfo.Issuer,
		certInfo.ValidFrom.Format(time.RFC3339),
		certInfo.ValidTo.Format(time.RFC3339),
		strings.Join(certInfo.SANs, "; "),
		certInfo.VerificationStatus,
		strings.Join(warnings, "; "),
		"",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	if config.SMTPS {
		fmt.Println("\n✓ SMTPS test completed successfully")
	} else {
		fmt.Println("\n✓ STARTTLS test completed successfully")
	}
	logger.LogInfo(slogLogger, "teststarttls completed successfully",
		"tlsVersion", tlsInfo.Version,
		"cipherSuite", tlsInfo.CipherSuite)

	return nil
}

// printTLSInfo displays TLS connection details.
func printTLSInfo(info *tlsutil.TLSInfo) {
	fmt.Println("TLS Connection Details:")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  Protocol Version:    %s\n", info.Version)
	fmt.Printf("  Cipher Suite:        %s\n", info.CipherSuite)
	fmt.Printf("  Cipher Strength:     %s\n", strings.ToUpper(info.CipherSuiteStrength))
	if info.ServerName != "" {
		fmt.Printf("  Server Name (SNI):   %s\n", info.ServerName)
	}
	if info.NegotiatedProtocol != "" {
		fmt.Printf("  Negotiated Protocol: %s\n", info.NegotiatedProtocol)
	}
	fmt.Println(strings.Repeat("═", 60))
}

// printCertificateInfo displays certificate details.
func printCertificateInfo(info *tlsutil.CertificateInfo) {
	fmt.Println("\nCertificate Information:")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  Subject:             %s\n", info.Subject)
	fmt.Printf("  Issuer:              %s\n", info.Issuer)
	fmt.Printf("  Serial Number:       %s\n", info.SerialNumber)
	fmt.Printf("  Valid From:          %s\n", info.ValidFrom.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Valid To:            %s\n", info.ValidTo.Format("2006-01-02 15:04:05 MST"))

	if info.IsExpired {
		fmt.Printf("  Status:              ⚠ EXPIRED\n")
	} else {
		fmt.Printf("  Days Until Expiry:   %d\n", info.DaysUntilExpiry)
	}

	if len(info.SANs) > 0 {
		fmt.Println("  Subject Alternative Names:")
		for _, san := range info.SANs {
			fmt.Printf("    • %s\n", san)
		}
	}

	fmt.Printf("  Signature Algorithm: %s\n", info.SignatureAlgorithm)
	fmt.Printf("  Public Key:          %s (%d bits)\n", info.PublicKeyAlgorithm, info.PublicKeySize)

	if len(info.KeyUsage) > 0 {
		fmt.Printf("  Key Usage:           %s\n", strings.Join(info.KeyUsage, ", "))
	}
	if len(info.ExtKeyUsage) > 0 {
		fmt.Printf("  Extended Key Usage:  %s\n", strings.Join(info.ExtKeyUsage, ", "))
	}

	fmt.Printf("  Verification:        %s\n", strings.ToUpper(info.VerificationStatus))
	fmt.Printf("  Chain Length:        %d certificate(s)\n", info.ChainLength)

	if info.IsSelfSigned {
		fmt.Println("  ⚠ Self-signed certificate")
	}

	fmt.Println(strings.Repeat("═", 60))
}
