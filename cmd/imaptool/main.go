package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gomailtesttool/internal/common/logger"
	"gomailtesttool/internal/common/version"
)

func main() {
	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Parse configuration
	config := parseAndConfigureFlags()

	// Handle version flag
	if config.ShowVersion {
		fmt.Printf("imaptool version %s\n", version.Get())
		fmt.Println("Part of gomailtesttool suite - https://github.com/ehlo-pl/gomailtesttool")
		os.Exit(0)
	}

	// Validate action is provided
	if config.Action == "" {
		fmt.Fprintln(os.Stderr, "Error: -action is required")
		fmt.Fprintln(os.Stderr, "Use -help for usage information")
		os.Exit(1)
	}

	// Validate configuration
	if err := validateConfiguration(config); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Setup slog logger
	slogLogger := logger.SetupLogger(config.VerboseMode, config.LogLevel)

	// Setup file logger (CSV or JSON)
	logFormat, err := logger.ParseLogFormat(config.LogFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log format: %v\n", err)
		os.Exit(1)
	}

	csvLogger, err := logger.NewLogger(logFormat, "imaptool", config.Action)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer csvLogger.Close()

	// Execute the action
	if err := executeAction(ctx, config, csvLogger, slogLogger); err != nil {
		logger.LogError(slogLogger, "Action failed", "action", config.Action, "error", err)
		os.Exit(1)
	}
}
