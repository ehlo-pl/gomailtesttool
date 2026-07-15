package msgraph

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
)

// graphConnectEndpoint is a lightweight Graph path used only to prove
// reachability. Unauthenticated it returns HTTP 401, which is exactly what we
// want: any HTTP response means the endpoint is alive. It is deliberately not
// $metadata (large body).
const graphConnectEndpoint = "https://graph.microsoft.com/v1.0/me"

// graphConnectTimeout bounds the whole probe (dial + TLS handshake + response).
const graphConnectTimeout = 30 * time.Second

// testConnect performs an unauthenticated HTTP/TLS probe against the Microsoft
// Graph endpoint. No credentials or mailbox are required — any HTTP status
// (401 expected) confirms reachability.
//
// For a global service like graph.microsoft.com this test cannot verify auth or
// permissions; its diagnostic value is catching proxy/firewall blocks and TLS
// interception. The certificate Issuer is the payload: a corporate TLS-MITM
// proxy surfaces there. (This is why the API-based gmail protocol omits it.)
func testConnect(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Testing Microsoft Graph connectivity to %s...\n\n", graphConnectEndpoint)

	writeConnectHeader(csvLogger, slogLogger)

	transport, err := connectTransport(config)
	if err != nil {
		writeConnectCSV(csvLogger, slogLogger, config, "", 0, "", "", nil, 0, err.Error())
		return err
	}
	client := &http.Client{Transport: transport, Timeout: graphConnectTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphConnectEndpoint, nil)
	if err != nil {
		writeConnectCSV(csvLogger, slogLogger, config, "", 0, "", "", nil, 0, err.Error())
		return err
	}

	logDebug(slogLogger, "Probing Graph endpoint", "url", graphConnectEndpoint, "proxy", config.ProxyURL)

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		// A transport error (DNS, connection refused, or an untrusted MITM
		// certificate that fails verification → no resp.TLS) is itself a useful
		// diagnostic signal.
		fmt.Printf("✗ Could not reach Graph endpoint: %s\n", err)
		writeConnectCSV(csvLogger, slogLogger, config, "", 0, "", "", nil, elapsed, err.Error())
		logError(slogLogger, "Graph probe failed", "error", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	httpStatus := resp.Status
	statusCode := resp.StatusCode

	var certs []*x509.Certificate
	tlsVersion, cipherSuite := "", ""
	if resp.TLS != nil {
		tlsVersion = tlsutil.TLSVersionString(resp.TLS.Version)
		cipherSuite = tlsCipherName(resp.TLS.CipherSuite)
		certs = resp.TLS.PeerCertificates
	}

	if config.OutputFormat == "json" {
		printJSON(buildConnectResult(httpStatus, statusCode, tlsVersion, cipherSuite, certs, elapsed))
	} else {
		// Any HTTP response (including 401) means the endpoint is alive.
		fmt.Printf("✓ Graph endpoint responded: HTTP %s\n", httpStatus)
		if statusCode == http.StatusUnauthorized {
			fmt.Println("  (401 Unauthorized — endpoint is reachable; credentials are required for data operations)")
		}

		if resp.TLS != nil {
			fmt.Printf("\nTLS Connection:\n")
			fmt.Printf("  Protocol:     %s\n", tlsVersion)
			fmt.Printf("  Cipher Suite: %s\n", cipherSuite)

			if len(certs) > 0 {
				cert := certs[0]
				fmt.Printf("\nServer Certificate:\n")
				fmt.Printf("  Subject:    %s\n", cert.Subject.CommonName)
				fmt.Printf("  Issuer:     %s\n", cert.Issuer.CommonName)
				fmt.Printf("  Valid From: %s\n", cert.NotBefore.Format("2006-01-02"))
				fmt.Printf("  Valid To:   %s\n", cert.NotAfter.Format("2006-01-02"))
				if len(cert.DNSNames) > 0 {
					fmt.Printf("  SANs:       %s\n", strings.Join(cert.DNSNames, ", "))
				}
				fmt.Println("  (An unexpected Issuer here indicates a TLS-intercepting proxy.)")
			}
			fmt.Println()
		}

		fmt.Printf("  Response time: %d ms\n\n", elapsed)
		fmt.Println("✓ Connectivity test completed")
	}

	logVerbose(config.VerboseMode, "testconnect completed: http_status=%s elapsed_ms=%d", httpStatus, elapsed)

	writeConnectCSV(csvLogger, slogLogger, config, httpStatus, statusCode, tlsVersion, cipherSuite, certs, elapsed, "")
	return nil
}

// connectTransport builds an HTTP transport for the probe. The proxy is set
// explicitly from --proxy when provided (falling back to the standard proxy
// environment variables), rather than relying on the default transport's cached
// environment lookup.
func connectTransport(config *Config) (*http.Transport, error) {
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", config.ProxyURL, err)
		}
		return &http.Transport{Proxy: http.ProxyURL(proxyURL)}, nil
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment}, nil
}

