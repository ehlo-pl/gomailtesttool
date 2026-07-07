//go:build !integration
// +build !integration

package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateFilePath tests file path validation including security checks for path traversal attacks
func TestValidateFilePath(t *testing.T) {
	// Create a temporary test file for valid path tests
	tmpFile, err := os.CreateTemp("", "validation_test_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Create a temporary directory (should fail - not a regular file)
	tmpDir, err := os.MkdirTemp("", "validation_test_dir_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tests := []struct {
		name      string
		path      string
		fieldName string
		wantErr   bool
		errMsg    string
	}{
		// Valid cases
		{"Valid: Empty path (optional field)", "", "TestFile", false, ""},
		{"Valid: Absolute path to temp file", tmpFile.Name(), "TestFile", false, ""},
		{"Valid: Relative path to current file", "validation_test.go", "TestFile", false, ""},

		// Security: Path traversal attacks (CRITICAL)
		{"Security: Unix path traversal (../../)", "../../etc/passwd", "TestFile", true, "not allowed"},
		{"Security: Windows path traversal (..\\..\\)", "..\\..\\Windows\\System32\\config", "TestFile", true, "not allowed"},
		{"Security: Mixed path traversal", "../../../sensitive", "TestFile", true, "not allowed"},
		{"Security: Multiple traversal attempts", "../../../../../../../../etc/shadow", "TestFile", true, "not allowed"},
		{"Security: Hidden traversal in path", "safe/../../etc/passwd", "TestFile", true, "not allowed"},

		// File not found
		{"Error: File does not exist", "/nonexistent/file/path.txt", "TestFile", true, "not found"},
		{"Error: Nonexistent file in temp", filepath.Join(os.TempDir(), "nonexistent_validation_test.txt"), "TestFile", true, "not found"},

		// Directory vs file
		{"Error: Path is directory not file", tmpDir, "TestFile", true, "not a regular file"},

		// Invalid paths
		{"Error: Invalid path characters (NUL)", "file\x00name.txt", "TestFile", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("ValidateFilePath() error message = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestValidateEmail tests email validation including security checks for injection attacks
func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"Valid: Simple email", "user@example.com", false, ""},
		{"Valid: Email with plus", "test.name+tag@example.co.uk", false, ""},
		{"Valid: Email with dots", "first.last@sub.domain.com", false, ""},
		{"Valid: Email with numbers", "user123@example456.com", false, ""},

		// Invalid format
		{"Error: Empty email", "", true, "empty"},
		{"Error: Missing @", "userexample.com", true, "missing @"},
		{"Error: Multiple @ symbols", "user@@example.com", true, "invalid"},
		{"Error: Empty local part", "@example.com", true, "invalid"},
		{"Error: Empty domain", "user@", true, "invalid"},

		// Security: Potential injection attempts
		{"Security: CRLF injection attempt", "user@example.com\r\nBcc: attacker@evil.com", true, "invalid"},
		{"Security: Newline injection", "user@example.com\nCc: leak@evil.com", true, "invalid"},

		// Whitespace handling
		{"Valid: Trimmed whitespace", "  user@example.com  ", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("ValidateEmail() error message = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestValidateEmails tests validation of email slices
func TestValidateEmails(t *testing.T) {
	tests := []struct {
		name      string
		emails    []string
		fieldName string
		wantErr   bool
	}{
		{"Valid: Empty slice", []string{}, "To", false},
		{"Valid: Single valid email", []string{"user@example.com"}, "To", false},
		{"Valid: Multiple valid emails", []string{"user1@example.com", "user2@example.com"}, "CC", false},
		{"Error: One invalid in list", []string{"valid@example.com", "invalid"}, "BCC", true},
		{"Error: Invalid email in middle", []string{"user1@example.com", "@invalid", "user3@example.com"}, "To", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmails(tt.emails, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmails() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateGUID tests GUID format validation
func TestValidateGUID(t *testing.T) {
	tests := []struct {
		name      string
		guid      string
		fieldName string
		wantErr   bool
		errMsg    string
	}{
		// Valid cases
		{"Valid: Standard GUID", "12345678-1234-1234-1234-123456789012", "TenantID", false, ""},
		{"Valid: Lowercase GUID", "abcdef12-3456-7890-abcd-ef1234567890", "ClientID", false, ""},
		{"Valid: Uppercase GUID", "ABCDEF12-3456-7890-ABCD-EF1234567890", "ClientID", false, ""},
		{"Valid: Mixed case GUID", "AaBbCcDd-1234-5678-90Ab-CdEf12345678", "TenantID", false, ""},

		// Invalid format
		{"Error: Empty GUID", "", "TenantID", true, "empty"},
		{"Error: Too short", "12345678-1234-1234-1234-12345678901", "TenantID", true, "36 characters"},
		{"Error: Too long", "12345678-1234-1234-1234-1234567890123", "ClientID", true, "36 characters"},
		{"Error: Missing dashes", "12345678123412341234123456789012", "TenantID", true, "36 characters"},
		{"Error: Wrong dash position 1", "1234567-81234-1234-1234-123456789012", "TenantID", true, "dashes at wrong positions"},
		{"Error: Wrong dash position 2", "12345678-123-41234-1234-123456789012", "TenantID", true, "dashes at wrong positions"},
		{"Error: Wrong dash position 3", "12345678-1234-123-41234-123456789012", "TenantID", true, "dashes at wrong positions"},
		{"Error: Wrong dash position 4", "12345678-1234-1234-123-4123456789012", "TenantID", true, "dashes at wrong positions"},

		// Whitespace handling
		{"Valid: Trimmed whitespace", "  12345678-1234-1234-1234-123456789012  ", "TenantID", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGUID(tt.guid, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("ValidateGUID() error message = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestValidateHostname tests hostname validation including DNS names and IP addresses
func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  bool
		errMsg   string
	}{
		// Valid DNS names
		{"Valid: Simple hostname", "example.com", false, ""},
		{"Valid: Subdomain", "mail.example.com", false, ""},
		{"Valid: Multiple subdomains", "smtp.mail.example.co.uk", false, ""},
		{"Valid: Hostname with hyphen", "mail-server.example.com", false, ""},
		{"Valid: Localhost", "localhost", false, ""},

		// Valid IPv4 addresses
		{"Valid: IPv4 localhost", "127.0.0.1", false, ""},
		{"Valid: IPv4 address", "192.168.1.1", false, ""},
		{"Valid: IPv4 public", "8.8.8.8", false, ""},

		// Valid IPv6 addresses
		{"Valid: IPv6 localhost", "::1", false, ""},
		{"Valid: IPv6 address", "2001:db8::1", false, ""},
		{"Valid: IPv6 full", "2001:0db8:0000:0000:0000:0000:0000:0001", false, ""},

		// Invalid cases
		{"Error: Empty hostname", "", true, "empty"},
		{"Error: Too long (>253 chars)", strings.Repeat("a", 254), true, "too long"},
		{"Error: Invalid character @", "host@example.com", true, "invalid character"},
		{"Error: Invalid character space", "host name.com", true, "invalid character"},
		{"Error: Starts with hyphen", "-hostname.com", true, "cannot start or end"},
		{"Error: Ends with hyphen", "hostname-", true, "cannot start or end"},
		{"Error: Starts with dot", ".hostname.com", true, "cannot start or end"},
		{"Error: Ends with dot", "hostname.com.", true, "cannot start or end"},

		// Whitespace handling
		{"Valid: Trimmed whitespace", "  example.com  ", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("ValidateHostname() error message = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// TestValidatePort tests port number validation
func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"Valid: SMTP port 25", 25, false},
		{"Valid: HTTP port 80", 80, false},
		{"Valid: HTTPS port 443", 443, false},
		{"Valid: Submission port 587", 587, false},
		{"Valid: Minimum port 1", 1, false},
		{"Valid: Maximum port 65535", 65535, false},
		{"Valid: High port", 8080, false},
		{"Error: Port 0", 0, true},
		{"Error: Negative port", -1, true},
		{"Error: Port too high", 65536, true},
		{"Error: Port far too high", 100000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateSMTPAddress tests SMTP address validation (RFC 5321 format)
func TestValidateSMTPAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{"Valid: Simple address", "user@example.com", false},
		{"Valid: Address with angle brackets", "<user@example.com>", false},
		{"Valid: Address with plus", "<test+tag@example.com>", false},
		{"Error: Empty address", "", true},
		{"Error: Invalid format", "not-an-email", true},
		{"Error: Missing domain", "user@", true},
		{"Error: Missing local part", "@example.com", true},
		{"Valid: Trimmed whitespace", "  user@example.com  ", false},
		{"Valid: Angle brackets with whitespace", "  <user@example.com>  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSMTPAddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSMTPAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
