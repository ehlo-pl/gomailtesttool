package security

import (
	"testing"
)

func TestMaskUsername(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "us****om"},
		{"ab@example.com", "ab****om"},
		{"test", "****"},
		{"ab", "****"},
		{"a", "****"},
		{"abcde", "ab****de"},
	}

	for _, tt := range tests {
		result := MaskUsername(tt.input)
		if result != tt.expected {
			t.Errorf("MaskUsername(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMaskPassword(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"secretpassword", "se****rd"},
		{"123456789", "12****89"},
		{"password", "****"},
		{"test", "****"},
		{"ab", "****"},
		{"", ""},
		{"abcde", "****"},
	}

	for _, tt := range tests {
		result := MaskPassword(tt.input)
		if result != tt.expected {
			t.Errorf("MaskPassword(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMaskAccessToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ya29.a0ARrdaM_1234567890abcdefghij", "ya29.a0A...ghij"},
		{"1234567890123456", "12****56"},
		{"short", "****"},
		{"1234", "****"},
		{"abc", "****"},
		{"ab", "****"},
		{"a", "****"},
		{"", ""},
		{"12345678901234567", "12345678...4567"},
	}

	for _, tt := range tests {
		result := MaskAccessToken(tt.input)
		if result != tt.expected {
			t.Errorf("MaskAccessToken(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-long-secret-value", "my-l****"},
		{"shrt", "****"},
		{"ab", "****"},
		{"", ""},
		{"abcde", "abcd****"},
	}

	for _, tt := range tests {
		result := MaskSecret(tt.input)
		if result != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMaskGUID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"12345678-1234-1234-1234-123456789012", "12345678****"},
		{"short", "short****"},
		{"12345678", "12345678****"},
		{"1234567890", "12345678****"},
	}

	for _, tt := range tests {
		result := MaskGUID(tt.input)
		if result != tt.expected {
			t.Errorf("MaskGUID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "us****@ex****"},
		{"a@b.com", "****@b.****"},
		{"ab@cd.com", "****@cd****"},
		{"longuser@longdomain.com", "lo****@lo****"},
		{"", ""},
		{"noemail", "no****il"}, // Treated as username
	}

	for _, tt := range tests {
		result := MaskEmail(tt.input)
		if result != tt.expected {
			t.Errorf("MaskEmail(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
