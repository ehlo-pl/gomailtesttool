// Package protocol provides JMAP session management utilities.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WellKnownPath is the well-known path for JMAP autodiscovery.
const WellKnownPath = "/.well-known/jmap"

// DiscoveryURL returns the JMAP discovery URL for a hostname.
func DiscoveryURL(hostname string) string {
	// Ensure no trailing slash
	hostname = strings.TrimSuffix(hostname, "/")
	// Ensure https scheme
	if !strings.HasPrefix(hostname, "http://") && !strings.HasPrefix(hostname, "https://") {
		hostname = "https://" + hostname
	}
	return hostname + WellKnownPath
}

// ParseSession parses a JMAP session from JSON.
func ParseSession(data []byte) (*Session, error) {
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session: %w", err)
	}
	return &session, nil
}

// GetCapabilityNames returns the list of capability URIs.
func (s *Session) GetCapabilityNames() []string {
	var names []string
	for name := range s.Capabilities {
		names = append(names, name)
	}
	return names
}

// HasCapability checks if the server supports a capability.
func (s *Session) HasCapability(uri string) bool {
	_, ok := s.Capabilities[uri]
	return ok
}

// HasMailCapability checks if the server supports JMAP Mail.
func (s *Session) HasMailCapability() bool {
	return s.HasCapability(MailCapability)
}

// HasSubmissionCapability checks if the server supports JMAP Submission.
func (s *Session) HasSubmissionCapability() bool {
	return s.HasCapability(SubmissionCapability)
}

// GetPrimaryMailAccountId returns the primary account ID for mail.
func (s *Session) GetPrimaryMailAccountId() (Id, bool) {
	id, ok := s.PrimaryAccounts[MailCapability]
	return id, ok
}

// GetAccountCount returns the number of accounts.
func (s *Session) GetAccountCount() int {
	return len(s.Accounts)
}

// GetAccountNames returns the names of all accounts.
func (s *Session) GetAccountNames() []string {
	var names []string
	for _, account := range s.Accounts {
		names = append(names, account.Name)
	}
	return names
}

// CoreCapabilityInfo contains parsed core capability information.
type CoreCapabilityInfo struct {
	MaxSizeUpload         int64    `json:"maxSizeUpload"`
	MaxConcurrentUpload   int      `json:"maxConcurrentUpload"`
	MaxSizeRequest        int64    `json:"maxSizeRequest"`
	MaxConcurrentRequests int      `json:"maxConcurrentRequests"`
	MaxCallsInRequest     int      `json:"maxCallsInRequest"`
	MaxObjectsInGet       int      `json:"maxObjectsInGet"`
	MaxObjectsInSet       int      `json:"maxObjectsInSet"`
	CollationAlgorithms   []string `json:"collationAlgorithms"`
}

// GetCoreCapability parses and returns the core capability information.
func (s *Session) GetCoreCapability() (*CoreCapabilityInfo, error) {
	raw, ok := s.Capabilities[CoreCapability]
	if !ok {
		return nil, fmt.Errorf("core capability not found")
	}
	var info CoreCapabilityInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("failed to parse core capability: %w", err)
	}
	return &info, nil
}

// MailCapabilityInfo contains parsed mail capability information.
type MailCapabilityInfo struct {
	MaxMailboxesPerEmail       *int64   `json:"maxMailboxesPerEmail"`
	MaxMailboxDepth            *int     `json:"maxMailboxDepth"`
	MaxSizeMailboxName         int      `json:"maxSizeMailboxName"`
	MaxSizeAttachmentsPerEmail int64    `json:"maxSizeAttachmentsPerEmail"`
	EmailQuerySortOptions      []string `json:"emailQuerySortOptions"`
	MayCreateTopLevelMailbox   bool     `json:"mayCreateTopLevelMailbox"`
}

// GetMailCapability parses and returns the mail capability information.
func (s *Session) GetMailCapability() (*MailCapabilityInfo, error) {
	raw, ok := s.Capabilities[MailCapability]
	if !ok {
		return nil, fmt.Errorf("mail capability not found")
	}
	var info MailCapabilityInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("failed to parse mail capability: %w", err)
	}
	return &info, nil
}

// Validate checks if the session has the required fields.
func (s *Session) Validate() error {
	if s.APIURL == "" {
		return fmt.Errorf("session missing apiUrl")
	}
	if len(s.Capabilities) == 0 {
		return fmt.Errorf("session missing capabilities")
	}
	if !s.HasCapability(CoreCapability) {
		return fmt.Errorf("session missing core capability")
	}
	return nil
}

// Summary returns a human-readable summary of the session.
func (s *Session) Summary() string {
	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Username: %s\n", s.Username)
	_, _ = fmt.Fprintf(&sb, "API URL: %s\n", s.APIURL)
	_, _ = fmt.Fprintf(&sb, "Accounts: %d\n", len(s.Accounts))
	_, _ = fmt.Fprintf(&sb, "Capabilities: %s\n", strings.Join(s.GetCapabilityNames(), ", "))
	return sb.String()
}
