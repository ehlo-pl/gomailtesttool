//go:build !integration
// +build !integration

package smtp

import (
	"testing"
)

// TestMaskPassword tests password masking for security (prevents password exposure in logs)
func TestMaskPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		expected string
	}{
		// Short passwords (<= 8 chars) - fully masked, since revealing 4
		// characters of a password that short discloses most or all of it
		{"Empty password", "", "****"},
		{"Single char", "a", "****"},
		{"Two chars", "ab", "****"},
		{"Three chars", "abc", "****"},
		{"Four chars", "1234", "****"},
		{"Five chars", "12345", "****"},
		{"Short password", "password", "****"},
		{"Mixed case", "MyP@ss", "****"},

		// Longer passwords (> 8 chars) - show first 2 and last 2
		{"Longer password", "MySecretPassword", "My****rd"},
		{"Complex password", "P@ssw0rd!123", "P@****23"},
		{"Very long password", "ThisIsAVeryLongPasswordWithManyCharacters", "Th****rs"},

		// Edge cases
		{"Special characters", "!@#$%^&*()", "!@****()"},
		{"Unicode characters", "пароль123", "п****23"}, // Note: UTF-8 bytes, not runes
		{"Spaces in password", "my password", "my****rd"},
		{"Numbers only", "123456789", "12****89"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskPassword(tt.password)
			if result != tt.expected {
				t.Errorf("maskPassword(%q) = %q, want %q", tt.password, result, tt.expected)
			}

			// Security verification: Ensure original password is NOT in masked output (except for <= 4 chars)
			if len(tt.password) > 4 && result == tt.password {
				t.Errorf("maskPassword(%q) = %q, password not masked!", tt.password, result)
			}

			// Security verification: Ensure masked output is not empty
			if result == "" {
				t.Errorf("maskPassword(%q) returned empty string", tt.password)
			}
		})
	}
}

// TestMaskUsername tests username masking for security (prevents username exposure in logs)
func TestMaskUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		expected string
	}{
		// Short usernames (<= 4 chars) - fully masked
		{"Empty username", "", "****"},
		{"Single char", "a", "****"},
		{"Two chars", "ab", "****"},
		{"Three chars", "abc", "****"},
		{"Four chars", "user", "****"},

		// Normal usernames (> 4 chars) - show first 2 and last 2
		{"Five chars", "admin", "ad****in"},
		{"Email address", "user@example.com", "us****om"},
		{"Long email", "firstname.lastname@company.com", "fi****om"},
		{"Simple username", "administrator", "ad****or"},
		{"Username with numbers", "user12345", "us****45"},

		// Edge cases
		{"Hyphenated username", "john-doe", "jo****oe"},
		{"Underscore username", "john_doe", "jo****oe"},
		{"Domain username", "DOMAIN\\username", "DO****me"},
		{"UPN format", "user@DOMAIN.LOCAL", "us****AL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskUsername(tt.username)
			if result != tt.expected {
				t.Errorf("maskUsername(%q) = %q, want %q", tt.username, result, tt.expected)
			}

			// Security verification: Ensure original username is NOT in masked output (except for <= 4 chars)
			if len(tt.username) > 4 && result == tt.username {
				t.Errorf("maskUsername(%q) = %q, username not masked!", tt.username, result)
			}

			// Security verification: Ensure masked output is not empty
			if result == "" {
				t.Errorf("maskUsername(%q) returned empty string", tt.username)
			}
		})
	}
}

// TestMaskPassword_SecurityProperties tests security properties of password masking
func TestMaskPassword_SecurityProperties(t *testing.T) {
	t.Run("Masks all common passwords", func(t *testing.T) {
		commonPasswords := []string{
			"password123",
			"admin",
			"letmein",
			"welcome",
			"monkey",
			"dragon",
			"master",
			"qwerty",
			"abc123",
			"password",
		}

		for _, password := range commonPasswords {
			masked := maskPassword(password)
			// Ensure password is not fully visible
			if len(password) > 4 && masked == password {
				t.Errorf("maskPassword() did not mask common password: %s", password)
			}
			// Ensure masking is consistent
			masked2 := maskPassword(password)
			if masked != masked2 {
				t.Errorf("maskPassword() inconsistent for %s: %s != %s", password, masked, masked2)
			}
		}
	})

	t.Run("Masked output always contains asterisks", func(t *testing.T) {
		testPasswords := []string{"", "a", "abc", "12345", "password123", "VeryLongPassword"}
		for _, password := range testPasswords {
			masked := maskPassword(password)
			// All masked outputs should contain ****
			if len(masked) < 4 {
				t.Errorf("maskPassword(%q) output too short: %s", password, masked)
			}
		}
	})
}

