package main

import (
	"context"
	"fmt"
	"log/slog"

	"gomailtesttool/internal/common/logger"
)

// executeAction dispatches to the appropriate action handler.
func executeAction(ctx context.Context, config *Config, csvLogger logger.Logger, slogLogger *slog.Logger) error {
	switch config.Action {
	case ActionTestConnect:
		return testConnect(ctx, config, csvLogger, slogLogger)
	case ActionTestStartTLS:
		return testStartTLS(ctx, config, csvLogger, slogLogger)
	case ActionTestAuth:
		return testAuth(ctx, config, csvLogger, slogLogger)
	case ActionSendMail:
		return sendMail(ctx, config, csvLogger, slogLogger)
	default:
		return fmt.Errorf("unknown action: %s", config.Action)
	}
}
