// Package security provides security-related utilities for the gomailtesttool suite.
// It includes functions for safely masking sensitive data in logs and output.
package security

// MaskUsername masks a username for safe logging.
// Shows first 2 and last 2 characters with **** in between.
// Short usernames (4 characters or less) are fully masked.
func MaskUsername(username string) string {
	if len(username) <= 4 {
		return "****"
	}
	return username[:2] + "****" + username[len(username)-2:]
}

// MaskPassword masks a password for safe logging.
// Shows first 2 and last 2 characters with **** in between.
// Short passwords (8 characters or less) are fully masked, since revealing
// 4 characters of a password that short would disclose most or all of it.
// Empty passwords return empty string.
func MaskPassword(password string) string {
	if len(password) == 0 {
		return ""
	}
	if len(password) <= 8 {
		return "****"
	}
	return password[:2] + "****" + password[len(password)-2:]
}

// MaskAccessToken masks an access token for safe logging.
// Shows first 8 and last 4 characters with ... in between for tokens longer
// than 16 characters. Tokens of 9-16 characters show only first 2 and last 2.
// Tokens of 8 characters or fewer are fully masked, since any partial reveal
// would disclose most or all of the token.
// Empty tokens return empty string.
func MaskAccessToken(token string) string {
	if len(token) == 0 {
		return ""
	}
	if len(token) <= 8 {
		return "****"
	}
	if len(token) <= 16 {
		return token[:2] + "****" + token[len(token)-2:]
	}
	return token[:8] + "..." + token[len(token)-4:]
}

// MaskSecret masks a client secret or similar credential.
// Shows first 4 characters followed by asterisks indicating length.
// Empty secrets return empty string.
func MaskSecret(secret string) string {
	if len(secret) == 0 {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:4] + "****"
}

// MaskGUID masks a GUID/UUID for safe logging.
// Shows first 8 characters followed by asterisks.
// Preserves enough to identify the resource while hiding the full value.
func MaskGUID(guid string) string {
	if len(guid) <= 8 {
		return guid + "****"
	}
	return guid[:8] + "****"
}

// MaskEmail masks an email address for safe logging.
// Shows first 2 characters of local part and domain.
// Example: "user@example.com" becomes "us****@ex****"
func MaskEmail(email string) string {
	if len(email) == 0 {
		return ""
	}

	// Find @ symbol
	atIndex := -1
	for i, c := range email {
		if c == '@' {
			atIndex = i
			break
		}
	}

	if atIndex == -1 {
		// No @ found, treat as username
		return MaskUsername(email)
	}

	localPart := email[:atIndex]
	domain := email[atIndex+1:]

	maskedLocal := "****"
	if len(localPart) > 2 {
		maskedLocal = localPart[:2] + "****"
	}

	maskedDomain := "****"
	if len(domain) > 2 {
		maskedDomain = domain[:2] + "****"
	}

	return maskedLocal + "@" + maskedDomain
}

// MaskErrorMessage sanitizes an error message by masking potential credentials.
// This should be used before logging error messages that may contain sensitive data.
// It looks for common patterns like passwords, tokens, secrets, and auth-related strings.
func MaskErrorMessage(errMsg string, credentials ...string) string {
	result := errMsg

	// Mask any explicitly provided credentials
	for _, cred := range credentials {
		if cred != "" && len(cred) > 3 {
			// Replace credential with masked version
			masked := "****"
			if len(cred) > 8 {
				masked = cred[:2] + "****" + cred[len(cred)-2:]
			}
			result = replaceIgnoreCase(result, cred, masked)
		}
	}

	return result
}

// replaceIgnoreCase replaces all occurrences of old in s with new (case-insensitive).
func replaceIgnoreCase(s, old, replacement string) string {
	if old == "" {
		return s
	}

	result := s
	lowerS := toLower(s)
	lowerOld := toLower(old)

	// Find and replace all occurrences
	for {
		idx := indexOf(lowerS, lowerOld)
		if idx == -1 {
			break
		}
		result = result[:idx] + replacement + result[idx+len(old):]
		lowerS = lowerS[:idx] + replacement + lowerS[idx+len(old):]
	}

	return result
}

// toLower converts string to lowercase without importing strings package.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// indexOf finds the first occurrence of substr in s.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
