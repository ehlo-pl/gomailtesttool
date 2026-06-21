//go:build !integration
// +build !integration

// Package main provides a portable CLI tool for interacting with Microsoft Graph API
// to manage Exchange Online (EXO) mailboxes. The tool supports sending emails,
// creating calendar events, and retrieving inbox messages and calendar events.
//
// Authentication methods supported:
//   - Client Secret: Standard App Registration secret
//   - PFX Certificate: Certificate file with private key
//   - Windows Certificate Store: Thumbprint-based certificate retrieval (Windows only)
//
// All operations are automatically logged to action-specific CSV files in the
// system temp directory for audit and troubleshooting purposes.
//
// Example usage:
//
//	msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "user@example.com" -action sendmail
//
// Version information is defined in version.go for this legacy module.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Handle -completion flag FIRST, before anything else runs
	// This ensures only completion script is output, all other flags are ignored
	for i, arg := range os.Args {
		if arg == "-completion" && i+1 < len(os.Args) {
			shellType := os.Args[i+1]
			if shellType == "bash" {
				fmt.Print(generateBashCompletion())
				os.Exit(0)
			} else if shellType == "powershell" {
				fmt.Print(generatePowerShellCompletion())
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, "Error: Invalid completion shell type '%s'\n", shellType)
				fmt.Fprintf(os.Stderr, "Valid options: bash, powershell\n\n")
				fmt.Fprintf(os.Stderr, "Usage:\n")
				fmt.Fprintf(os.Stderr, "  %s -completion bash > msgraphtool-completion.bash\n", os.Args[0])
				fmt.Fprintf(os.Stderr, "  %s -completion powershell > msgraphtool-completion.ps1\n", os.Args[0])
				os.Exit(1)
			}
		}
	}

	if err := run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// setupSignalHandling configures graceful shutdown on interrupt signals
// Returns a cancellable context for use throughout the application
func setupSignalHandling() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	// Handle interrupt signals (Ctrl+C, SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n\nReceived interrupt signal. Shutting down gracefully...")
		cancel()
	}()

	return ctx, cancel
}

// initializeServices sets up CSV logging and proxy configuration based on
// the provided configuration. Creates a CSV logger for the specified action
// and configures HTTP/HTTPS proxy environment variables if a proxy URL is specified.
//
// Returns the CSV logger (or nil if initialization failed) and any error encountered.
// If CSV logger initialization fails, a warning is logged but execution continues.
func initializeServices(config *Config) (*CSVLogger, error) {
	// Initialize CSV logging
	logger, err := NewCSVLogger(config.Action)
	if err != nil {
		log.Printf("Warning: Could not initialize CSV logging: %v", err)
		logger = nil // Continue without logging
	}

	// Configure proxy if specified
	// Go's http package automatically uses HTTP_PROXY/HTTPS_PROXY environment variables
	if config.ProxyURL != "" {
		os.Setenv("HTTP_PROXY", config.ProxyURL)
		os.Setenv("HTTPS_PROXY", config.ProxyURL)
		fmt.Printf("Using proxy: %s\n", config.ProxyURL)
	}

	return logger, nil
}

// run is the main application entry point that orchestrates the tool's execution flow.
// It performs the following steps:
//  1. Sets up graceful shutdown handling for interrupt signals
//  2. Parses and validates configuration from flags and environment variables
//  3. Initializes services (CSV logging, proxy configuration)
//  4. Creates Microsoft Graph SDK client with appropriate authentication
//  5. Executes the requested action (getevents, sendmail, sendinvite, getinbox)
//
// Returns an error if any step fails, nil on successful completion.
func run() error {
	// 1. Setup signal handling for graceful shutdown
	ctx, cancel := setupSignalHandling()
	defer cancel()

	// 2. Parse command-line flags and apply environment variables
	config := parseAndConfigureFlags()

	// 3. Handle version flag early exit
	if config.ShowVersion {
		fmt.Printf("Microsoft Graph EXO Mails/Calendar Golang Testing Tool - Version %s\n", version)
		return nil
	}

	// 4. Validate configuration
	if err := validateConfiguration(config); err != nil {
		fmt.Printf("Error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// 5. Setup structured logger
	slogger := setupLogger(config)
	slogger.Info("Application starting", "version", version, "action", config.Action)

	// Load body template if provided (validation already done in step 4)
	if config.BodyTemplate != "" {
		content, err := os.ReadFile(config.BodyTemplate)
		if err != nil {
			return fmt.Errorf("failed to read body template file: %w", err)
		}
		config.BodyHTML = string(content)
		slogger.Info("Loaded email body from template", "path", config.BodyTemplate, "size", len(config.BodyHTML))
	}

	// 6. Initialize services (CSV logging and proxy)
	csvLogger, err := initializeServices(config)
	if err != nil {
		// Error already logged in initializeServices, continue without logger
	}
	if csvLogger != nil {
		defer csvLogger.Close()
	}

	// 7. Setup Microsoft Graph client
	client, err := setupGraphClient(ctx, config, slogger)
	if err != nil {
		return err
	}

	// 8. Execute the requested action
	return executeAction(ctx, client, config, csvLogger)
}