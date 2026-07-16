package pop3

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
// For POP3S mode, tests implicit TLS (TLS handshake happens immediately after
// TCP connect); otherwise the connection is upgraded with STLS (RFC 2595).
func testStartTLS(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.POP3S {
		fmt.Printf("Testing POP3S (implicit TLS) on %s:%d...\n\n", config.Host, config.Port)
	} else {
		fmt.Printf("Testing STLS on %s:%d...\n\n", config.Host, config.Port)
	}

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Connect_Address", "STLS_Available",
			"TLS_Version", "Cipher_Suite", "Cert_Subject", "Cert_Issuer",
			"Cert_Valid_From", "Cert_Valid_To", "Cert_SANs",
			"Verification_Status", "Warnings", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	writeFailure := func(stlsAvailable, errMsg string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, stlsAvailable, "", "", "", "", "", "", "", "", "", errMsg,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	client := NewPOP3Client(config)
	logger.LogDebug(slogLogger, "Connecting to POP3 server")

	if err := client.Connect(ctx); err != nil {
		logger.LogError(slogLogger, "Connection failed", "error", err)
		writeFailure("unknown", err.Error())
		return err
	}
	defer func() { _ = client.Quit() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	var connState *tls.ConnectionState

	if config.POP3S {
		// For POP3S, the TLS handshake already happened during Connect().
		connState = client.GetTLSState()
		if connState == nil {
			msg := "POP3S connection state not available"
			logger.LogError(slogLogger, msg)
			writeFailure("N/A (POP3S)", msg)
			return errors.New(msg)
		}
		fmt.Printf("✓ POP3S TLS handshake completed\n\n")
	} else {
		caps, err := client.Capabilities(ctx)
		if err != nil {
			logger.LogError(slogLogger, "CAPA failed", "error", err)
			writeFailure("unknown", err.Error())
			return err
		}

		if !caps.SupportsSTLS() {
			msg := "STLS not advertised by server"
			fmt.Printf("✗ %s\n", msg)
			logger.LogWarn(slogLogger, msg)
			writeFailure("false", msg)
			return errors.New(msg)
		}

		fmt.Printf("✓ STLS capability available\n\n")

		fmt.Println("Performing TLS handshake...")
		tlsVersion := tlsutil.ParseTLSVersion(config.TLSVersion)
		tlsConfig := &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: config.SkipVerify,
			MinVersion:         tlsVersion,
			MaxVersion:         tlsVersion, // Force exact TLS version
		}

		logger.LogDebug(slogLogger, "Starting TLS handshake",
			"skipVerify", config.SkipVerify,
			"tlsVersion", config.TLSVersion)
		if err := client.StartTLS(tlsConfig); err != nil {
			logger.LogError(slogLogger, "STLS handshake failed", "error", err)
			writeFailure("true", err.Error())
			return fmt.Errorf("TLS handshake failed: %w", err)
		}
		connState = client.GetTLSState()

		fmt.Printf("✓ TLS handshake successful\n\n")
	}

	tlsInfo := tlsutil.AnalyzeTLSConnection(connState)
	tlsutil.PrintTLSInfo(tlsInfo)

	certInfo := tlsutil.AnalyzeCertificateChain(connState.PeerCertificates, config.Host)
	tlsutil.PrintCertificateInfo(certInfo)

	warnings := tlsutil.CheckTLSWarnings(tlsInfo, certInfo, config.SkipVerify)
	tlsutil.PrintTLSWarnings(warnings)
	tlsutil.PrintTLSRecommendations(tlsutil.GetTLSRecommendations(tlsInfo))

	// Test the encrypted connection with a CAPA round-trip.
	fmt.Println("\n✓ Testing encrypted connection...")
	if _, err := client.Capabilities(ctx); err != nil {
		fmt.Printf("  ⚠ CAPA on encrypted connection failed: %v\n", err)
		logger.LogWarn(slogLogger, "CAPA on encrypted connection failed", "error", err)
	} else {
		fmt.Println("  ✓ Encrypted connection working")
	}

	stlsAvailable := "true"
	if config.POP3S {
		stlsAvailable = "N/A (POP3S)"
	}

	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		config.ConnectAddress,
		stlsAvailable,
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

	if config.POP3S {
		fmt.Println("\n✓ POP3S test completed successfully")
	} else {
		fmt.Println("\n✓ STLS test completed successfully")
	}
	logger.LogInfo(slogLogger, "teststarttls completed successfully",
		"tlsVersion", tlsInfo.Version,
		"cipherSuite", tlsInfo.CipherSuite)

	return nil
}
