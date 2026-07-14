package msgraph

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/retry"
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

// ifEmpty returns defaultVal if s is empty, otherwise returns s
func ifEmpty(s, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	return s
}

// derefOr dereferences a *string, returning fallback when the pointer is nil.
// Graph SDK getters return nil for absent fields (e.g. a message with no
// subject), so use this before dereferencing values that may be unset.
func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
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

// validateSearchSubject validates a subject search pattern used in an OData
// contains() filter. Single quotes are escaped (doubled) by the caller before
// being embedded in the filter, so this only rejects empty/oversized input and
// control characters that have no place in an email subject.
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

// isRetryableGraphError classifies Graph API errors for retry purposes.
// Throttling (429 TooManyRequests/activityLimitReached) and transient service
// errors (503 ServiceUnavailable, 504 GatewayTimeout) are retryable, honoring
// the Retry-After response header when present. Other OData errors (auth
// failures, 4xx) are permanent. Non-OData errors fall back to the generic
// network-error classification.
func isRetryableGraphError(err error) (bool, time.Duration) {
	var odataErr *odataerrors.ODataError
	if !errors.As(err, &odataErr) {
		return retry.IsRetryableError(err), 0
	}

	code := ""
	if errorInfo := odataErr.GetErrorEscaped(); errorInfo != nil && errorInfo.GetCode() != nil {
		code = *errorInfo.GetCode()
	}

	switch {
	case odataErr.ResponseStatusCode == 429 || code == "TooManyRequests" || code == "activityLimitReached",
		odataErr.ResponseStatusCode == 503 || code == "ServiceUnavailable",
		odataErr.ResponseStatusCode == 504 || code == "GatewayTimeout":
		return true, graphRetryAfter(odataErr)
	}

	return false, 0
}

// graphRetryAfter extracts the Retry-After header (delay in seconds) from a
// Graph API error response. Returns 0 when absent or unparsable, which makes
// the retry loop fall back to exponential backoff.
func graphRetryAfter(odataErr *odataerrors.ODataError) time.Duration {
	headers := odataErr.GetResponseHeaders()
	if headers == nil {
		return 0
	}
	values := headers.Get("Retry-After")
	if len(values) == 0 {
		return 0
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// retryWithBackoff wraps the common retry package with Graph-aware error
// classification, so throttling and transient service errors are actually
// retried (and Retry-After is honored) instead of failing on the first attempt.
func retryWithBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, operation func() error) error {
	return retry.RetryWithBackoffFunc(ctx, maxRetries, baseDelay, operation, isRetryableGraphError)
}

// errResultNotYetVisible signals that a mailbox query succeeded but returned
// no messages. Graph is eventually consistent: a just-sent message may not be
// indexed yet, so this condition is retried like a transient error.
var errResultNotYetVisible = errors.New("no messages matched the filter (message may not be indexed yet)")

// isRetryableGraphErrorOrEmptyResult extends isRetryableGraphError so an
// empty (but successful) result set is retried with exponential backoff,
// while real Graph errors keep their existing classification and
// Retry-After handling.
func isRetryableGraphErrorOrEmptyResult(err error) (bool, time.Duration) {
	if errors.Is(err, errResultNotYetVisible) {
		return true, 0
	}
	return isRetryableGraphError(err)
}

// fetchMessagesWithRetry runs fetch under Graph-aware retry logic and
// additionally retries when the result set is empty, because Graph is
// eventually consistent and a just-delivered message may not be visible to
// $filter queries yet. Exhausting retries on an empty result is NOT an
// error: it returns (nil, nil) and the caller reports "no messages found"
// as before. Real API errors, network errors, and context cancellation
// propagate.
func fetchMessagesWithRetry(ctx context.Context, maxRetries int, baseDelay time.Duration, operation string, fetch func() ([]models.Messageable, error)) ([]models.Messageable, error) {
	var messages []models.Messageable
	attempt := 0
	err := retry.RetryWithBackoffFunc(ctx, maxRetries, baseDelay, func() error {
		attempt++
		result, apiErr := fetch()
		if apiErr != nil {
			return apiErr
		}
		messages = result
		// On the final attempt an empty result is accepted as the answer
		// (return nil, not the sentinel) so the retry loop does not log a
		// misleading failure for an ordinary not-found outcome.
		if len(messages) == 0 && attempt <= maxRetries {
			log.Printf("[INFO] %s: no matching messages yet (attempt %d/%d); message may not be indexed yet, retrying...", operation, attempt, maxRetries+1)
			return errResultNotYetVisible
		}
		return nil
	}, isRetryableGraphErrorOrEmptyResult)
	if errors.Is(err, errResultNotYetVisible) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return messages, nil
}
