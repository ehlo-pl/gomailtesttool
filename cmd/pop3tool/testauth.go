package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gomailtesttool/internal/common/logger"
)

// testAuth tests POP3 authentication.
func testAuth(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	fmt.Printf("Testing POP3 authentication to %s:%d...\n", config.Host, config.Port)

	// CSV columns for testauth
	columns := []string{"Action", "Status", "Server", "Port", "Username", "Auth_Method", "Auth_Result", "Error"}
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
			maskUsername(config.Username), "", "FAILURE", err.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = client.Quit() }()

	fmt.Printf("✓ Connected to %s:%d\n", config.Host, config.Port)

	// Upgrade to TLS if needed
	if config.StartTLS && client.GetTLSState() == nil {
		fmt.Println("Upgrading to TLS via STLS...")
		if err := client.StartTLS(nil); err != nil {
			logger.LogError(slogLogger, "STLS upgrade failed", "error", err)

			if logErr := csvLogger.WriteRow([]string{
				config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
				maskUsername(config.Username), "", "FAILURE", fmt.Sprintf("STLS failed: %v", err),
			}); logErr != nil {
				logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
			}
			return fmt.Errorf("STLS failed: %w", err)
		}
		fmt.Println("✓ TLS upgrade successful")
	}

	// Get capabilities to determine auth methods
	caps, _ := client.Capabilities(ctx)

	// Determine auth method
	authMethod := config.AuthMethod
	if strings.EqualFold(authMethod, "auto") {
		if config.AccessToken != "" {
			if caps != nil && caps.SupportsXOAUTH2() {
				authMethod = "XOAUTH2"
			} else {
				logger.LogWarn(slogLogger, "Access token provided but XOAUTH2 not supported by server")
				authMethod = "USER"
			}
		} else {
			authMethod = "USER"
		}
	}

	fmt.Printf("Authenticating with method: %s\n", authMethod)

	// Authenticate
	var authErr error
	if config.AccessToken != "" && strings.EqualFold(authMethod, "XOAUTH2") {
		authErr = client.Auth(ctx, config.Username, "", config.AccessToken)
	} else {
		authErr = client.Auth(ctx, config.Username, config.Password, "")
	}

	if authErr != nil {
		logger.LogError(slogLogger, "Authentication failed",
			"error", authErr,
			"username", maskUsername(config.Username),
			"method", authMethod)

		if logErr := csvLogger.WriteRow([]string{
			config.Action, "FAILURE", config.Host, fmt.Sprintf("%d", config.Port),
			maskUsername(config.Username), authMethod, "FAILURE", authErr.Error(),
		}); logErr != nil {
			logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
		}
		return fmt.Errorf("authentication failed: %w", authErr)
	}

	logger.LogInfo(slogLogger, "Authentication successful",
		"username", maskUsername(config.Username),
		"method", authMethod)

	if logErr := csvLogger.WriteRow([]string{
		config.Action, "SUCCESS", config.Host, fmt.Sprintf("%d", config.Port),
		maskUsername(config.Username), authMethod, "SUCCESS", "",
	}); logErr != nil {
		logger.LogError(slogLogger, "Failed to write CSV row", "error", logErr)
	}

	fmt.Println("\n✓ Authentication successful")
	return nil
}
