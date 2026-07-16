package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"

	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	"github.com/ehlo-pl/gomailtesttool/internal/common/ratelimit"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
	imapprotocol "github.com/ehlo-pl/gomailtesttool/internal/imap/protocol"
)

// IMAPClient wraps an IMAP connection with additional functionality.
type IMAPClient struct {
	client   *imapclient.Client
	host     string
	port     int
	config   *Config
	caps     *imapprotocol.Capabilities
	limiter  *ratelimit.Limiter
	tlsState *tls.ConnectionState
	selected *imap.SelectData // data of the currently selected mailbox, nil before SelectMailbox
}

// MailboxInfo holds information about a mailbox.
type MailboxInfo struct {
	Name       string
	Attributes []string
	Messages   uint32
	Unseen     uint32
}

// NewIMAPClient creates a new IMAP client.
func NewIMAPClient(config *Config) *IMAPClient {
	var limiter *ratelimit.Limiter
	if config.RateLimit > 0 {
		limiter = ratelimit.New(config.RateLimit)
	}

	return &IMAPClient{
		host:    config.Host,
		port:    config.Port,
		config:  config,
		limiter: limiter,
	}
}

// Connect establishes a connection to the IMAP server.
func (c *IMAPClient) Connect(ctx context.Context) error {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	// Determine connection address (override or default to host)
	connectHost := c.host
	if c.config.ConnectAddress != "" {
		connectHost = c.config.ConnectAddress
	}

	// Resolve to a specific address family if --ipv4/--ipv6 was requested
	connectHost, err := network.ResolveForDial(ctx, connectHost, c.config.IPv4Only, c.config.IPv6Only)
	if err != nil {
		return err
	}
	address := net.JoinHostPort(connectHost, fmt.Sprintf("%d", c.port))

	options := &imapclient.Options{
		TLSConfig: &tls.Config{
			ServerName:         c.host,
			InsecureSkipVerify: c.config.SkipVerify,
			MinVersion:         tlsutil.ParseTLSVersion(c.config.TLSVersion),
		},
	}

	var client *imapclient.Client

	if c.config.IMAPS {
		// Implicit TLS (IMAPS)
		client, err = imapclient.DialTLS(address, options)
		if err == nil {
			c.tlsState = &tls.ConnectionState{} // Mark as TLS connection
		}
	} else if c.config.StartTLS {
		// Explicit TLS via STARTTLS
		client, err = imapclient.DialStartTLS(address, options)
		if err == nil {
			c.tlsState = &tls.ConnectionState{} // Mark as TLS connection
		}
	} else {
		// Plain connection
		client, err = imapclient.DialInsecure(address, options)
	}

	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	c.client = client

	// Parse capabilities from greeting
	if caps := client.Caps(); caps != nil {
		c.caps = convertCaps(caps)
	}

	return nil
}

// GetGreeting returns the server greeting (capabilities from greeting).
func (c *IMAPClient) GetGreeting() string {
	if c.caps != nil {
		return c.caps.String()
	}
	return ""
}

// GetCapabilities returns the server capabilities.
func (c *IMAPClient) GetCapabilities() *imapprotocol.Capabilities {
	return c.caps
}

// GetTLSState returns the TLS connection state (if TLS is active).
func (c *IMAPClient) GetTLSState() *tls.ConnectionState {
	return c.tlsState
}

// StartTLS is not supported after connection with go-imap v2.
// Use DialStartTLS instead by setting config.StartTLS = true before Connect.
func (c *IMAPClient) StartTLS(tlsConfig *tls.Config) error {
	// In go-imap v2, STARTTLS must be done at connection time using DialStartTLS
	// This method is kept for API compatibility but returns an error
	return fmt.Errorf("STARTTLS must be enabled before Connect() by setting StartTLS=true in config")
}

// Auth authenticates with the server using the specified method.
func (c *IMAPClient) Auth(ctx context.Context, username, password, accessToken string) error {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	method := c.config.AuthMethod

	// Auto-select auth method
	if strings.EqualFold(method, "auto") {
		if accessToken != "" && c.caps != nil && c.caps.SupportsXOAUTH2() {
			method = "XOAUTH2"
		} else if c.caps != nil && c.caps.SupportsPlain() {
			method = "PLAIN"
		} else if c.caps != nil && c.caps.SupportsLogin() {
			method = "LOGIN"
		} else {
			method = "LOGIN" // Fallback
		}
	}

	switch strings.ToUpper(method) {
	case "XOAUTH2":
		return c.authXOAUTH2(username, accessToken)
	case "PLAIN":
		return c.authPlain(username, password)
	case "LOGIN":
		return c.authLogin(username, password)
	default:
		return fmt.Errorf("unsupported auth method: %s", method)
	}
}

// authPlain performs PLAIN authentication.
func (c *IMAPClient) authPlain(username, password string) error {
	saslClient := sasl.NewPlainClient("", username, password)
	if err := c.client.Authenticate(saslClient); err != nil {
		return fmt.Errorf("PLAIN authentication failed: %w", err)
	}
	return nil
}

// authLogin performs LOGIN authentication (direct LOGIN command).
func (c *IMAPClient) authLogin(username, password string) error {
	if err := c.client.Login(username, password).Wait(); err != nil {
		return fmt.Errorf("LOGIN failed: %w", err)
	}
	return nil
}

// authXOAUTH2 performs XOAUTH2 authentication.
func (c *IMAPClient) authXOAUTH2(username, accessToken string) error {
	saslClient := sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
		Username: username,
		Token:    accessToken,
	})
	if err := c.client.Authenticate(saslClient); err != nil {
		return fmt.Errorf("XOAUTH2 authentication failed: %w", err)
	}
	return nil
}

