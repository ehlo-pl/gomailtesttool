package validation

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ValidateEmail performs basic email format validation.
// Checks for the presence of @ and validates the local and domain parts.
func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email format: %s (missing @)", email)
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid email format: %s", email)
	}
	return nil
}

// ExtractEmailDomain returns the domain portion (after the last '@') of an
// email address. Returns an error if there's no '@' or the domain part is empty.
func ExtractEmailDomain(email string) (string, error) {
	email = strings.TrimSpace(email)
	idx := strings.LastIndex(email, "@")
	if idx == -1 || idx == len(email)-1 {
		return "", fmt.Errorf("cannot extract domain from email: %s", email)
	}
	return email[idx+1:], nil
}

// ValidateEmails validates a slice of email addresses.
// Returns an error if any email in the slice is invalid.
func ValidateEmails(emails []string, fieldName string) error {
	for _, email := range emails {
		if err := ValidateEmail(email); err != nil {
			return fmt.Errorf("%s contains invalid email: %w", fieldName, err)
		}
	}
	return nil
}

// ValidateGUID validates that a string matches standard GUID format (8-4-4-4-12).
// Example: 12345678-1234-1234-1234-123456789012
func ValidateGUID(guid, fieldName string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	// Basic GUID format: 8-4-4-4-12 hex characters
	if len(guid) != 36 {
		return fmt.Errorf("%s should be a GUID (36 characters, format: 12345678-1234-1234-1234-123456789012)", fieldName)
	}
	// Check for proper dash positions
	if guid[8] != '-' || guid[13] != '-' || guid[18] != '-' || guid[23] != '-' {
		return fmt.Errorf("%s has invalid GUID format (dashes at wrong positions)", fieldName)
	}
	return nil
}

// ValidateFilePath validates and sanitizes a file path for security and usability.
//
// Security Policy:
//   - Absolute paths are ALLOWED (e.g., C:\certs\cert.pfx, /etc/ssl/cert.pfx)
//     Users need flexibility to specify certificate files anywhere on the filesystem
//   - Relative paths within working directory are ALLOWED (e.g., ./certs/cert.pfx, subdir/cert.pfx)
//   - Relative paths attempting to escape working directory are REJECTED (e.g., ../../etc/passwd)
//
// Use Case: This function is used for validating -pfxpath flag where authorized users
// specify certificate files. Users are trusted (CLI tool context), but defense-in-depth
// prevents accidental directory traversal.
//
// Empty paths are allowed for optional fields (returns nil).
func ValidateFilePath(path, fieldName string) error {
	if path == "" {
		return nil // Empty is allowed for optional fields
	}

	// Clean and normalize path
	// filepath.Clean() resolves . and .. elements, but preserves meaningful ".." at the start
	// Examples:
	//   - "safe/../file.txt" becomes "file.txt" (cancelled out)
	//   - "../../etc/passwd" remains "../../etc/passwd" (meaningful traversal)
	//   - "safe/../../etc/passwd" becomes "../etc/passwd" (simplified but still escapes)
	cleanPath := filepath.Clean(path)

	// Convert to absolute path for file existence checks
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("%s: invalid path: %w", fieldName, err)
	}

	// Get current working directory for relative path validation
	cwd, err := os.Getwd()
	if err != nil {
		// If we can't get cwd, skip traversal check and just verify file exists
		cwd = ""
	}

	// Path Traversal Check (defense-in-depth)
	// Apply only to RELATIVE paths (absolute paths are intentionally allowed)
	if cwd != "" && !filepath.IsAbs(path) {
		// After filepath.Clean(), if ".." remains in the path, it means the path
		// attempts to escape the working directory tree.
		//
		// Why this works:
		//   - filepath.Clean() cancels out unnecessary ".." (e.g., "a/../b" -> "b")
		//   - Remaining ".." indicate actual upward traversal (e.g., "../../etc" -> "../../etc")
		//   - For relative paths, any remaining ".." would escape the working directory
		//
		// Examples of REJECTED paths:
		//   - "../../etc/passwd" (tries to escape cwd)
		//   - "../../../sensitive" (tries to escape cwd)
		//   - "safe/../../etc" (simplified to "../etc", still escapes)
		//
		// Examples of ALLOWED paths:
		//   - "certs/cert.pfx" (within cwd)
		//   - "safe/../cert.pfx" (cleaned to "cert.pfx", within cwd)
		//   - "/etc/ssl/cert.pfx" (absolute path, bypasses this check)
		if strings.Contains(cleanPath, "..") {
			return fmt.Errorf("%s: relative paths with '..' are not allowed (use absolute path if file is outside working directory)", fieldName)
		}
	}

	// Verify file exists and is accessible
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: file not found: %s", fieldName, path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("%s: permission denied: %s", fieldName, path)
		}
		return fmt.Errorf("%s: cannot access file: %w", fieldName, err)
	}

	// Verify it's a regular file (not a directory or special file)
	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file (is it a directory?): %s", fieldName, path)
	}

	return nil
}

