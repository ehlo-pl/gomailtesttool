package protocol

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

func newPipeConn(t *testing.T) (net.Conn, *bufio.Reader, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, bufio.NewReader(client), server
}

func TestReadResponseWithTimeout_Success(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		timeout     time.Duration
		wantSuccess bool
		wantMessage string
	}{
		{
			name:        "Valid +OK response within timeout",
			input:       "+OK done\r\n",
			timeout:     1 * time.Second,
			wantSuccess: true,
			wantMessage: "done",
		},
		{
			name:        "Valid -ERR response within timeout",
			input:       "-ERR no such mailbox\r\n",
			timeout:     1 * time.Second,
			wantSuccess: false,
			wantMessage: "no such mailbox",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, reader, server := newPipeConn(t)
			go func() {
				_, _ = server.Write([]byte(tt.input))
			}()

			resp, err := ReadResponseWithTimeout(client, reader, tt.timeout)
			if err != nil {
				t.Fatalf("ReadResponseWithTimeout() unexpected error: %v", err)
			}

			if resp.Success != tt.wantSuccess {
				t.Errorf("ReadResponseWithTimeout() Success = %v, want %v", resp.Success, tt.wantSuccess)
			}

			if resp.Message != tt.wantMessage {
				t.Errorf("ReadResponseWithTimeout() Message = %q, want %q", resp.Message, tt.wantMessage)
			}
		})
	}
}

func TestReadResponseWithTimeout_Timeout(t *testing.T) {
	// Server side never writes, simulating a hanging server.
	client, reader, _ := newPipeConn(t)

	timeout := 50 * time.Millisecond

	start := time.Now()
	resp, err := ReadResponseWithTimeout(client, reader, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ReadResponseWithTimeout() expected timeout error, got nil")
	}

	if resp != nil {
		t.Errorf("ReadResponseWithTimeout() expected nil response on timeout, got %v", resp)
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("ReadResponseWithTimeout() error should contain 'timeout', got: %v", err)
	}

	if elapsed < timeout || elapsed > timeout+200*time.Millisecond {
		t.Errorf("ReadResponseWithTimeout() elapsed time = %v, want approximately %v", elapsed, timeout)
	}
}

func TestReadResponseWithTimeout_DeadlineCleared(t *testing.T) {
	client, reader, server := newPipeConn(t)

	// First read times out.
	if _, err := ReadResponseWithTimeout(client, reader, 50*time.Millisecond); err == nil {
		t.Fatal("expected timeout error on first read")
	}

	// A subsequent read without a deadline should succeed once data arrives,
	// proving the earlier deadline was cleared.
	go func() {
		_, _ = server.Write([]byte("+OK ready\r\n"))
	}()

	resp, err := ReadResponseWithTimeout(client, reader, 1*time.Second)
	if err != nil {
		t.Fatalf("ReadResponseWithTimeout() unexpected error after deadline clear: %v", err)
	}

	if !resp.Success || resp.Message != "ready" {
		t.Errorf("ReadResponseWithTimeout() = %+v, want Success=true Message=%q", resp, "ready")
	}
}
