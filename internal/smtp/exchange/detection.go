package exchange

import (
	"fmt"
	"regexp"
	"strings"

	"gomailtesttool/internal/smtp/protocol"
)

// ExchangeInfo holds information about a detected Exchange server.
type ExchangeInfo struct {
	IsExchange bool   // Whether the server is Exchange
	Version    string // Exchange version (if detectable)
	Banner     string // SMTP banner text
}

// DetectExchange checks if the SMTP server is Microsoft Exchange.
// Detection is based on banner text and SMTP capabilities.
func DetectExchange(banner string, capabilities protocol.Capabilities) *ExchangeInfo {
	info := &ExchangeInfo{
		IsExchange: false,
		Version:    "Unknown",
		Banner:     banner,
	}

	bannerLower := strings.ToLower(banner)

	// Check banner for Exchange signatures
	if strings.Contains(bannerLower, "microsoft esmtp mail service") ||
		strings.Contains(bannerLower, "microsoft exchange") {
		info.IsExchange = true
		info.Version = extractExchangeVersion(banner)
		return info
	}

	// Check for Exchange-specific SMTP extensions
	exchangeExtensions := []string{"X-EXPS", "XEXCH50", "X-ANONYMOUSTLS", "X-EXCH50"}
	for _, ext := range exchangeExtensions {
		if capabilities.Has(ext) {
			info.IsExchange = true
			// Try to extract version from banner even if not in typical format
			info.Version = extractExchangeVersion(banner)
			return info
		}
	}

	return info
}

// extractExchangeVersion attempts to parse the Exchange version from the banner.
// Returns "Unknown" if version cannot be determined.
func extractExchangeVersion(banner string) string {
	// Pattern 1: "Microsoft ESMTP MAIL Service, Version: 10.0.14393.0"
	re1 := regexp.MustCompile(`Version:\s*(\d+\.\d+\.\d+\.\d+)`)
	if matches := re1.FindStringSubmatch(banner); len(matches) > 1 {
		return mapVersionNumber(matches[1])
	}

	// Pattern 2: "Microsoft Exchange Server 2019"
	re2 := regexp.MustCompile(`Microsoft Exchange Server (\d+)`)
	if matches := re2.FindStringSubmatch(banner); len(matches) > 1 {
		return "Exchange " + matches[1]
	}

	// Pattern 3: Version number in parentheses
	re3 := regexp.MustCompile(`\((\d+\.\d+\.\d+)`)
	if matches := re3.FindStringSubmatch(banner); len(matches) > 1 {
		return mapVersionNumber(matches[1])
	}

	return "Unknown"
}

// mapVersionNumber maps Exchange build numbers to friendly version names.
func mapVersionNumber(version string) string {
	// Major Exchange versions by build number
	switch {
	case strings.HasPrefix(version, "15.2."):
		return "Exchange 2019 (" + version + ")"
	case strings.HasPrefix(version, "15.1."):
		return "Exchange 2016 (" + version + ")"
	case strings.HasPrefix(version, "15.0."):
		return "Exchange 2013 (" + version + ")"
	case strings.HasPrefix(version, "14."):
		return "Exchange 2010 (" + version + ")"
	case strings.HasPrefix(version, "8."):
		return "Exchange 2007 (" + version + ")"
	case strings.HasPrefix(version, "6.5."):
		return "Exchange 2003 (" + version + ")"
	case strings.HasPrefix(version, "6.0."):
		return "Exchange 2000 (" + version + ")"
	default:
		return "Exchange (" + version + ")"
	}
}

// GetExchangeDiagnostics returns Exchange-specific diagnostic information.
func GetExchangeDiagnostics(capabilities protocol.Capabilities) []string {
	var diagnostics []string

	// Message size limit
	sizeLimit := capabilities.GetMaxMessageSize()
	if sizeLimit > 0 {
		diagnostics = append(diagnostics, fmt.Sprintf("Maximum message size: %d bytes (%.2f MB)", sizeLimit, float64(sizeLimit)/(1024*1024)))
	}

	// Authentication methods
	authMethods := capabilities.GetAuthMechanisms()
	if len(authMethods) > 0 {
		diagnostics = append(diagnostics, fmt.Sprintf("Supported authentication: %s", strings.Join(authMethods, ", ")))
	}

	// TLS support
	if capabilities.SupportsSTARTTLS() {
		diagnostics = append(diagnostics, "STARTTLS is supported")
	} else {
		diagnostics = append(diagnostics, "WARNING: STARTTLS not supported - connection is insecure")
	}

	// 8BITMIME support
	if capabilities.Supports8BITMIME() {
		diagnostics = append(diagnostics, "8-bit MIME is supported")
	}

	// Pipelining
	if capabilities.SupportsPipelining() {
		diagnostics = append(diagnostics, "Command pipelining is supported")
	}

	return diagnostics
}