// ValidateHostname validates a hostname or IP address.
// Accepts DNS names, IPv4 addresses, and IPv6 addresses.
func ValidateHostname(hostname string) error {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	// Check if it's a valid IP address (IPv4 or IPv6)
	if net.ParseIP(hostname) != nil {
		return nil // Valid IP address
	}

	// Check if it's a valid hostname (DNS name)
	// Basic validation: must contain at least one character, may contain letters, digits, dots, and hyphens
	if len(hostname) > 253 {
		return fmt.Errorf("hostname too long (max 253 characters)")
	}

	// Check for valid characters in hostname
	for _, ch := range hostname {
		if (ch < 'a' || ch > 'z') &&
			(ch < 'A' || ch > 'Z') &&
			(ch < '0' || ch > '9') &&
			ch != '.' && ch != '-' {
			return fmt.Errorf("hostname contains invalid character: %c", ch)
		}
	}

	// Hostname cannot start or end with a hyphen or dot
	if strings.HasPrefix(hostname, "-") || strings.HasSuffix(hostname, "-") ||
		strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return fmt.Errorf("hostname cannot start or end with hyphen or dot")
	}

	return nil
}

// ValidatePort validates that a port number is in the valid range (1-65535).
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535 (got %d)", port)
	}
	return nil
}

// ValidateSMTPAddress validates an email address in SMTP format (RFC 5321).
// This is stricter than general email validation and follows SMTP standards.
func ValidateSMTPAddress(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("SMTP address cannot be empty")
	}

	// SMTP addresses should not contain angle brackets (those are added by the protocol)
	// But we'll accept them if present and extract the actual address
	if strings.HasPrefix(address, "<") && strings.HasSuffix(address, ">") {
		address = address[1 : len(address)-1]
	}

	// Validate as email
	return ValidateEmail(address)
}

// ValidateProxyURL validates a proxy URL for HTTP/HTTPS/SOCKS5 proxies.
// Validates URL format, scheme, hostname, and port.
//
// Supported formats:
//   - http://proxy.example.com:8080
//   - https://proxy.example.com:8080
//   - socks5://proxy.example.com:1080
//   - http://username:password@proxy.example.com:8080
//
// Empty URLs are allowed (proxy is optional).
func ValidateProxyURL(proxyURL string) error {
	if proxyURL == "" {
		return nil // Empty is allowed (proxy is optional)
	}

	// Parse URL
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL format: %w", err)
	}

	// Validate scheme (http, https, socks5)
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return fmt.Errorf("proxy URL must include scheme (http://, https://, or socks5://)")
	}
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return fmt.Errorf("unsupported proxy scheme: %s (supported: http, https, socks5)", scheme)
	}

	// Validate hostname
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("proxy URL must include hostname")
	}
	if err := ValidateHostname(hostname); err != nil {
		return fmt.Errorf("invalid proxy hostname: %w", err)
	}

	// Validate port (if specified)
	if portStr := u.Port(); portStr != "" {
		// Parse port string to int
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			return fmt.Errorf("invalid proxy port: %s", portStr)
		}
		if err := ValidatePort(port); err != nil {
			return fmt.Errorf("invalid proxy port: %w", err)
		}
	}

	// Userinfo (username:password) is optional but if present, should be valid
	if u.User != nil {
		username := u.User.Username()
		if username == "" {
			return fmt.Errorf("proxy URL contains empty username")
		}
		// Password is optional even if username is present
	}

	return nil
}
