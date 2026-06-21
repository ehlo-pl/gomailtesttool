// Package protocol provides POP3 protocol response parsing utilities.
package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// DefaultResponseTimeout is the default timeout for reading POP3 responses.
// This prevents indefinite hangs when communicating with misbehaving servers.
const DefaultResponseTimeout = 30 * time.Second

// POP3Response represents a POP3 server response.
type POP3Response struct {
	Success bool     // true for +OK, false for -ERR
	Message string   // Response message after +OK/-ERR
	Lines   []string // Additional lines for multiline responses
}

// IsSuccess returns true if the response indicates success.
func (r *POP3Response) IsSuccess() bool {
	return r.Success
}

// Error returns the error message if the response indicates failure.
func (r *POP3Response) Error() string {
	if r.Success {
		return ""
	}
	return r.Message
}

// ReadResponse reads a single-line POP3 response.
// POP3 responses start with +OK or -ERR followed by optional text.
func ReadResponse(reader *bufio.Reader) (*POP3Response, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseResponseLine(line)
}

// ReadResponseWithTimeout reads and parses a POP3 response with a timeout.
//
// The timeout is enforced via conn.SetReadDeadline rather than a goroutine, so
// no goroutine is left blocked on a read after this function returns. The read
// deadline is cleared before returning so it doesn't affect later operations
// on conn.
func ReadResponseWithTimeout(conn net.Conn, reader *bufio.Reader, timeout time.Duration) (*POP3Response, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}
	defer conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	resp, err := ReadResponse(reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("timeout waiting for POP3 response after %v", timeout)
		}
		return nil, err
	}
	return resp, nil
}

// ReadMultilineResponse reads a multiline POP3 response.
// Multiline responses end with a line containing only "."
// Lines starting with "." have the leading dot removed (dot-stuffing).
func ReadMultilineResponse(reader *bufio.Reader) (*POP3Response, error) {
	// First read the status line
	resp, err := ReadResponse(reader)
	if err != nil {
		return nil, err
	}

	// If it's an error response, return immediately
	if !resp.Success {
		return resp, nil
	}

	// Read the multiline body
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read multiline response: %w", err)
		}

		// Trim CRLF
		line = strings.TrimRight(line, "\r\n")

		// Check for termination line
		if line == "." {
			break
		}

		// Handle dot-stuffing (lines starting with ".." become ".")
		if strings.HasPrefix(line, "..") {
			line = line[1:]
		}

		lines = append(lines, line)
	}

	resp.Lines = lines
	return resp, nil
}

// parseResponseLine parses a single POP3 response line.
func parseResponseLine(line string) (*POP3Response, error) {
	line = strings.TrimRight(line, "\r\n")

	resp := &POP3Response{}

	if strings.HasPrefix(line, "+OK") {
		resp.Success = true
		if len(line) > 3 {
			resp.Message = strings.TrimPrefix(line[3:], " ")
		}
	} else if strings.HasPrefix(line, "-ERR") {
		resp.Success = false
		if len(line) > 4 {
			resp.Message = strings.TrimPrefix(line[4:], " ")
		}
	} else if strings.HasPrefix(line, "+ ") {
		// Continuation response (used in SASL AUTH)
		resp.Success = true
		resp.Message = strings.TrimPrefix(line, "+ ")
	} else {
		return nil, fmt.Errorf("invalid POP3 response: %s", line)
	}

	return resp, nil
}

// ParseGreeting extracts the APOP timestamp from a POP3 greeting.
// The timestamp is enclosed in angle brackets: <timestamp@host>
// Returns empty string if no timestamp is found.
func ParseGreeting(greeting string) (timestamp string) {
	start := strings.Index(greeting, "<")
	if start == -1 {
		return ""
	}
	end := strings.Index(greeting[start:], ">")
	if end == -1 {
		return ""
	}
	return greeting[start : start+end+1]
}

// ParseStatResponse parses a STAT response.
// Format: +OK <count> <size>
func ParseStatResponse(resp *POP3Response) (count int, size int64, err error) {
	if !resp.Success {
		return 0, 0, fmt.Errorf("STAT failed: %s", resp.Message)
	}

	_, err = fmt.Sscanf(resp.Message, "%d %d", &count, &size)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse STAT response: %w", err)
	}

	return count, size, nil
}

// MessageInfo represents information about a single message.
type MessageInfo struct {
	Number int
	Size   int64
	UIDL   string
}

// ParseListResponse parses a LIST multiline response.
// Each line has format: <msg-num> <size>
func ParseListResponse(resp *POP3Response) ([]MessageInfo, error) {
	if !resp.Success {
		return nil, fmt.Errorf("LIST failed: %s", resp.Message)
	}

	var messages []MessageInfo
	for _, line := range resp.Lines {
		var num int
		var size int64
		_, err := fmt.Sscanf(line, "%d %d", &num, &size)
		if err != nil {
			continue // Skip malformed lines
		}
		messages = append(messages, MessageInfo{Number: num, Size: size})
	}

	return messages, nil
}

// ParseUIDLResponse parses a UIDL multiline response.
// Each line has format: <msg-num> <unique-id>
func ParseUIDLResponse(resp *POP3Response) ([]MessageInfo, error) {
	if !resp.Success {
		return nil, fmt.Errorf("UIDL failed: %s", resp.Message)
	}

	var messages []MessageInfo
	for _, line := range resp.Lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}
		var num int
		_, err := fmt.Sscanf(parts[0], "%d", &num)
		if err != nil {
			continue
		}
		messages = append(messages, MessageInfo{Number: num, UIDL: parts[1]})
	}

	return messages, nil
}
