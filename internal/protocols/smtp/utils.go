package smtp

// maskPassword masks a password for display in logs and error messages.
// For passwords <= 8 characters, returns "****" (revealing 4 characters of a
// password that short would disclose most or all of it)
// For longer passwords, shows first 2 and last 2 characters with **** in between
// This prevents password exposure in logs while allowing identification of which credential was used.
//
// Examples:
//   - "abc" -> "****"
//   - "password123" -> "pa****23"
//   - "MySecretP@ss" -> "My****ss"
func maskPassword(password string) string {
	if len(password) <= 8 {
		return "****"
	}
	// Show first 2 and last 2 characters
	return password[:2] + "****" + password[len(password)-2:]
}

// maskUsername masks a username for display in logs and error messages.
// For usernames <= 4 characters, returns "****"
// For longer usernames, shows first 2 and last 2 characters with **** in between
// Useful for email addresses in authentication contexts.
//
// Examples:
//   - "abc" -> "****"
//   - "user@example.com" -> "us****om"
//   - "admin" -> "ad****in"
func maskUsername(username string) string {
	if len(username) <= 4 {
		return "****"
	}
	// Show first 2 and last 2 characters
	return username[:2] + "****" + username[len(username)-2:]
}

// maskAccessToken masks an OAuth2 access token for display in logs.
// For tokens <= 16 characters, returns "****"
// For longer tokens, shows first 8 and last 4 characters with ... in between
// Access tokens are typically longer than passwords, so we show more context.
//
// Examples:
//   - "short" -> "****"
//   - "ya29.a0AfH6SMBxyz123456789abcdef" -> "ya29.a0A...cdef"
func maskAccessToken(token string) string {
	if len(token) <= 16 {
		return "****"
	}
	return token[:8] + "..." + token[len(token)-4:]
}
