package jmap

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

// JMAPClient wraps HTTP client for JMAP operations.
type JMAPClient struct {
	config     *Config
	httpClient *http.Client
	session    *protocol.Session
}

// NewJMAPClient creates a new JMAP client.
func NewJMAPClient(config *Config) *JMAPClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         config.Host, // Use original host for SNI
			InsecureSkipVerify: config.SkipVerify,
		},
	}

	// Override the dial address if --address was given and/or resolve to a
	// specific address family if --ipv4/--ipv6 was requested.
	if config.ConnectAddress != "" || config.IPv4Only || config.IPv6Only {
		dialer := &net.Dialer{}
		transport.DialContext = func(ctx context.Context, dialNetwork, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				// If no port in address, use original
				return dialer.DialContext(ctx, dialNetwork, addr)
			}
			if config.ConnectAddress != "" {
				host = config.ConnectAddress
			}
			host, err = network.ResolveForDial(ctx, host, config.IPv4Only, config.IPv6Only)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, dialNetwork, net.JoinHostPort(host, port))
		}
	}

	return &JMAPClient{
		config: config,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// GetDiscoveryURL returns the JMAP discovery URL.
func (c *JMAPClient) GetDiscoveryURL() string {
	host := c.config.Host
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		if c.config.Port == 443 {
			host = "https://" + bracketIPv6(host)
		} else {
			host = "https://" + net.JoinHostPort(host, strconv.Itoa(c.config.Port))
		}
	}
	return protocol.DiscoveryURL(host)
}

// bracketIPv6 wraps host in [] if it's a literal IPv6 address, as required
// for use in a URL authority component. Other hosts are returned unchanged.
func bracketIPv6(host string) string {
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return "[" + host + "]"
	}
	return host
}

// Discover fetches the JMAP session from the well-known URL.
func (c *JMAPClient) Discover(ctx context.Context) (*protocol.Session, error) {
	url := c.GetDiscoveryURL()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if provided
	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discovery failed with status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	session, err := protocol.ParseSession(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse session: %w", err)
	}

	if err := session.Validate(); err != nil {
		return nil, fmt.Errorf("invalid session: %w", err)
	}

	c.session = session
	return session, nil
}

// addAuth adds authentication headers to the request.
func (c *JMAPClient) addAuth(req *http.Request) {
	authMethod := c.config.AuthMethod

	// Auto-detect auth method
	if strings.EqualFold(authMethod, "auto") {
		if c.config.AccessToken != "" {
			authMethod = "bearer"
		} else if c.config.Password != "" {
			authMethod = "basic"
		}
	}

	switch strings.ToLower(authMethod) {
	case "bearer":
		if c.config.AccessToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
		}
	case "basic":
		if c.config.Username != "" && c.config.Password != "" {
			req.SetBasicAuth(c.config.Username, c.config.Password)
		}
	}
}

// GetSession returns the discovered session.
func (c *JMAPClient) GetSession() *protocol.Session {
	return c.session
}

// TestAuth tests authentication by fetching the session.
func (c *JMAPClient) TestAuth(ctx context.Context) error {
	_, err := c.Discover(ctx)
	return err
}

// GetMailboxes fetches the list of mailboxes using JMAP.
func (c *JMAPClient) GetMailboxes(ctx context.Context) ([]protocol.Mailbox, error) {
	if c.session == nil {
		if _, err := c.Discover(ctx); err != nil {
			return nil, fmt.Errorf("failed to discover session: %w", err)
		}
	}

	// Get primary mail account ID
	accountId, ok := c.session.GetPrimaryMailAccountId()
	if !ok {
		return nil, fmt.Errorf("no primary mail account found")
	}

	// Build Mailbox/get request
	request := protocol.Request{
		Using: []string{protocol.CoreCapability, protocol.MailCapability},
		MethodCalls: []protocol.MethodCall{
			{
				Name: "Mailbox/get",
				Arguments: map[string]interface{}{
					"accountId": accountId,
				},
				CallId: "c0",
			},
		},
	}

	// Make API request
	response, err := c.makeAPIRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	// Parse response
	if len(response.MethodResponses) == 0 {
		return nil, fmt.Errorf("no method responses")
	}

	methodResp := response.MethodResponses[0]
	if methodResp.Name == "error" {
		return nil, fmt.Errorf("JMAP error: %s", string(methodResp.Arguments))
	}

	// Parse the response using the helper function
	mailboxResponse, err := protocol.ParseMailboxGetResponse(&methodResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mailbox response: %w", err)
	}

	return mailboxResponse.List, nil
}

