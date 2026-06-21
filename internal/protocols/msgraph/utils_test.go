//go:build !integration
// +build !integration

package msgraph

import (
	"strings"
	"testing"
)

// TestValidateMessageID tests Message-ID validation with focus on OData injection prevention (CRITICAL SECURITY)
func TestValidateMessageID(t *testing.T) {
	tests := []struct {
		name    string
		msgID   string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"Valid: Standard RFC 5322 format", "<CABcD1234@mail.example.com>", false, ""},
		{"Valid: Complex message ID", "<20240101120000.12345@exchange.example.com>", false, ""},
		{"Valid: Short message ID", "<a@b.com>", false, ""},
		{"Valid: Message ID with dots", "<user.name.123@mail.server.example.com>", false, ""},
		{"Valid: Message ID with hyphens", "<msg-id-123@example.com>", false, ""},
		{"Valid: Message ID with underscores", "<msg_id_123@example.com>", false, ""},

		// Invalid format
		{"Error: Empty message ID", "", true, "empty"},
		{"Error: Missing angle brackets", "msg-id@example.com", true, "angle brackets"},
		{"Error: Missing opening bracket", "msg-id@example.com>", true, "angle brackets"},
		{"Error: Missing closing bracket", "<msg-id@example.com", true, "angle brackets"},

		// Length validation
		{"Error: Max length exceeded", "<" + strings.Repeat("a", 997) + ">", true, "maximum length"},

		// Security: Quote character injection (CRITICAL)
		{"Security: Single quote", "<msg'id@example.com>", true, "invalid characters"},
		{"Security: Double quote", "<msg\"id@example.com>", true, "invalid characters"},
		{"Security: Backslash", "<msg\\id@example.com>", true, "invalid characters"},
		{"Security: Multiple quotes", "<'msg\"id\\@example.com>", true, "invalid characters"},

		// Security: OData OR operator injection (CRITICAL)
		{"Security: OR operator lowercase", "<msg or 1 eq 1@example.com>", true, "OData operators"},
		{"Security: OR operator uppercase", "<msg OR 1 eq 1@example.com>", true, "OData operators"},
		{"Security: OR operator mixed case", "<msg Or 1 eq 1@example.com>", true, "OData operators"},
		{"Security: OR with filter manipulation", "<test or internetMessageId eq evil@example.com>", true, "OData operators"},

		// Security: OData AND operator injection (CRITICAL)
		{"Security: AND operator lowercase", "<msg and isRead eq false@example.com>", true, "OData operators"},
		{"Security: AND operator uppercase", "<msg AND true@example.com>", true, "OData operators"},
		{"Security: AND operator mixed case", "<msg And true@example.com>", true, "OData operators"},

		// Security: OData EQ operator injection (CRITICAL)
		{"Security: EQ operator lowercase", "<msg eq malicious@example.com>", true, "OData operators"},
		{"Security: EQ operator uppercase", "<msg EQ evil@example.com>", true, "OData operators"},
		{"Security: EQ operator mixed case", "<msg Eq bad@example.com>", true, "OData operators"},

		// Security: OData NE operator injection (CRITICAL)
		{"Security: NE operator lowercase", "<msg ne null@example.com>", true, "OData operators"},
		{"Security: NE operator uppercase", "<msg NE null@example.com>", true, "OData operators"},
		{"Security: NE operator mixed case", "<msg Ne null@example.com>", true, "OData operators"},

		// Security: OData LT operator injection (CRITICAL)
		{"Security: LT operator lowercase", "<msg lt 100@example.com>", true, "OData operators"},
		{"Security: LT operator uppercase", "<msg LT 100@example.com>", true, "OData operators"},
		{"Security: LT operator mixed case", "<msg Lt 100@example.com>", true, "OData operators"},

		// Security: OData GT operator injection (CRITICAL)
		{"Security: GT operator lowercase", "<msg gt 0@example.com>", true, "OData operators"},
		{"Security: GT operator uppercase", "<msg GT 0@example.com>", true, "OData operators"},
		{"Security: GT operator mixed case", "<msg Gt 0@example.com>", true, "OData operators"},

		// Security: OData LE operator injection (CRITICAL)
		{"Security: LE operator lowercase", "<msg le 100@example.com>", true, "OData operators"},
		{"Security: LE operator uppercase", "<msg LE 100@example.com>", true, "OData operators"},
		{"Security: LE operator mixed case", "<msg Le 100@example.com>", true, "OData operators"},

		// Security: OData GE operator injection (CRITICAL)
		{"Security: GE operator lowercase", "<msg ge 0@example.com>", true, "OData operators"},
		{"Security: GE operator uppercase", "<msg GE 0@example.com>", true, "OData operators"},
		{"Security: GE operator mixed case", "<msg Ge 0@example.com>", true, "OData operators"},

		// Security: OData NOT operator injection (CRITICAL)
		{"Security: NOT operator lowercase", "<msg not contains test@example.com>", true, "OData operators"},
		{"Security: NOT operator uppercase", "<msg NOT contains test@example.com>", true, "OData operators"},
		{"Security: NOT operator mixed case", "<msg Not contains test@example.com>", true, "OData operators"},

		// Security: Complex injection attempts (CRITICAL)
		{"Security: Multiple operators", "<msg or 1 eq 1 and true@example.com>", true, "OData operators"},
		{"Security: Nested injection", "<test or isRead eq false@example.com>", true, "OData operators"},
		{"Security: Combined quote and operator", "<msg'test or 1 eq 1@example.com>", true, "invalid characters"},

		// Edge cases: Valid strings that contain operator-like substrings
		{"Valid: Email with 'or' in domain", "<user@corporate.com>", false, ""},
		{"Valid: Email with 'and' in local part", "<andrew@example.com>", false, ""},
		{"Valid: Message ID with 'eq' substring", "<equipment123@example.com>", false, ""},
		{"Valid: Message ID with 'not' substring", "<notification@example.com>", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageID(tt.msgID)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMessageID(%q) error = %v, wantErr %v", tt.msgID, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("validateMessageID(%q) error message = %v, should contain %v", tt.msgID, err.Error(), tt.errMsg)
			}
		})
	}
}

