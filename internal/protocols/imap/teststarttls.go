package imap

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
)

// testStartTLS performs comprehensive TLS/SSL testing with detailed diagnostics.
// The go-imap client hides handshake details, so this action speaks the minimal
// IMAP needed (greeting, CAPABILITY, STARTTLS) over a raw socket to expose the
// full tls.ConnectionState. For IMAPS mode, tests implicit TLS instead.
func testStartTLS(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.IMAPS {
		fmt.Printf("Testing IMAPS (implicit TLS) on %s:%d...\n\n", config.Host, config.Port)
	} else {
		fmt.Printf("Testing STARTTLS on %s:%d...\n\n", config.Host, config.Port)
	}

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

	writeFailure := func(starttlsAvailable, errMsg string) {
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, starttlsAvailable, "", "", "", "", "", "", "", "", "", errMsg,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
	}

	// Resolve the connect address the same way IMAPClient.Connect does.
	connectHost := config.Host
	if config.ConnectAddress != "" {
		connectHost = config.ConnectAddress
	}
	connectHost, err := network.ResolveForDial(ctx, connectHost, config.IPv4Only, config.IPv6Only)
	if err != nil {
		writeFailure("unknown", err.Error())
		return err
	}
	address := net.JoinHostPort(connectHost, fmt.Sprintf("%d", config.Port))

	tlsVersion := tlsutil.ParseTLSVersion(config.TLSVersion)
	tlsConfig := &tls.Config{
		ServerName:         config.Host,
		InsecureSkipVerify: config.SkipVerify,
		MinVersion:         tlsVersion,
		MaxVersion:         tlsVersion, // Force exact TLS version
	}

	dialer := &net.Dialer{Timeout: config.Timeout}

	var connState *tls.ConnectionState
	var tlsConn *tls.Conn

	if config.IMAPS {
		// Implicit TLS: handshake immediately after TCP connect.
		conn, err := tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
		if err != nil {
			logger.LogError(slogLogger, "IMAPS connection failed", "error", err)
			writeFailure("N/A (IMAPS)", err.Error())
			return fmt.Errorf("IMAPS connection failed: %w", err)
		}
		tlsConn = conn
		defer func() { _ = tlsConn.Close() }()

		state := tlsConn.ConnectionState()
		connState = &state
		fmt.Printf("✓ IMAPS TLS handshake completed\n\n")

		// Consume the greeting so the connection is in a clean state.
		reader := bufio.NewReader(tlsConn)
		_ = tlsConn.SetReadDeadline(time.Now().Add(config.Timeout))
		if _, err := reader.ReadString('\n'); err != nil {
			logger.LogWarn(slogLogger, "Failed to read IMAPS greeting", "error", err)
		}
	} else {
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			logger.LogError(slogLogger, "Connection failed", "error", err)
			writeFailure("unknown", err.Error())
			return fmt.Errorf("connection failed: %w", err)
		}
		defer func() { _ = conn.Close() }()

		reader := bufio.NewReader(conn)

		// Read the untagged greeting ("* OK ...").
		_ = conn.SetReadDeadline(time.Now().Add(config.Timeout))
		greeting, err := reader.ReadString('\n')
		if err != nil {
			writeFailure("unknown", err.Error())
			return fmt.Errorf("failed to read greeting: %w", err)
		}
		if !strings.HasPrefix(greeting, "* OK") && !strings.HasPrefix(greeting, "* PREAUTH") {
			msg := fmt.Sprintf("unexpected greeting: %s", strings.TrimSpace(greeting))
			writeFailure("unknown", msg)
			return errors.New(msg)
		}
		fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

		// Confirm STARTTLS is advertised.
		capLines, ok, err := imapCommand(conn, reader, "a1", "CAPABILITY", config.Timeout)
		if err != nil {
			writeFailure("unknown", err.Error())
			return fmt.Errorf("CAPABILITY failed: %w", err)
		}
		advertised := ok && strings.Contains(strings.ToUpper(strings.Join(capLines, " ")), "STARTTLS")
		if !advertised {
			msg := "STARTTLS not advertised by server"
			fmt.Printf("✗ %s\n", msg)
			logger.LogWarn(slogLogger, msg)
			writeFailure("false", msg)
			return errors.New(msg)
		}
		fmt.Printf("✓ STARTTLS capability available\n\n")

		// Issue STARTTLS and upgrade the connection.
		if _, ok, err := imapCommand(conn, reader, "a2", "STARTTLS", config.Timeout); err != nil || !ok {
			if err == nil {
				err = errors.New("server rejected STARTTLS")
			}
			logger.LogError(slogLogger, "STARTTLS failed", "error", err)
			writeFailure("true", err.Error())
			return fmt.Errorf("STARTTLS failed: %w", err)
		}

		fmt.Println("Performing TLS handshake...")
		logger.LogDebug(slogLogger, "Starting TLS handshake",
			"skipVerify", config.SkipVerify,
			"tlsVersion", config.TLSVersion)
		tlsConn = tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			logger.LogError(slogLogger, "TLS handshake failed", "error", err)
			writeFailure("true", err.Error())
			return fmt.Errorf("TLS handshake failed: %w", err)
		}
		state := tlsConn.ConnectionState()
		connState = &state
		fmt.Printf("✓ TLS handshake successful\n\n")
	}

	tlsInfo := tlsutil.AnalyzeTLSConnection(connState)
	tlsutil.PrintTLSInfo(tlsInfo)

	certInfo := tlsutil.AnalyzeCertificateChain(connState.PeerCertificates, config.Host)
	tlsutil.PrintCertificateInfo(certInfo)

	warnings := tlsutil.CheckTLSWarnings(tlsInfo, certInfo, config.SkipVerify)
	tlsutil.PrintTLSWarnings(warnings)
	tlsutil.PrintTLSRecommendations(tlsutil.GetTLSRecommendations(tlsInfo))

	// Test the encrypted connection with a CAPABILITY round-trip, then LOGOUT.
	fmt.Println("\n✓ Testing encrypted connection...")
	tlsReader := bufio.NewReader(tlsConn)
	if _, ok, err := imapCommand(tlsConn, tlsReader, "a3", "CAPABILITY", config.Timeout); err != nil || !ok {
		if err == nil {
			err = errors.New("server returned non-OK")
		}
		fmt.Printf("  ⚠ CAPABILITY on encrypted connection failed: %v\n", err)
		logger.LogWarn(slogLogger, "CAPABILITY on encrypted connection failed", "error", err)
	} else {
		fmt.Println("  ✓ Encrypted connection working")
	}
	_, _, _ = imapCommand(tlsConn, tlsReader, "a4", "LOGOUT", config.Timeout)

	starttlsAvailable := "true"
	if config.IMAPS {
		starttlsAvailable = "N/A (IMAPS)"
	}

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

	if config.IMAPS {
		fmt.Println("\n✓ IMAPS test completed successfully")
	} else {
		fmt.Println("\n✓ STARTTLS test completed successfully")
	}
	logger.LogInfo(slogLogger, "teststarttls completed successfully",
		"tlsVersion", tlsInfo.Version,
		"cipherSuite", tlsInfo.CipherSuite)

	return nil
}

// imapCommand sends a tagged IMAP command and reads response lines until the
// tagged completion line. It returns all response lines and whether the tagged
// response was OK.
func imapCommand(conn net.Conn, reader *bufio.Reader, tag, command string, timeout time.Duration) ([]string, bool, error) {
	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	if _, err := fmt.Fprintf(conn, "%s %s\r\n", tag, command); err != nil {
		return nil, false, fmt.Errorf("failed to send %s: %w", command, err)
	}

	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return lines, false, fmt.Errorf("failed to read %s response: %w", command, err)
		}
		line = strings.TrimRight(line, "\r\n")
		lines = append(lines, line)
		if strings.HasPrefix(line, tag+" ") {
			return lines, strings.HasPrefix(line, tag+" OK"), nil
		}
	}
}