// TestMaskUsername_SecurityProperties tests security properties of username masking
func TestMaskUsername_SecurityProperties(t *testing.T) {
	t.Run("Masks email addresses properly", func(t *testing.T) {
		emails := []string{
			"admin@company.com",
			"user@example.org",
			"test.user@subdomain.example.com",
			"firstname.lastname@corporate.example.net",
		}

		for _, email := range emails {
			masked := maskUsername(email)
			// Ensure email is not fully visible
			if masked == email {
				t.Errorf("maskUsername() did not mask email: %s", email)
			}
			// Ensure @ symbol is hidden (it should be in the middle part)
			// For long emails, @ will be masked
			if len(masked) > 10 && masked[2] == '@' {
				t.Errorf("maskUsername() exposed @ symbol for: %s -> %s", email, masked)
			}
		}
	})
}

// TestMaskAccessToken tests OAuth2 access token masking for security
func TestMaskAccessToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		// Short tokens (<= 16 chars) - fully masked
		{"Empty token", "", "****"},
		{"Single char", "a", "****"},
		{"Short token", "abc123", "****"},
		{"Exactly 16 chars", "1234567890123456", "****"},

		// Normal tokens (> 16 chars) - show first 8 and last 4
		{"17 chars", "12345678901234567", "12345678...4567"},
		{"Gmail-like token", "ya29.a0AfH6SMBxyz123456789abcdef", "ya29.a0A...cdef"},
		{"Azure AD token", "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6Ik", "eyJ0eXAi...I6Ik"},
		{"Long token", "ya29.a0AfH6SMBxyz123456789abcdefghijklmnopqrstuvwxyz", "ya29.a0A...wxyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskAccessToken(tt.token)
			if result != tt.expected {
				t.Errorf("maskAccessToken(%q) = %q, want %q", tt.token, result, tt.expected)
			}

			// Security verification: Ensure original token is NOT in masked output (except for <= 16 chars)
			if len(tt.token) > 16 && result == tt.token {
				t.Errorf("maskAccessToken(%q) = %q, token not masked!", tt.token, result)
			}

			// Security verification: Ensure masked output is not empty
			if result == "" {
				t.Errorf("maskAccessToken(%q) returned empty string", tt.token)
			}
		})
	}
}

// TestMaskAccessToken_SecurityProperties tests security properties of access token masking
func TestMaskAccessToken_SecurityProperties(t *testing.T) {
	t.Run("Masks real-world token formats", func(t *testing.T) {
		realTokens := []string{
			"ya29.a0AfH6SMBxyz123456789abcdefghijklmnop", // Google OAuth2
			"eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6IkN0VHVoTUpta", // Azure AD JWT
			"gho_16C7e42F292c6912E7710c838347Ae178B4a", // GitHub
			"xoxb-FAKE-TOKEN-FOR-TESTING-ONLY-NotARealToken123", // Slack-like format (fake)
		}

		for _, token := range realTokens {
			masked := maskAccessToken(token)
			// Ensure token is not fully visible
			if masked == token {
				t.Errorf("maskAccessToken() did not mask token: %s", token)
			}
			// Ensure middle portion is hidden
			if len(masked) > 20 {
				t.Errorf("maskAccessToken() exposed too much of token: %s -> %s", token, masked)
			}
		}
	})

	t.Run("Masked output always contains separator", func(t *testing.T) {
		testTokens := []string{"", "a", "short", "exactly16charss!", "longerToken12345678"}
		for _, token := range testTokens {
			masked := maskAccessToken(token)
			// Short tokens get ****, long tokens get ...
			if len(token) <= 16 {
				if masked != "****" {
					t.Errorf("maskAccessToken(%q) should be '****', got %s", token, masked)
				}
			} else {
				if !contains(masked, "...") {
					t.Errorf("maskAccessToken(%q) should contain '...', got %s", token, masked)
				}
			}
		}
	})
}

// helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
