package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"google.golang.org/api/googleapi"

	"github.com/ehlo-pl/gomailtesttool/internal/common/retry"
)

// logDebug logs a debug message if logger is not nil.
func logDebug(l *slog.Logger, msg string, args ...any) {
	if l != nil {
		l.Debug(msg, args...)
	}
}

// logWarn logs a warning message if logger is not nil.
func logWarn(l *slog.Logger, msg string, args ...any) {
	if l != nil {
		l.Warn(msg, args...)
	}
}

// logError logs an error message if logger is not nil.
func logError(l *slog.Logger, msg string, args ...any) {
	if l != nil {
		l.Error(msg, args...)
	}
}

// logVerbose prints verbose output to stderr if verbose mode is enabled.
func logVerbose(verbose bool, format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] "+format+"\n", args...)
	}
}

// validateMessageID validates an Internet Message-ID to prevent search-operator
// injection. Message-IDs must follow RFC 5322 format: <local@domain>. It mirrors
// the msgraph guard, rejecting quotes/backslashes and query operators before the
// value is placed into a Gmail "rfc822msgid:" search.
func validateMessageID(msgID string) error {
	if msgID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}
	if !strings.HasPrefix(msgID, "<") || !strings.HasSuffix(msgID, ">") {
		return fmt.Errorf("must be enclosed in angle brackets: <local@domain>")
	}
	if len(msgID) > 998 {
		return fmt.Errorf("exceeds maximum length of 998 characters")
	}
	if strings.ContainsAny(msgID, "'\"\\") {
		return fmt.Errorf("contains invalid characters: quotes and backslashes not allowed")
	}
	// Reject whitespace, which would split the search into extra operators.
	if strings.ContainsAny(msgID, " \t\r\n") {
		return fmt.Errorf("must not contain whitespace")
	}
	return nil
}

// validateSearchSubject validates a subject search pattern used in a Gmail
// "subject:" query. It rejects empty/oversized input and control characters.
func validateSearchSubject(subject string) error {
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("subject cannot be empty")
	}
	if len(subject) > 998 {
		return fmt.Errorf("exceeds maximum length of 998 characters")
	}
	for _, r := range subject {
		if r < 0x20 && r != '\t' {
			return fmt.Errorf("contains invalid control characters")
		}
	}
	return nil
}

// parseFlexibleTime parses a time string accepting RFC3339 (with timezone) or
// the PowerShell sortable format (without timezone, assumed UTC).
func parseFlexibleTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("time string is empty")
	}
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", timeStr); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339 like '2026-01-15T14:00:00Z' or PowerShell sortable like '2026-01-15T14:00:00')")
}

// enrichGmailAPIError adds actionable context to Gmail/Calendar API and OAuth
// errors, mirroring msgraph's enrichGraphAPIError. It unwraps *googleapi.Error
// for HTTP-level detail and special-cases the common domain-wide-delegation
// scope mismatch (unauthorized_client).
func enrichGmailAPIError(err error, operation string) error {
	if err == nil {
		return nil
	}

	// OAuth token-exchange failures (e.g. DWD scope mismatch) are not
	// googleapi.Error values; detect them by their standard error code.
	if strings.Contains(err.Error(), "unauthorized_client") {
		return fmt.Errorf("authorization failed during %s: the impersonated user or requested scopes are not authorized. "+
			"Check the Admin Console (Security ▸ API controls ▸ Domain-wide delegation) grants this client the exact scopes, "+
			"and that --mailbox is a real Workspace user: %w", operation, err)
	}

	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return err
	}

	switch gerr.Code {
	case 429:
		retryAfter := ""
		if gerr.Header != nil {
			retryAfter = gerr.Header.Get("Retry-After")
		}
		log.Printf("[WARN] Gmail API rate limit exceeded during %s", operation)
		msg := fmt.Sprintf("rate limit exceeded during %s", operation)
		if retryAfter != "" {
			msg += fmt.Sprintf(" (retry after %s seconds)", retryAfter)
		}
		msg += ". Consider reducing request frequency or increasing --retrydelay"
		return fmt.Errorf("%s: %w", msg, err)
	case 401, 403:
		return fmt.Errorf("access denied during %s (code %d): verify the token scopes and that domain-wide "+
			"delegation authorizes this client for the mailbox: %w", operation, gerr.Code, err)
	case 500, 502, 503, 504:
		log.Printf("[WARN] Gmail API service error during %s (code: %d)", operation, gerr.Code)
		return fmt.Errorf("service temporarily unavailable during %s (code %d): %w", operation, gerr.Code, err)
	}
	return err
}

// isRetryableGmailError reports whether err is transient and worth retrying.
// It retries throttling (429) and server (5xx) responses, otherwise delegating
// to the shared string-based transient-error detector.
func isRetryableGmailError(err error) bool {
	if err == nil {
		return false
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		switch gerr.Code {
		case 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	return retry.IsRetryableError(err)
}

// retryGmail runs operation with exponential backoff, retrying only on
// Gmail-retryable errors. It honors context cancellation.
func retryGmail(ctx context.Context, config *Config, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		lastErr = operation()
		if lastErr == nil {
			return nil
		}
		if !isRetryableGmailError(lastErr) {
			return lastErr
		}
		if attempt == config.MaxRetries {
			return fmt.Errorf("operation failed after %d retries: %w", config.MaxRetries, lastErr)
		}

		delay := config.RetryDelay * time.Duration(1<<uint(attempt))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		log.Printf("Retryable error (attempt %d/%d): %v. Retrying in %v...", attempt+1, config.MaxRetries, lastErr, delay)
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}
	}
	return lastErr
}

// printJSON marshals data to indented JSON and prints it to stdout.
func printJSON(data any) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		return
	}
	fmt.Println(string(out))
}
