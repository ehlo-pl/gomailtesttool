package imap

// maskUsername masks usernames/emails for safe display in logs.
// Shows first 2 and last 2 characters with **** in between.
// Examples: "user@example.com" -> "us****om", "ab" -> "****"
func maskUsername(username string) string {
	if len(username) <= 4 {
		return "****"
	}
	return username[:2] + "****" + username[len(username)-2:]
}