// TestIfEmpty tests the conditional string helper
func TestIfEmpty(t *testing.T) {
	tests := []struct {
		name       string
		str        string
		defaultVal string
		expected   string
	}{
		{"Empty string returns default", "", "default", "default"},
		{"Non-empty string returns original", "value", "default", "value"},
		{"Empty default", "", "", ""},
		{"Both non-empty", "value", "default", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ifEmpty(tt.str, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("ifEmpty(%q, %q) = %q, want %q", tt.str, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

// TestTruncate tests string truncation with ellipsis
func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		maxLen   int
		expected string
	}{
		{"Short string (no truncation)", "hello", 10, "hello"},
		{"Exact length (no truncation)", "hello", 5, "hello"},
		{"Truncate long string", "this is a long string", 10, "this is a ..."},
		{"Truncate to zero", "hello", 0, "..."},
		{"Truncate to 1", "hello", 1, "h..."},
		{"Empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.str, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.str, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// TestInt32Ptr tests the Int32 pointer helper
func TestInt32Ptr(t *testing.T) {
	tests := []struct {
		name  string
		value int32
	}{
		{"Zero value", 0},
		{"Positive value", 42},
		{"Negative value", -10},
		{"Max int32", 2147483647},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int32Ptr(tt.value)
			if result == nil {
				t.Errorf("Int32Ptr(%d) returned nil", tt.value)
				return
			}
			if *result != tt.value {
				t.Errorf("Int32Ptr(%d) = %d, want %d", tt.value, *result, tt.value)
			}
		})
	}
}
