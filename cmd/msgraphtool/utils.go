package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"gomailtesttool/internal/common/logger"
	"gomailtesttool/internal/common/retry"
)

// logDebug logs a debug message if logger is not nil
func logDebug(l *slog.Logger, msg string, args ...any) {
	if l != nil {
		l.Debug(msg, args...)
	}
}

// logError logs an error message if logger is not nil
func logError(l *slog.Logger, msg string, args ...any) {
	if l != nil {
		l.Error(msg, args...)
	}
}

// logVerbose prints verbose output to stderr if verbose mode is enabled
func logVerbose(verbose bool, format string, args ...interface{}) {
	if verbose {
		prefix := "[VERBOSE] "
		fmt.Fprintf(os.Stderr, prefix+format+"\n", args...)
	}
}

// maskSecret masks a secret for display, showing only first and last 4 characters
func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "********"
	}
	// Show first 4 and last 4 characters
	return secret[:4] + "********" + secret[len(secret)-4:]
}

// maskGUID masks a GUID for logging, showing only first 4 and last 4 characters
func maskGUID(guid string) string {
	if len(guid) <= 8 {
		return "****"
	}
	return guid[:4] + "****-****-****-****" + guid[len(guid)-4:]
}

// ifEmpty returns defaultVal if s is empty, otherwise returns s
func ifEmpty(s, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	return s
}

// truncate truncates a string to maxLen characters, adding ellipsis if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Int32Ptr creates a pointer to an int32 value
func Int32Ptr(i int32) *int32 {
	return &i
}

// pointerTo is a generic helper function to create pointers to values
func pointerTo[T any](v T) *T {
	return &v
}

// validateMessageID validates an Internet Message-ID to prevent OData injection attacks.
// Message-IDs must follow RFC 5322 format: <local@domain>
// This function blocks injection attempts by rejecting:
// - Quote characters that could break OData filter syntax
// - OData operators that could modify query logic
// - Invalid Message-ID formats
func validateMessageID(msgID string) error {
	// Message-ID must not be empty
	if msgID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	// Message-ID must be enclosed in angle brackets (RFC 5322)
	if !strings.HasPrefix(msgID, "<") || !strings.HasSuffix(msgID, ">") {
		return fmt.Errorf("must be enclosed in angle brackets: <local@domain>")
	}

	// Check length (RFC 5322: max 998 characters)
	if len(msgID) > 998 {
		return fmt.Errorf("exceeds maximum length of 998 characters")
	}

	// SECURITY: Reject quote characters that could break OData filter
	// This prevents injection attacks like: ' or 1 eq 1 or '
	if strings.ContainsAny(msgID, "'\"\\") {
		return fmt.Errorf("contains invalid characters: quotes and backslashes not allowed")
	}

	// SECURITY: Reject OData operators to prevent filter manipulation
	// This blocks injection patterns like: ' or internetMessageId eq '
	msgIDLower := strings.ToLower(msgID)
	odataKeywords := []string{" or ", " and ", " eq ", " ne ", " lt ", " gt ", " le ", " ge ", " not "}
	for _, keyword := range odataKeywords {
		if strings.Contains(msgIDLower, keyword) {
			return fmt.Errorf("contains OData operators which are not allowed")
		}
	}

	return nil
}

// enrichGraphAPIError enriches Graph API errors with additional context,
// particularly for rate limiting scenarios. It detects rate limit errors (429)
// and extracts the Retry-After header if available.
func enrichGraphAPIError(err error, csvLogger logger.Logger, operation string) error {
	if err == nil {
		return nil
	}

	// Check if this is an OData error from Microsoft Graph
	var odataErr *odataerrors.ODataError
	if !errors.As(err, &odataErr) {
		// Not an OData error, return as-is
		return err
	}

	// Extract error details if available
	if odataErr.GetErrorEscaped() == nil {
		return err
	}

	errorInfo := odataErr.GetErrorEscaped()
	code := ""
	message := ""

	if errorInfo.GetCode() != nil {
		code = *errorInfo.GetCode()
	}
	if errorInfo.GetMessage() != nil {
		message = *errorInfo.GetMessage()
	}

	// Handle rate limiting (429 TooManyRequests)
	if code == "TooManyRequests" || code == "activityLimitReached" {
		log.Printf("[WARN] Graph API rate limit exceeded during %s (code: %s)", operation, code)

		// Try to extract Retry-After header
		retryAfter := ""
		if odataErr.GetResponseHeaders() != nil {
			if retryHeaders := odataErr.GetResponseHeaders().Get("Retry-After"); len(retryHeaders) > 0 {
				retryAfter = retryHeaders[0] // Get first value
				log.Printf("[INFO] Rate limit retry guidance available: retry after %s seconds", retryAfter)
			}
		}

		// Build enriched error message
		enrichedMsg := fmt.Sprintf("rate limit exceeded during %s", operation)
		if retryAfter != "" {
			enrichedMsg += fmt.Sprintf(" (retry after %s seconds)", retryAfter)
		}
		enrichedMsg += ". Consider: 1) Reducing request frequency, 2) Implementing exponential backoff, 3) Reviewing API throttling limits"

		return fmt.Errorf("%s: %w", enrichedMsg, err)
	}

	// Handle other service errors (503, 504)
	if code == "ServiceUnavailable" || code == "GatewayTimeout" {
		log.Printf("[WARN] Graph API service error during %s (code: %s, message: %s)", operation, code, message)
		return fmt.Errorf("service temporarily unavailable during %s (code: %s): %w", operation, code, err)
	}

	// For other OData errors, log details for debugging
	if code != "" {
		log.Printf("[DEBUG] Graph API error during %s (code: %s, message: %s)", operation, code, message)
	}

	return err
}

// retryWithBackoff is a wrapper around the common retry package for backward compatibility
// with existing Graph tool code.
func retryWithBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, operation func() error) error {
	return retry.RetryWithBackoff(ctx, maxRetries, baseDelay, operation)
}
