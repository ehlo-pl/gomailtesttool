//go:build !integration
// +build !integration

package msgraph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TestIsRetryableGraphErrorOrEmptyResult(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantRetryable  bool
		wantRetryAfter time.Duration
	}{
		{
			name:           "empty-result sentinel is retryable",
			err:            errResultNotYetVisible,
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "wrapped sentinel still detected via errors.Is",
			err:            fmt.Errorf("operation failed after 3 retries: %w", errResultNotYetVisible),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "429 with Retry-After delegates to graph classification",
			err:            newTestODataError(429, "TooManyRequests", "17"),
			wantRetryable:  true,
			wantRetryAfter: 17 * time.Second,
		},
		{
			name:          "401 auth failure is permanent",
			err:           newTestODataError(401, "InvalidAuthenticationToken", ""),
			wantRetryable: false,
		},
		{
			name:          "context cancellation is never retryable",
			err:           context.Canceled,
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable, retryAfter := isRetryableGraphErrorOrEmptyResult(tt.err)
			if retryable != tt.wantRetryable {
				t.Errorf("isRetryableGraphErrorOrEmptyResult() retryable = %v, want %v", retryable, tt.wantRetryable)
			}
			if retryable && retryAfter != tt.wantRetryAfter {
				t.Errorf("isRetryableGraphErrorOrEmptyResult() retryAfter = %v, want %v", retryAfter, tt.wantRetryAfter)
			}
		})
	}
}

func TestFetchMessagesWithRetry_EmptyThenFound(t *testing.T) {
	calls := 0
	messages, err := fetchMessagesWithRetry(context.Background(), 3, time.Millisecond, "test", func() ([]models.Messageable, error) {
		calls++
		if calls < 3 {
			return []models.Messageable{}, nil
		}
		return []models.Messageable{models.NewMessage()}, nil
	})

	if err != nil {
		t.Fatalf("fetchMessagesWithRetry() error = %v, want nil once message becomes visible", err)
	}
	if len(messages) != 1 {
		t.Errorf("got %d messages, want 1", len(messages))
	}
	if calls != 3 {
		t.Errorf("fetch called %d times, want 3 (empty results must be retried)", calls)
	}
}

func TestFetchMessagesWithRetry_EmptyForeverReturnsNotFound(t *testing.T) {
	calls := 0
	messages, err := fetchMessagesWithRetry(context.Background(), 2, time.Millisecond, "test", func() ([]models.Messageable, error) {
		calls++
		return []models.Messageable{}, nil
	})

	if err != nil {
		t.Fatalf("fetchMessagesWithRetry() error = %v, want nil (not-found is not an error)", err)
	}
	if len(messages) != 0 {
		t.Errorf("got %d messages, want 0", len(messages))
	}
	if calls != 3 {
		t.Errorf("fetch called %d times, want 3 (maxRetries+1 attempts before giving up)", calls)
	}
}

func TestFetchMessagesWithRetry_ZeroRetriesSingleAttempt(t *testing.T) {
	calls := 0
	messages, err := fetchMessagesWithRetry(context.Background(), 0, time.Millisecond, "test", func() ([]models.Messageable, error) {
		calls++
		return []models.Messageable{}, nil
	})

	if err != nil {
		t.Fatalf("fetchMessagesWithRetry() error = %v, want nil", err)
	}
	if len(messages) != 0 {
		t.Errorf("got %d messages, want 0", len(messages))
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, want 1 (maxRetries=0 keeps single-attempt semantics)", calls)
	}
}

func TestFetchMessagesWithRetry_PermanentErrorNotMasked(t *testing.T) {
	calls := 0
	_, err := fetchMessagesWithRetry(context.Background(), 3, time.Millisecond, "test", func() ([]models.Messageable, error) {
		calls++
		return nil, newTestODataError(401, "InvalidAuthenticationToken", "")
	})

	if err == nil {
		t.Fatal("fetchMessagesWithRetry() = nil, want error (auth failures must not be swallowed as not-found)")
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, want 1 (auth failures must not be retried)", calls)
	}
}

func TestFetchMessagesWithRetry_ThrottleThenFound(t *testing.T) {
	calls := 0
	messages, err := fetchMessagesWithRetry(context.Background(), 3, time.Millisecond, "test", func() ([]models.Messageable, error) {
		calls++
		if calls == 1 {
			return nil, newTestODataError(429, "TooManyRequests", "")
		}
		return []models.Messageable{models.NewMessage()}, nil
	})

	if err != nil {
		t.Fatalf("fetchMessagesWithRetry() error = %v, want nil after throttling clears", err)
	}
	if len(messages) != 1 {
		t.Errorf("got %d messages, want 1", len(messages))
	}
	if calls != 2 {
		t.Errorf("fetch called %d times, want 2 (429s must still be retried)", calls)
	}
}

func TestFetchMessagesWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := fetchMessagesWithRetry(ctx, 3, 50*time.Millisecond, "test", func() ([]models.Messageable, error) {
		calls++
		cancel() // abort during the backoff wait after this empty attempt
		return []models.Messageable{}, nil
	})

	if err == nil {
		t.Fatal("fetchMessagesWithRetry() = nil, want cancellation error (must not be reported as not-found)")
	}
	if !strings.Contains(err.Error(), "cancelled") && !errors.Is(err, context.Canceled) {
		t.Errorf("fetchMessagesWithRetry() error = %v, want context cancellation", err)
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, want 1 (cancellation must stop retries)", calls)
	}
}