// ListMailboxes lists all mailboxes.
func (c *IMAPClient) ListMailboxes(ctx context.Context) ([]MailboxInfo, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	// List all mailboxes
	listCmd := c.client.List("", "*", nil)
	mailboxes, err := listCmd.Collect()
	if err != nil {
		return nil, fmt.Errorf("LIST failed: %w", err)
	}

	var result []MailboxInfo
	for _, mb := range mailboxes {
		info := MailboxInfo{
			Name:       mb.Mailbox,
			Attributes: convertMailboxAttrs(mb.Attrs),
		}

		// Try to get STATUS for message counts (optional)
		// Some servers may not allow STATUS on all mailboxes
		statusCmd := c.client.Status(mb.Mailbox, &imap.StatusOptions{
			NumMessages: true,
			NumUnseen:   true,
		})
		if status, err := statusCmd.Wait(); err == nil {
			if status.NumMessages != nil {
				info.Messages = *status.NumMessages
			}
			if status.NumUnseen != nil {
				info.Unseen = *status.NumUnseen
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// SelectMailbox selects (opens) the given mailbox for subsequent SEARCH/FETCH commands.
func (c *IMAPClient) SelectMailbox(ctx context.Context, mailbox string) error {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	data, err := c.client.Select(mailbox, nil).Wait()
	if err != nil {
		return fmt.Errorf("SELECT %s failed: %w", mailbox, err)
	}
	c.selected = data
	return nil
}

// MessageSummary holds envelope information for a listed message.
type MessageSummary struct {
	UID     uint32
	Subject string
	From    string
	Date    string
}

// ListMessages fetches envelope data for the newest count messages of the
// currently selected mailbox (newest first). SelectMailbox must be called first.
func (c *IMAPClient) ListMessages(ctx context.Context, count int) ([]MessageSummary, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	if c.selected == nil {
		return nil, fmt.Errorf("no mailbox selected")
	}
	numMessages := c.selected.NumMessages
	if numMessages == 0 {
		return nil, nil
	}

	// Fetch the last count sequence numbers (the newest messages).
	from := uint32(1)
	if count > 0 && uint32(count) < numMessages {
		from = numMessages - uint32(count) + 1
	}
	var seqSet imap.SeqSet
	seqSet.AddRange(from, numMessages)

	fetchCmd := c.client.Fetch(seqSet, &imap.FetchOptions{Envelope: true, UID: true})
	messages, err := fetchCmd.Collect()
	if err != nil {
		return nil, fmt.Errorf("FETCH failed: %w", err)
	}

	result := make([]MessageSummary, 0, len(messages))
	for _, msg := range messages {
		summary := MessageSummary{UID: uint32(msg.UID)}
		if env := msg.Envelope; env != nil {
			summary.Subject = env.Subject
			if len(env.From) > 0 {
				summary.From = env.From[0].Addr()
			}
			if !env.Date.IsZero() {
				summary.Date = env.Date.Format("2006-01-02 15:04:05")
			}
		}
		result = append(result, summary)
	}

	// Sequence numbers ascend oldest-to-newest; present newest first.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

// SearchMessages searches the selected mailbox for messages matching the
// given Message-ID header and/or a subject substring. At least one of
// messageID/subject must be non-empty. Returns matching UIDs.
func (c *IMAPClient) SearchMessages(ctx context.Context, messageID, subject string) ([]imap.UID, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	criteria := &imap.SearchCriteria{}
	if messageID != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "Message-Id", Value: messageID})
	}
	if subject != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "Subject", Value: subject})
	}

	data, err := c.client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("SEARCH failed: %w", err)
	}

	uidSet, ok := data.All.(imap.UIDSet)
	if !ok {
		return nil, nil
	}
	uids, _ := uidSet.Nums()
	return uids, nil
}

// FetchRFC822 fetches the full raw RFC822 message body for the given UID.
func (c *IMAPClient) FetchRFC822(ctx context.Context, uid imap.UID) ([]byte, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	fetchCmd := c.client.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}},
	})
	defer func() { _ = fetchCmd.Close() }()

	msg := fetchCmd.Next()
	if msg == nil {
		return nil, fmt.Errorf("no message returned for UID %d", uid)
	}

	buf, err := msg.Collect()
	if err != nil {
		return nil, fmt.Errorf("FETCH failed for UID %d: %w", uid, err)
	}

	body := buf.FindBodySection(&imap.FetchItemBodySection{})
	if body == nil {
		return nil, fmt.Errorf("no body section returned for UID %d", uid)
	}
	return body, nil
}

// Logout sends the LOGOUT command and closes the connection.
func (c *IMAPClient) Logout() error {
	if c.client != nil {
		return c.client.Logout().Wait()
	}
	return nil
}

// Close closes the connection without sending LOGOUT.
func (c *IMAPClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// convertCaps converts go-imap capabilities to our protocol.Capabilities.
func convertCaps(caps imap.CapSet) *imapprotocol.Capabilities {
	var capsList []string
	for cap := range caps {
		capsList = append(capsList, string(cap))
	}
	return imapprotocol.NewCapabilities(capsList)
}

// convertMailboxAttrs converts mailbox attributes to strings.
func convertMailboxAttrs(attrs []imap.MailboxAttr) []string {
	var result []string
	for _, attr := range attrs {
		result = append(result, string(attr))
	}
	return result
}
