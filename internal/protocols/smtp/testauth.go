package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
	smtptls "github.com/ehlo-pl/gomailtesttool/internal/smtp/tls"
)

// testAuth performs SMTP authentication testing.
func testAuth(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	if config.SMTPS {
		if config.ConnectAddress != "" {
			fmt.Printf("Testing SMTP authentication on %s:%d (SMTPS) (connecting via %s)...\n\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			fmt.Printf("Testing SMTP authentication on %s:%d (SMTPS)...\n\n", config.Host, config.Port)
		}
	} else {
		if config.ConnectAddress != "" {
			fmt.Printf("Testing SMTP authentication on %s:%d (connecting via %s)...\n\n", config.Host, config.Port, config.ConnectAddress)
		} else {
			fmt.Printf("Testing SMTP authentication on %s:%d...\n\n", config.Host, config.Port)
		}
	}

	// Write CSV header
	if shouldWrite, _ := csvLogger.ShouldWriteHeader(); shouldWrite {
		if err := csvLogger.WriteHeader([]string{
			"Action", "Status", "Server", "Port", "Connect_Address", "Username",
			"Auth_Mechanisms_Available", "Auth_Method_Used", "Auth_Result",
			"TLS_Version", "Cipher_Suite", "Cipher_Strength",
			"Cert_Subject", "Cert_Issuer", "Cert_SANs",
			"Cert_Valid_From", "Cert_Valid_To", "Cert_Verification_Status",
			"Error",
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
			config.ConnectAddress, config.Username, "", "", "FAILURE",
			"", "", "", "", "", "", "", "", "", // No TLS info on connection failure
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}
	defer client.Close()

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
			config.ConnectAddress, config.Username, "", "", "FAILURE",
			"", "", "", "", "", "", "", "", "", // No TLS info on EHLO failure
			err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return err
	}

	// Check for AUTH capability
	authMechanisms := caps.GetAuthMechanisms()
	if len(authMechanisms) == 0 {
		msg := "Server does not advertise AUTH capability"
		fmt.Printf("✗ %s\n", msg)
		logger.LogWarn(slogLogger, msg)
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.Username, "none", "", "FAILURE",
			"", "", "", "", "", "", "", "", "", // No TLS info yet
			msg,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("server does not advertise AUTH capability")
	}

	fmt.Printf("✓ Server supports AUTH mechanisms: %s\n\n", strings.Join(authMechanisms, ", "))

	// Handle TLS: either already established via SMTPS, or upgrade via STARTTLS
	var tlsState *tls.ConnectionState
	if config.SMTPS {
		// For SMTPS, TLS is already established
		tlsState = client.GetTLSState()
		if config.VerboseMode && tlsState != nil {
			displayComprehensiveTLSInfo(tlsState, client.GetHost(), config.VerboseMode)
		}
	} else if !config.NoStartTLS && (config.Port == 25 || config.Port == 587) && caps.SupportsSTARTTLS() {
		// STARTTLS if on port 25/587 and available
		fmt.Println("Upgrading to TLS before authentication...")
		tlsVersion := smtptls.ParseTLSVersion(config.TLSVersion)
		tlsConfig := &tls.Config{
			ServerName:         client.GetHost(), // resolved MX hostname if --use-mx, otherwise --host
			InsecureSkipVerify: config.SkipVerify,
			MinVersion:         tlsVersion,
			MaxVersion:         tlsVersion, // Force exact TLS version
		}

		tlsState, err = client.StartTLS(tlsConfig)
		if err != nil {
			logger.LogError(slogLogger, "STARTTLS failed", "error", err)
			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				config.ConnectAddress, config.Username, strings.Join(authMechanisms, ", "), "", "FAILURE",
				"", "", "", "", "", "", "", "", "", // No TLS info on STARTTLS failure
				fmt.Sprintf("STARTTLS failed: %v", err),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("STARTTLS failed: %w", err)
		}

		fmt.Println("✓ TLS upgrade successful")

		// Show TLS cipher information in verbose mode
		if config.VerboseMode {
			displayComprehensiveTLSInfo(tlsState, client.GetHost(), config.VerboseMode)
		}

		// Re-run EHLO on encrypted connection
		caps, err = client.EHLO("smtptool.local")
		if err != nil {
			return fmt.Errorf("EHLO on encrypted connection failed: %w", err)
		}
		authMechanisms = caps.GetAuthMechanisms()
	}

	// Determine auth method to use
	var methodsToTry []string
	if config.AuthMethod == "auto" {
		methodsToTry = []string{"auto"}
	} else {
		methodsToTry = []string{config.AuthMethod}
	}

	methodUsed := selectAuthMechanism(methodsToTry, authMechanisms, config.AccessToken != "")
	if methodUsed == "" {
		msg := fmt.Sprintf("No compatible authentication mechanism found (requested: %s, available: %s)",
			config.AuthMethod, strings.Join(authMechanisms, ", "))
		fmt.Printf("✗ %s\n", msg)
		tlsData := formatTLSInfoForCSV(tlsState, client.GetHost())
		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			config.ConnectAddress, config.Username, strings.Join(authMechanisms, ", "), "", "FAILURE",
			tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
			tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
			tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
			msg,
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return errors.New(msg)
	}

	fmt.Printf("Attempting authentication with method: %s\n", methodUsed)
	logger.LogDebug(slogLogger, "Authenticating", "method", methodUsed, "username", security.MaskUsername(config.Username))

	// Authenticate
	err = client.Auth(config.Username, config.Password, config.AccessToken, []string{methodUsed})

	authResult := "SUCCESS"
	status := "SUCCESS"
	errorMsg := ""

	if err != nil {
		authResult = "FAILURE"
		status = "FAILURE"
		errorMsg = err.Error()
		fmt.Printf("\n✗ Authentication failed: %v\n", err)
		logger.LogError(slogLogger, "Authentication failed",
			"error", err,
			"username", security.MaskUsername(config.Username),
			"password", security.MaskPassword(config.Password),
			"accesstoken", security.MaskAccessToken(config.AccessToken),
			"method", methodUsed)

		// Show TLS cipher information on auth failure if verbose and TLS was used
		if config.VerboseMode && tlsState != nil {
			fmt.Println("\nAuthentication failed. TLS Connection Details:")
			displayComprehensiveTLSInfo(tlsState, client.GetHost(), config.VerboseMode)
		}
	} else {
		fmt.Printf("\n✓ Authentication successful\n")
		logger.LogInfo(slogLogger, "Authentication successful", "method", methodUsed)
	}

	// Log to CSV
	tlsData := formatTLSInfoForCSV(tlsState, client.GetHost())
	if logErr := csvLogger.WriteRow([]string{
		config.Action, status, config.Host, fmt.Sprintf("%d", config.Port),
		config.ConnectAddress, config.Username, strings.Join(authMechanisms, ", "),
		methodUsed, authResult,
		tlsData.TLSVersion, tlsData.CipherSuite, tlsData.CipherStrength,
		tlsData.CertSubject, tlsData.CertIssuer, tlsData.CertSANs,
		tlsData.CertValidFrom, tlsData.CertValidTo, tlsData.VerificationStatus,
		errorMsg,
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	if err != nil {
		return err
	}

	fmt.Println("\n✓ Authentication test completed successfully")
	return nil
}
