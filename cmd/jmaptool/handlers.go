package main

import (
	"context"
	"fmt"
	"log/slog"

	"gomailtesttool/internal/common/logger"
)

// executeAction dispatches to the appropriate handler based on action.
func executeAction(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	switch config.Action {
	case "testconnect":
		return testConnect(ctx, config, csvLogger, slogLogger)
	case "testauth":
		return testAuth(ctx, config, csvLogger, slogLogger)
	case "getmailboxes":
		return getMailboxes(ctx, config, csvLogger, slogLogger)
	default:
		return fmt.Errorf("unknown action: %s", config.Action)
	}
}
