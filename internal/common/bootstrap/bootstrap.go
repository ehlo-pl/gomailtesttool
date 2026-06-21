// Package bootstrap provides shared startup helpers used by every cmd/*tool
// main.go: signal handling and logger initialization.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// SetupSignalContext returns a context cancelled on SIGINT/SIGTERM.
// On signal it prints a shutdown notice to stdout and cancels the context.
func SetupSignalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n\nReceived interrupt signal. Shutting down gracefully...")
		cancel()
	}()

	return ctx, cancel
}

// InitLoggers builds the structured slog logger and the file logger (CSV/JSON)
// from raw config primitives. Callers decide whether a non-nil error is fatal.
//
// The slog logger is always returned (never nil) so callers can log the
// failure. The file logger is nil when an error is returned.
func InitLoggers(toolName, action string, verbose bool, logLevel, logFormat string) (*slog.Logger, logger.Logger, error) {
	slogLogger := logger.SetupLogger(verbose, logLevel)

	format, err := logger.ParseLogFormat(logFormat)
	if err != nil {
		return slogLogger, nil, fmt.Errorf("invalid log format: %w", err)
	}

	fileLogger, err := logger.NewLogger(format, toolName, action)
	if err != nil {
		return slogLogger, nil, fmt.Errorf("failed to initialize file logger: %w", err)
	}

	return slogLogger, fileLogger, nil
}

// LoadConfigFile loads a YAML configuration file into v, if path is non-empty.
// Keys in the file correspond to flag names (e.g. "host", "port", "bodyhtml"),
// and act as defaults: explicit CLI flags and environment variables still take
// precedence over values loaded from the file. If path is empty, this is a no-op.
// If path is non-empty and the file cannot be read or parsed, an error is returned.
func LoadConfigFile(v *viper.Viper, path string) error {
	if path == "" {
		return nil
	}

	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	return nil
}

// LoadConfigFileSection loads the values nested under the given top-level
// section key (e.g. "smtp", "msgraph") of a YAML configuration file and
// merges them into v as config-level defaults — env vars and any flags
// bound on v still take precedence. If path is empty or the section is
// absent from the file, this is a no-op. If path is non-empty and the file
// cannot be read or parsed, an error is returned.
func LoadConfigFileSection(v *viper.Viper, path, section string) error {
	if path == "" {
		return nil
	}

	fileViper := viper.New()
	fileViper.SetConfigFile(path)
	fileViper.SetConfigType("yaml")

	if err := fileViper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	sub := fileViper.Sub(section)
	if sub == nil {
		return nil
	}

	return v.MergeConfigMap(sub.AllSettings())
}
