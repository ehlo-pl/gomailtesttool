package retry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// IsRetryableError determines if an error is transient and worth retrying.
// Returns true for network timeouts, connection errors, and temporary failures.
// Returns false for context cancellation, permanent errors, and authentication failures.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context cancellation - never retry these
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check error message for common transient patterns
	errMsg := strings.ToLower(err.Error())
	transientPatterns := []string{
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"try again",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"broken pipe",
		"connection timed out",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// IsSMTPRetryableError determines if an SMTP error code is retryable.
// Returns true for 4xx temporary SMTP errors.
// Returns false for 5xx permanent SMTP errors and 2xx/3xx success codes.
func IsSMTPRetryableError(smtpCode int) bool {
	// 4xx codes are temporary failures - retry
	if smtpCode >= 400 && smtpCode < 500 {
		return true
	}
	// 5xx codes are permanent failures - don't retry
	// 2xx/3xx are success codes - don't retry
	return false
}

// Classifier decides whether an error is worth retrying. A returned
// retryAfter > 0 requests that specific delay before the next attempt
// (e.g. from an HTTP Retry-After header) instead of exponential backoff.
type Classifier func(err error) (retryable bool, retryAfter time.Duration)

const (
	// maxBackoffDelay caps the exponential backoff delay.
	maxBackoffDelay = 30 * time.Second
	// maxRetryAfterDelay caps a server-provided Retry-After delay so a
	// pathological header value cannot stall the tool indefinitely.
	maxRetryAfterDelay = 5 * time.Minute
)

// nextDelay computes the wait before the next attempt: a server-provided
// retryAfter (capped at maxRetryAfterDelay) when given, otherwise
// exponential backoff from baseDelay (capped at maxBackoffDelay).
func nextDelay(attempt int, baseDelay, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, maxRetryAfterDelay)
	}
	delay := baseDelay * time.Duration(1<<uint(attempt))
	if delay <= 0 || delay > maxBackoffDelay {
		// <= 0 guards against shift overflow with very large attempt counts
		delay = maxBackoffDelay
	}
	return delay
}

// RetryWithBackoff wraps an operation with exponential backoff retry logic.
// The operation is retried up to maxRetries times with exponentially increasing delays.
// Base delay doubles on each attempt (capped at 30 seconds).
// Context cancellation is respected and will stop retries immediately.
// Errors are classified with IsRetryableError; use RetryWithBackoffFunc to
// supply a custom classifier (e.g. protocol-aware status-code checks).
//
// Example usage:
//
//	err := retry.RetryWithBackoff(ctx, 3, 2*time.Second, func() error {
//	    return doSomethingThatMightFail()
//	})
func RetryWithBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, operation func() error) error {
	return RetryWithBackoffFunc(ctx, maxRetries, baseDelay, operation, func(err error) (bool, time.Duration) {
		return IsRetryableError(err), 0
	})
}

// RetryWithBackoffFunc is RetryWithBackoff with a caller-supplied Classifier,
// letting protocols recognize their own transient failures (e.g. Graph API
// 429/503 OData errors) and honor server-provided Retry-After delays.
func RetryWithBackoffFunc(ctx context.Context, maxRetries int, baseDelay time.Duration, operation func() error, classify Classifier) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Execute the operation
		lastErr = operation()

		// Success - return immediately
		if lastErr == nil {
			if attempt > 0 {
				log.Printf("Operation succeeded after %d retries", attempt)
			}
			return nil
		}

		// Check if error is retryable
		retryable, retryAfter := classify(lastErr)
		if !retryable {
			// Non-retryable error - fail immediately
			return lastErr
		}

		// Last attempt failed - return error
		if attempt == maxRetries {
			return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
		}

		delay := nextDelay(attempt, baseDelay, retryAfter)

		log.Printf("Retryable error encountered (attempt %d/%d): %v. Retrying in %v...",
			attempt+1, maxRetries, lastErr, delay)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next retry attempt
		}
	}

	return lastErr
}
