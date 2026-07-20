package ews

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
)

// testConnect performs an HTTP/TLS probe against the EWS endpoint.
// Credentials are not required — HTTP 401/403 confirms the server is running.
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	logger.Tprintf("Testing EWS connectivity to https://%s:%d%s...\n", config.Host, config.Port, config.EWSPath)
	fmt.Println()

	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "EWSPath",
			"HTTP_Status", "HTTP_StatusCode",
			"TLS_Version", "Cipher_Suite",
			"Cert_Subject", "Cert_Issuer", "Cert_SANs",
			"Cert_Valid_From", "Cert_Valid_To",
			"Response_Time_ms", "Error",
		}); err != nil {
			logger.LogError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}

	ewsClient, err := NewEWSClient(config)
	if err != nil {
		writeConnectCSV(csvLogger, slogLogger, config, "", 0, "", "", nil, 0, err.Error())
		return err
	}

	logger.LogDebug(slogLogger, "Probing EWS endpoint", "url", ewsClient.EWSUrl())

	start := time.Now()
	resp, err := ewsClient.Probe(ctx)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		writeConnectCSV(csvLogger, slogLogger, config, "", 0, "", "", nil, elapsed, err.Error())
		logger.LogError(slogLogger, "EWS probe failed", "error", err)
		return err
	}

	httpStatus := resp.Status
	statusCode := resp.StatusCode

	// Any HTTP response (including 401/403) means the server is alive.
	logger.Tprintf("✓ EWS endpoint responded: HTTP %s\n", httpStatus)
	switch statusCode {
	case http.StatusUnauthorized:
		logger.Tprintln("  (401 Unauthorized — server is alive, credentials required for further operations)")
	case http.StatusForbidden:
		logger.Tprintln("  (403 Forbidden — server is alive, check permissions)")
	}

	// Display TLS information
	var certs []*x509.Certificate
	tlsVersion, cipherSuite := "", ""

	if resp.TLS != nil {
		state := resp.TLS
		tlsVersion = tlsutil.TLSVersionString(state.Version)
		cipherSuite = tlsCipherName(state.CipherSuite)
		certs = state.PeerCertificates

		fmt.Println()
		logger.Tprintf("TLS Connection:\n")
		logger.Tprintf("  Protocol:     %s\n", tlsVersion)
		logger.Tprintf("  Cipher Suite: %s\n", cipherSuite)

		if len(certs) > 0 {
			cert := certs[0]
			fmt.Println()
			logger.Tprintf("Server Certificate:\n")
			logger.Tprintf("  Subject:    %s\n", cert.Subject.CommonName)
			logger.Tprintf("  Issuer:     %s\n", cert.Issuer.CommonName)
			logger.Tprintf("  Valid From: %s\n", cert.NotBefore.Format("2006-01-02"))
			logger.Tprintf("  Valid To:   %s\n", cert.NotAfter.Format("2006-01-02"))
			if len(cert.DNSNames) > 0 {
				logger.Tprintf("  SANs:       %s\n", strings.Join(cert.DNSNames, ", "))
			}
		}
		fmt.Println()
	}

	logger.Tprintf("  Response time: %d ms\n", elapsed)
	fmt.Println()
	logger.Tprintln("✓ EWS connectivity test completed")

	logger.LogInfo(slogLogger, "testconnect completed",
		"http_status", httpStatus, "elapsed_ms", elapsed)

	writeConnectCSV(csvLogger, slogLogger, config, httpStatus, statusCode, tlsVersion, cipherSuite, certs, elapsed, "")
	return nil
}

func writeConnectCSV(
	csvLogger logger.Logger, slogLogger *slog.Logger,
	config *Config, httpStatus string, statusCode int,
	tlsVersion, cipherSuite string, certs []*x509.Certificate,
	elapsed int64, errStr string,
) {
	status := "SUCCESS"
	if errStr != "" {
		status = "FAILURE"
	}

	certSubject, certIssuer, certSANs, certValidFrom, certValidTo := certCSVFields(certs)

	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host,
		fmt.Sprintf("%d", config.Port), config.EWSPath,
		httpStatus, fmt.Sprintf("%d", statusCode),
		tlsVersion, cipherSuite,
		certSubject, certIssuer, certSANs,
		certValidFrom, certValidTo,
		fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}
