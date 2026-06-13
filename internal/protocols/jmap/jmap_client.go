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
	"strconv"
	"strings"
	"time"

	"github.com/ziembor/gomailtesttool/internal/common/network"
	"github.com/ziembor/gomailtesttool/internal/jmap/protocol"
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