// buildConnectResult assembles the structured result printed in --output json mode.
func buildConnectResult(httpStatus string, statusCode int, tlsVersion, cipherSuite string, certs []*x509.Certificate, elapsed int64) map[string]interface{} {
	certSubject, certIssuer, certSANs, certValidFrom, certValidTo := certCSVFields(certs)
	return map[string]interface{}{
		"action":         ActionTestConnect,
		"status":         StatusSuccess,
		"endpoint":       graphConnectEndpoint,
		"httpStatus":     httpStatus,
		"httpStatusCode": statusCode,
		"tlsVersion":     tlsVersion,
		"cipherSuite":    cipherSuite,
		"certSubject":    certSubject,
		"certIssuer":     certIssuer,
		"certSANs":       certSANs,
		"certValidFrom":  certValidFrom,
		"certValidTo":    certValidTo,
		"responseTimeMs": elapsed,
	}
}

func writeConnectHeader(csvLogger logger.Logger, slogLogger *slog.Logger) {
	if csvLogger == nil {
		return
	}
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Endpoint",
			"HTTP_Status", "HTTP_StatusCode",
			"TLS_Version", "Cipher_Suite",
			"Cert_Subject", "Cert_Issuer", "Cert_SANs",
			"Cert_Valid_From", "Cert_Valid_To",
			"Response_Time_ms", "Error",
		}); err != nil {
			logError(slogLogger, "Failed to write CSV header", "error", err)
		}
	}
}

func writeConnectCSV(
	csvLogger logger.Logger, slogLogger *slog.Logger,
	config *Config, httpStatus string, statusCode int,
	tlsVersion, cipherSuite string, certs []*x509.Certificate,
	elapsed int64, errStr string,
) {
	if csvLogger == nil {
		return
	}
	status := StatusSuccess
	if errStr != "" {
		status = StatusError
	}

	certSubject, certIssuer, certSANs, certValidFrom, certValidTo := certCSVFields(certs)

	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, graphConnectEndpoint,
		httpStatus, fmt.Sprintf("%d", statusCode),
		tlsVersion, cipherSuite,
		certSubject, certIssuer, certSANs,
		certValidFrom, certValidTo,
		fmt.Sprintf("%d", elapsed), errStr,
	}); logErr != nil {
		logError(slogLogger, "Failed to write CSV row", "error", logErr)
	}
}

// tlsCipherName returns the cipher suite name, or hex if unknown.
func tlsCipherName(id uint16) string {
	name := tls.CipherSuiteName(id)
	if name == "" {
		return fmt.Sprintf("0x%04x", id)
	}
	return name
}

// certCSVFields extracts cert fields for CSV from the first cert in a chain.
// Returns subject, issuer, SANs, validFrom, validTo.
func certCSVFields(certs []*x509.Certificate) (subject, issuer, sans, validFrom, validTo string) {
	if len(certs) == 0 {
		return "", "", "", "", ""
	}
	cert := certs[0]
	return cert.Subject.CommonName,
		cert.Issuer.CommonName,
		strings.Join(cert.DNSNames, ";"),
		cert.NotBefore.Format("2006-01-02"),
		cert.NotAfter.Format("2006-01-02")
}
