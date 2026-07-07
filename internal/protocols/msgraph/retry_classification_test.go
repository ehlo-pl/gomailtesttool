//go:build !integration
// +build !integration

package msgraph

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// newTestODataError builds a fake Graph API error. code and retryAfter are
// optional (empty string omits them); statusCode 0 leaves the status unset,
// mimicking responses where only the OData error code is populated.
func newTestODataError(statusCode int, code, retryAfter string) *odataerrors.ODataError {
	odataErr := odataerrors.NewODataError()
	mainError := odataerrors.NewMainError()
	message := "test error"
	mainError.SetMessage(&message)
	if code != "" {
		mainError.SetCode(&code)
	}
	odataErr.SetErrorEscaped(mainError)
	if statusCode != 0 {
		odataErr.SetStatusCode(statusCode)
	}
	if retryAfter != "" {
		odataErr.GetResponseHeaders().Add("Retry-After", retryAfter)
	}
	return odataErr
}

func TestIsRetryableGraphError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantRetryable  bool
		wantRetryAfter time.Duration
	}{
		{
			name:           "429 by status code",
			err:            newTestODataError(429, "", ""),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "TooManyRequests by code without status",
			err:            newTestODataError(0, "TooManyRequests", ""),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "activityLimitReached by code",
			err:            newTestODataError(0, "activityLimitReached", ""),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "429 with Retry-After header",
			err:            newTestODataError(429, "TooManyRequests", "17"),
			wantRetryable:  true,
			wantRetryAfter: 17 * time.Second,
		},
		{
			name:           "503 service unavailable",
			err:            newTestODataError(503, "ServiceUnavailable", ""),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "504 gateway timeout",
			err:            newTestODataError(504, "GatewayTimeout", ""),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:           "wrapped 429 still detected via errors.As",
			err:            fmt.Errorf("error fetching calendar: %w", newTestODataError(429, "TooManyRequests", "3")),
			wantRetryable:  true,
			wantRetryAfter: 3 * time.Second,
		},
		{
			name:          "401 auth failure is permanent",
			err:           newTestODataError(401, "InvalidAuthenticationToken", ""),
			wantRetryable: false,
		},
		{
			name:          "404 not found is permanent",
			err:           newTestODataError(404, "ErrorItemNotFound", ""),
			wantRetryable: false,
		},
		{
			name:          "400 bad request is permanent",
			err:           newTestODataError(400, "BadRequest", ""),
			wantRetryable: false,
		},
		{
			name:           "non-OData network timeout falls back to generic classification",
			err:            errors.New("dial tcp: i/o timeout"),
			wantRetryable:  true,
			wantRetryAfter: 0,
		},
		{
			name:          "non-OData permanent error",
			err:           errors.New("certificate signed by unknown authority"),
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
			retryable, retryAfter := isRetryableGraphError(tt.err)
			if retryable != tt.wantRetryable {
				t.Errorf("isRetryableGraphError() retryable = %v, want %v", retryable, tt.wantRetryable)
			}
			if retryable && retryAfter != tt.wantRetryAfter {
				t.Errorf("isRetryableGraphError() retryAfter = %v, want %v", retryAfter, tt.wantRetryAfter)
			}
		})
	}
}

func TestGraphRetryAfter_InvalidValues(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		want       time.Duration
	}{
		{name: "no header", retryAfter: "", want: 0},
		{name: "non-numeric value", retryAfter: "Wed, 21 Oct 2026 07:28:00 GMT", want: 0},
		{name: "negative value", retryAfter: "-5", want: 0},
		{name: "zero value", retryAfter: "0", want: 0},
		{name: "valid seconds with whitespace", retryAfter: " 42 ", want: 42 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			odataErr := newTestODataError(429, "TooManyRequests", tt.retryAfter)
			if got := graphRetryAfter(odataErr); got != tt.want {
				t.Errorf("graphRetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRetryWithBackoff_RetriesGraphThrottling is the regression test for the
// original bug: a Graph 429 ODataError must actually be retried by
// retryWithBackoff instead of failing on the first attempt.
func TestRetryWithBackoff_RetriesGraphThrottling(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), 3, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return newTestODataError(429, "TooManyRequests", "")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("retryWithBackoff() = %v, want nil after throttling clears", err)
	}
	if calls != 3 {
		t.Errorf("operation called %d times, want 3 (429s must be retried)", calls)
	}
}

func TestRetryWithBackoff_DoesNotRetryGraphAuthFailure(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return newTestODataError(401, "InvalidAuthenticationToken", "")
	})

	if err == nil {
		t.Fatal("retryWithBackoff() = nil, want error")
	}
	if calls != 1 {
		t.Errorf("operation called %d times, want 1 (auth failures must not be retried)", calls)
	}
}