// makeAPIRequest sends a JMAP request to the API endpoint.
func (c *JMAPClient) makeAPIRequest(ctx context.Context, request protocol.Request) (*protocol.Response, error) {
	if c.session == nil {
		return nil, fmt.Errorf("no session available")
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.session.APIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response protocol.Response
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// QueryInboxEmails queries the most recent messages from the inbox mailbox.
func (c *JMAPClient) QueryInboxEmails(ctx context.Context, limit uint32) ([]protocol.Email, error) {
	if c.session == nil {
		if _, err := c.Discover(ctx); err != nil {
			return nil, fmt.Errorf("failed to discover session: %w", err)
		}
	}

	accountId, ok := c.session.GetPrimaryMailAccountId()
	if !ok {
		return nil, fmt.Errorf("no primary mail account found")
	}

	// Find the inbox mailbox.
	mailboxes, err := c.GetMailboxes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get mailboxes: %w", err)
	}
	var inboxId protocol.Id
	for _, mb := range mailboxes {
		if mb.Role != nil && *mb.Role == "inbox" {
			inboxId = mb.Id
			break
		}
	}
	if inboxId == "" {
		return nil, fmt.Errorf("inbox mailbox not found")
	}

	queryReq := protocol.NewEmailQueryRequest(
		accountId,
		map[string]interface{}{"inMailbox": inboxId},
		limit,
	)
	queryResp, err := c.makeAPIRequest(ctx, *queryReq)
	if err != nil {
		return nil, err
	}
	if len(queryResp.MethodResponses) == 0 {
		return nil, fmt.Errorf("no method responses from Email/query")
	}
	if protocol.IsErrorResponse(queryResp.MethodResponses[0].Name) {
		return nil, fmt.Errorf("JMAP Email/query error: %s", string(queryResp.MethodResponses[0].Arguments))
	}
	emailQueryResult, err := protocol.ParseEmailQueryResponse(&queryResp.MethodResponses[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse Email/query response: %w", err)
	}

	if len(emailQueryResult.Ids) == 0 {
		return nil, nil
	}

	return c.fetchEmailDetails(ctx, accountId, emailQueryResult.Ids)
}

// QueryEmailsByFilter searches emails by Message-ID header and/or subject substring.
func (c *JMAPClient) QueryEmailsByFilter(ctx context.Context, messageID, subject string, limit uint32) ([]protocol.Email, error) {
	if c.session == nil {
		if _, err := c.Discover(ctx); err != nil {
			return nil, fmt.Errorf("failed to discover session: %w", err)
		}
	}

	accountId, ok := c.session.GetPrimaryMailAccountId()
	if !ok {
		return nil, fmt.Errorf("no primary mail account found")
	}

	filter := protocol.BuildEmailSearchFilter(messageID, subject)
	queryReq := protocol.NewEmailQueryRequest(accountId, filter, limit)
	queryResp, err := c.makeAPIRequest(ctx, *queryReq)
	if err != nil {
		return nil, err
	}
	if len(queryResp.MethodResponses) == 0 {
		return nil, fmt.Errorf("no method responses from Email/query")
	}
	if protocol.IsErrorResponse(queryResp.MethodResponses[0].Name) {
		return nil, fmt.Errorf("JMAP Email/query error: %s", string(queryResp.MethodResponses[0].Arguments))
	}
	emailQueryResult, err := protocol.ParseEmailQueryResponse(&queryResp.MethodResponses[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse Email/query response: %w", err)
	}

	if len(emailQueryResult.Ids) == 0 {
		return nil, nil
	}

	return c.fetchEmailDetails(ctx, accountId, emailQueryResult.Ids)
}

// fetchEmailDetails fetches metadata for a list of email IDs.
func (c *JMAPClient) fetchEmailDetails(ctx context.Context, accountId protocol.Id, ids []protocol.Id) ([]protocol.Email, error) {
	getReq := protocol.NewEmailGetRequest(accountId, ids,
		[]string{"id", "blobId", "messageId", "subject", "from", "to", "receivedAt", "preview", "size"})
	getResp, err := c.makeAPIRequest(ctx, *getReq)
	if err != nil {
		return nil, err
	}
	if len(getResp.MethodResponses) == 0 {
		return nil, fmt.Errorf("no method responses from Email/get")
	}
	if protocol.IsErrorResponse(getResp.MethodResponses[0].Name) {
		return nil, fmt.Errorf("JMAP Email/get error: %s", string(getResp.MethodResponses[0].Arguments))
	}
	emailGetResult, err := protocol.ParseEmailGetResponse(&getResp.MethodResponses[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse Email/get response: %w", err)
	}
	return emailGetResult.List, nil
}

// DownloadBlob downloads a blob by blobId, returning the raw bytes.
// Uses the session downloadUrl template, replacing {accountId}, {blobId}, {name}, {type}.
func (c *JMAPClient) DownloadBlob(ctx context.Context, accountId protocol.Id, blobId protocol.Id, name string) ([]byte, error) {
	if c.session == nil {
		return nil, fmt.Errorf("no session available")
	}

	dlURL := c.session.DownloadURL
	dlURL = strings.NewReplacer(
		"{accountId}", url.PathEscape(string(accountId)),
		"{blobId}", url.PathEscape(string(blobId)),
		"{name}", url.PathEscape(name),
		"{type}", "message/rfc822",
	).Replace(dlURL)

	req, err := http.NewRequestWithContext(ctx, "GET", dlURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob download request: %w", err)
	}
	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("blob download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("blob download failed with status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}
	return data, nil
}

// SendEmail creates a draft via Email/set and submits it via EmailSubmission/set.
// Falls back to Email/set only (without submission) when the server lacks the
// urn:ietf:params:jmap:submission capability.
func (c *JMAPClient) SendEmail(ctx context.Context, draft protocol.EmailCreate, mailFrom string, rcptTo []protocol.EmailAddress) error {
	if c.session == nil {
		if _, err := c.Discover(ctx); err != nil {
			return fmt.Errorf("failed to discover session: %w", err)
		}
	}

	if !c.session.HasCapability(protocol.SubmissionCapability) {
		return fmt.Errorf("JMAP server does not support %s capability — cannot send mail", protocol.SubmissionCapability)
	}

	accountId, ok := c.session.GetPrimaryMailAccountId()
	if !ok {
		return fmt.Errorf("no primary mail account found")
	}

	req := protocol.NewEmailSetAndSubmitRequest(accountId, draft, mailFrom, rcptTo)
	resp, err := c.makeAPIRequest(ctx, *req)
	if err != nil {
		return err
	}

	for _, mr := range resp.MethodResponses {
		if protocol.IsErrorResponse(mr.Name) {
			return fmt.Errorf("JMAP error in %s: %s", mr.CallId, string(mr.Arguments))
		}
	}
	return nil
}

// GetAuthMethod returns the authentication method that will be used.
func (c *JMAPClient) GetAuthMethod() string {
	authMethod := c.config.AuthMethod

	if strings.EqualFold(authMethod, "auto") {
		if c.config.AccessToken != "" {
			return "bearer"
		} else if c.config.Password != "" {
			return "basic"
		}
		return "none"
	}

	return authMethod
}
