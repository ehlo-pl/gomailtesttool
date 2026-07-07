//go:build !integration
// +build !integration

package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "context canceled", err: context.Canceled, want: false},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: false},
		{name: "wrapped context canceled", err: fmt.Errorf("dial failed: %w", context.Canceled), want: false},
		{name: "i/o timeout", err: errors.New("read tcp 1.2.3.4:25: i/o timeout"), want: true},
		{name: "connection reset", err: errors.New("connection reset by peer"), want: true},
		{name: "connection refused", err: errors.New("dial tcp: connection refused"), want: true},
		{name: "no such host", err: errors.New("lookup mail.example.com: no such host"), want: true},
		{name: "auth failure", err: errors.New("535 authentication credentials invalid"), want: false},
		{name: "generic permanent error", err: errors.New("550 mailbox unavailable"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.want {
				t.Errorf("IsRetryableError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsSMTPRetryableError(t *testing.T) {
	tests := []struct {
		name string
		code int
		want bool
	}{
		{name: "250 success", code: 250, want: false},
		{name: "354 intermediate", code: 354, want: false},
		{name: "421 service not available", code: 421, want: true},
		{name: "451 temporary failure", code: 451, want: true},
		{name: "550 permanent failure", code: 550, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSMTPRetryableError(tt.code); got != tt.want {
				t.Errorf("IsSMTPRetryableError(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestNextDelay(t *testing.T) {
	tests := []struct {
		name       string
		attempt    int
		baseDelay  time.Duration
		retryAfter time.Duration
		want       time.Duration
	}{
		{name: "first attempt uses base delay", attempt: 0, baseDelay: 2 * time.Second, want: 2 * time.Second},
		{name: "delay doubles per attempt", attempt: 2, baseDelay: 2 * time.Second, want: 8 * time.Second},
		{name: "backoff capped at 30s", attempt: 10, baseDelay: 2 * time.Second, want: 30 * time.Second},
		{name: "retry-after overrides backoff", attempt: 0, baseDelay: 2 * time.Second, retryAfter: 45 * time.Second, want: 45 * time.Second},
		{name: "retry-after capped at 5 minutes", attempt: 0, baseDelay: 2 * time.Second, retryAfter: time.Hour, want: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextDelay(tt.attempt, tt.baseDelay, tt.retryAfter); got != tt.want {
				t.Errorf("nextDelay(%d, %v, %v) = %v, want %v", tt.attempt, tt.baseDelay, tt.retryAfter, got, tt.want)
			}
		})
	}
}

func TestRetryWithBackoff_SuccessAfterRetries(t *testing.T) {
	calls := 0
	err := RetryWithBackoff(context.Background(), 3, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("i/o timeout")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("RetryWithBackoff() = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("operation called %d times, want 3", calls)
	}
}

func TestRetryWithBackoff_NonRetryableFailsImmediately(t *testing.T) {
	calls := 0
	permanent := errors.New("535 authentication failed")
	err := RetryWithBackoff(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return permanent
	})

	if !errors.Is(err, permanent) {
		t.Errorf("RetryWithBackoff() = %v, want %v", err, permanent)
	}
	if calls != 1 {
		t.Errorf("operation called %d times, want 1 (no retries for permanent errors)", calls)
	}
}

func TestRetryWithBackoff_ExhaustsRetries(t *testing.T) {
	calls := 0
	transient := errors.New("connection reset by peer")
	err := RetryWithBackoff(context.Background(), 2, time.Millisecond, func() error {
		calls++
		return transient
	})

	if err == nil {
		t.Fatal("RetryWithBackoff() = nil, want error after exhausting retries")
	}
	if !errors.Is(err, transient) {
		t.Errorf("RetryWithBackoff() = %v, want wrapped %v", err, transient)
	}
	if !strings.Contains(err.Error(), "failed after 2 retries") {
		t.Errorf("RetryWithBackoff() error = %q, want mention of retry exhaustion", err)
	}
	if calls != 3 {
		t.Errorf("operation called %d times, want 3 (initial + 2 retries)", calls)
	}
}

func TestRetryWithBackoff_ContextCancelledDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := RetryWithBackoff(ctx, 3, time.Minute, func() error {
		calls++
		return errors.New("i/o timeout")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("RetryWithBackoff() = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("operation called %d times, want 1 (cancelled during first wait)", calls)
	}
}

func TestRetryWithBackoffFunc_CustomClassifier(t *testing.T) {
	calls := 0
	throttled := errors.New("TooManyRequests")
	classify := func(err error) (bool, time.Duration) {
		return err.Error() == "TooManyRequests", 5 * time.Millisecond
	}

	start := time.Now()
	err := RetryWithBackoffFunc(context.Background(), 3, time.Millisecond, func() error {
		calls++
		if calls < 2 {
			return throttled
		}
		return nil
	}, classify)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RetryWithBackoffFunc() = %v, want nil", err)
	}
	if calls != 2 {
		t.Errorf("operation called %d times, want 2", calls)
	}
	if elapsed < 5*time.Millisecond {
		t.Errorf("elapsed %v, want >= 5ms (classifier retry-after honored)", elapsed)
	}
}

func TestRetryWithBackoffFunc_ClassifierRejects(t *testing.T) {
	calls := 0
	// An error the default string matcher would retry, rejected by the classifier.
	err := RetryWithBackoffFunc(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return errors.New("i/o timeout")
	}, func(error) (bool, time.Duration) { return false, 0 })

	if err == nil {
		t.Fatal("RetryWithBackoffFunc() = nil, want error")
	}
	if calls != 1 {
		t.Errorf("operation called %d times, want 1 (classifier said non-retryable)", calls)
	}
}