// GetExchangeWarnings returns common Exchange-related warnings and recommendations.
func GetExchangeWarnings() []string {
	warnings := []string{
		"Exchange typically restricts relay for unauthenticated connections",
		"Authentication usually requires TLS on port 587 (use -action teststarttls first)",
		"Exchange Online/Microsoft 365 requires modern authentication (OAuth 2.0)",
		"On-premises Exchange: Ensure proper SMTP connector configuration for relay",
		"Anonymous relay is typically disabled by default for security",
	}
	return warnings
}

// GetExchangeRecommendations returns recommendations for Exchange SMTP configuration.
func GetExchangeRecommendations(port int, capabilities protocol.Capabilities) []string {
	var recommendations []string

	// Port-specific recommendations
	switch port {
	case 25:
		recommendations = append(recommendations, "Port 25: Typically for server-to-server (MTA) relay")
		if capabilities.SupportsAuth() {
			recommendations = append(recommendations, "Authentication on port 25 usually requires STARTTLS first")
		}
	case 587:
		recommendations = append(recommendations, "Port 587: Message submission port (requires authentication)")
		if !capabilities.SupportsSTARTTLS() {
			recommendations = append(recommendations, "WARNING: Port 587 should support STARTTLS for secure authentication")
		}
	case 465:
		recommendations = append(recommendations, "Port 465: SMTP over implicit TLS (SMTPS)")
	}

	// Auth recommendations
	if capabilities.SupportsAuth() {
		authMethods := capabilities.GetAuthMechanisms()
		hasSecureAuth := false
		for _, method := range authMethods {
			if method == "LOGIN" || method == "PLAIN" {
				// These are insecure without TLS
			} else if method == "CRAM-MD5" || method == "NTLM" {
				hasSecureAuth = true
			}
		}
		if !hasSecureAuth && !capabilities.SupportsSTARTTLS() {
			recommendations = append(recommendations, "WARNING: Authentication methods require TLS for security")
		}
	}

	// Size recommendations
	sizeLimit := capabilities.GetMaxMessageSize()
	if sizeLimit > 0 && sizeLimit < 10*1024*1024 {
		recommendations = append(recommendations, fmt.Sprintf("Small message size limit detected (%.2f MB) - consider increasing for larger attachments", float64(sizeLimit)/(1024*1024)))
	}

	return recommendations
}

// FormatExchangeInfo returns a formatted string with Exchange server information.
func FormatExchangeInfo(info *ExchangeInfo, capabilities protocol.Capabilities) string {
	if !info.IsExchange {
		return ""
	}

	var result strings.Builder
	result.WriteString("\n")
	result.WriteString("═══════════════════════════════════════════════════════════\n")
	result.WriteString("  Microsoft Exchange Server Detected\n")
	result.WriteString("═══════════════════════════════════════════════════════════\n")
	result.WriteString(fmt.Sprintf("Version: %s\n", info.Version))
	result.WriteString(fmt.Sprintf("Banner:  %s\n", info.Banner))
	result.WriteString("\n")

	// Diagnostics
	diagnostics := GetExchangeDiagnostics(capabilities)
	if len(diagnostics) > 0 {
		result.WriteString("Exchange Capabilities:\n")
		for _, diag := range diagnostics {
			result.WriteString(fmt.Sprintf("  • %s\n", diag))
		}
		result.WriteString("\n")
	}

	// Warnings
	result.WriteString("Exchange Notes:\n")
	warnings := GetExchangeWarnings()
	for _, warning := range warnings {
		result.WriteString(fmt.Sprintf("  ⚠ %s\n", warning))
	}

	result.WriteString("═══════════════════════════════════════════════════════════\n")

	return result.String()
}
