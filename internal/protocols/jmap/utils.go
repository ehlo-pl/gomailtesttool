package jmap

// maskUsername masks a username for safe logging.
// Shows first 2 and last 2 characters with **** in between.
func maskUsername(username string) string {
	if len(username) <= 4 {
		return "****"
	}
	return username[:2] + "****" + username[len(username)-2:]
}

// maskPassword masks a password for safe logging.
// Shows first 2 and last 2 characters with **** in between.
// Passwords of 8 characters or fewer are fully masked, since revealing 4
// characters of a password that short would disclose most or all of it.
func maskPassword(password string) string {
	if len(password) == 0 {
		return ""
	}
	if len(password) <= 8 {
		return "****"
	}
	return password[:2] + "****" + password[len(password)-2:]
}

// maskAccessToken masks an access token for safe logging.
// Shows first 8 and last 4 characters with ... in between for tokens longer
// than 16 characters. Tokens of 9-16 characters show only first 2 and last 2.
// Tokens of 8 characters or fewer are fully masked.
func maskAccessToken(token string) string {
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
