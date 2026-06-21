package main

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"gomailtesttool/internal/common/ratelimit"
	"gomailtesttool/internal/pop3/protocol"
	smtptls "gomailtesttool/internal/smtp/tls"
)

// POP3Client wraps a POP3 connection with additional functionality.
type POP3Client struct {
	conn     net.Conn
	reader   *bufio.Reader
	host     string
	port     int
	config   *Config
	greeting string
	caps     *protocol.Capabilities
	limiter  *ratelimit.Limiter
	tlsState *tls.ConnectionState
}

// NewPOP3Client creates a new POP3 client.
func NewPOP3Client(config *Config) *POP3Client {
	var limiter *ratelimit.Limiter
	if config.RateLimit > 0 {
		limiter = ratelimit.New(config.RateLimit)
	}

	return &POP3Client{
		host:    config.Host,
		port:    config.Port,
		config:  config,
		limiter: limiter,
	}
}

// Connect establishes a connection to the POP3 server.
func (c *POP3Client) Connect(ctx context.Context) error {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	address := fmt.Sprintf("%s:%d", c.host, c.port)

	var conn net.Conn
	var err error

	dialer := &net.Dialer{
		Timeout: c.config.Timeout,
	}

	if c.config.POP3S {
		// Implicit TLS (POP3S)
		tlsConfig := &tls.Config{
			ServerName:         c.host,
			InsecureSkipVerify: c.config.SkipVerify,
			MinVersion:         smtptls.ParseTLSVersion(c.config.TLSVersion),
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
		if err != nil {
			return fmt.Errorf("POP3S connection failed: %w", err)
		}
		// Store TLS state
		if tlsConn, ok := conn.(*tls.Conn); ok {
			state := tlsConn.ConnectionState()
			c.tlsState = &state
		}
	} else {
		// Plain connection
		conn, err = dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// Read server greeting
	resp, err := protocol.ReadResponse(c.reader)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to read greeting: %w", err)
	}
	if !resp.Success {
		c.conn.Close()
		return fmt.Errorf("server rejected connection: %s", resp.Message)
	}
	c.greeting = resp.Message

	return nil
}

// GetGreeting returns the server greeting.
func (c *POP3Client) GetGreeting() string {
	return c.greeting
}

// GetTLSState returns the TLS connection state (if TLS is active).
func (c *POP3Client) GetTLSState() *tls.ConnectionState {
	return c.tlsState
}

// StartTLS upgrades the connection to TLS using STLS command.
func (c *POP3Client) StartTLS(tlsConfig *tls.Config) error {
	if c.limiter != nil {
		ctx := context.Background()
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	// Send STLS command
	if _, err := c.conn.Write([]byte(protocol.STLS())); err != nil {
		return fmt.Errorf("failed to send STLS: %w", err)
	}

	resp, err := protocol.ReadResponse(c.reader)
	if err != nil {
		return fmt.Errorf("failed to read STLS response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("STLS failed: %s", resp.Message)
	}

	// Upgrade to TLS
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			ServerName:         c.host,
			InsecureSkipVerify: c.config.SkipVerify,
			MinVersion:         smtptls.ParseTLSVersion(c.config.TLSVersion),
		}
	}

	tlsConn := tls.Client(c.conn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("TLS handshake failed: %w", err)
	}

	c.conn = tlsConn
	c.reader = bufio.NewReader(tlsConn)
	state := tlsConn.ConnectionState()
	c.tlsState = &state

	return nil
}

// Capabilities retrieves server capabilities using CAPA command.
func (c *POP3Client) Capabilities(ctx context.Context) (*protocol.Capabilities, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	if _, err := c.conn.Write([]byte(protocol.CAPA())); err != nil {
		return nil, fmt.Errorf("failed to send CAPA: %w", err)
	}

	resp, err := protocol.ReadMultilineResponse(c.reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read CAPA response: %w", err)
	}
	if !resp.Success {
		// CAPA not supported, return empty capabilities
		c.caps = protocol.NewCapabilities(nil)
		return c.caps, nil
	}

	c.caps = protocol.NewCapabilities(resp.Lines)
	return c.caps, nil
}

// GetCapabilities returns cached capabilities or fetches them.
func (c *POP3Client) GetCapabilities() *protocol.Capabilities {
	return c.caps
}

// Auth authenticates with the server using the specified method.
func (c *POP3Client) Auth(ctx context.Context, username, password, accessToken string) error {
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
		} else if c.caps != nil && c.caps.SupportsUSER() {
			method = "USER"
		} else {
			method = "USER" // Fallback
		}
	}

	switch strings.ToUpper(method) {
	case "XOAUTH2":
		return c.authXOAUTH2(username, accessToken)
	case "APOP":
		return c.authAPOP(username, password)
	case "USER", "":
		return c.authUSER(username, password)
	default:
		return fmt.Errorf("unsupported auth method: %s", method)
	}
}

// authUSER performs USER/PASS authentication.
func (c *POP3Client) authUSER(username, password string) error {
	// Send USER command
	if _, err := c.conn.Write([]byte(protocol.USER(username))); err != nil {
		return fmt.Errorf("failed to send USER: %w", err)
	}

	resp, err := protocol.ReadResponse(c.reader)
	if err != nil {
		return fmt.Errorf("failed to read USER response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("USER failed: %s", resp.Message)
	}

	// Send PASS command
	if _, err := c.conn.Write([]byte(protocol.PASS(password))); err != nil {
		return fmt.Errorf("failed to send PASS: %w", err)
	}

	resp, err = protocol.ReadResponse(c.reader)
	if err != nil {
		return fmt.Errorf("failed to read PASS response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("PASS failed: %s", resp.Message)
	}

	return nil
}

// authAPOP performs APOP authentication.
func (c *POP3Client) authAPOP(username, password string) error {
	// Extract timestamp from greeting
	timestamp := protocol.ParseGreeting(c.greeting)
	if timestamp == "" {
		return fmt.Errorf("APOP not supported: no timestamp in greeting")
	}

	// Calculate MD5 digest
	digest := fmt.Sprintf("%x", md5.Sum([]byte(timestamp+password)))

	// Send APOP command
	if _, err := c.conn.Write([]byte(protocol.APOP(username, digest))); err != nil {
		return fmt.Errorf("failed to send APOP: %w", err)
	}

	resp, err := protocol.ReadResponse(c.reader)
	if err != nil {
		return fmt.Errorf("failed to read APOP response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("APOP failed: %s", resp.Message)
	}

	return nil
}

// authXOAUTH2 performs XOAUTH2 authentication.
func (c *POP3Client) authXOAUTH2(username, accessToken string) error {
	// Build XOAUTH2 token
	token := protocol.XOAUTH2Token(username, accessToken)
	encoded := base64.StdEncoding.EncodeToString([]byte(token))

	// Send AUTH XOAUTH2 command
	if _, err := c.conn.Write([]byte(protocol.AUTH("XOAUTH2", encoded))); err != nil {
		return fmt.Errorf("failed to send AUTH XOAUTH2: %w", err)
	}

	resp, err := protocol.ReadResponse(c.reader)
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("XOAUTH2 authentication failed: %s", resp.Message)
	}

	return nil
}

// Stat returns mailbox statistics.
func (c *POP3Client) Stat(ctx context.Context) (count int, size int64, err error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return 0, 0, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	if _, err := c.conn.Write([]byte(protocol.STAT())); err != nil {
		return 0, 0, fmt.Errorf("failed to send STAT: %w", err)
	}

	resp, err := protocol.ReadResponse(c.reader)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read STAT response: %w", err)
	}

	return protocol.ParseStatResponse(resp)
}

// List returns information about all messages.
func (c *POP3Client) List(ctx context.Context) ([]protocol.MessageInfo, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	if _, err := c.conn.Write([]byte(protocol.LIST(0))); err != nil {
		return nil, fmt.Errorf("failed to send LIST: %w", err)
	}

	resp, err := protocol.ReadMultilineResponse(c.reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read LIST response: %w", err)
	}

	return protocol.ParseListResponse(resp)
}

// UIDL returns unique IDs for all messages.
func (c *POP3Client) UIDL(ctx context.Context) ([]protocol.MessageInfo, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	if _, err := c.conn.Write([]byte(protocol.UIDL(0))); err != nil {
		return nil, fmt.Errorf("failed to send UIDL: %w", err)
	}

	resp, err := protocol.ReadMultilineResponse(c.reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read UIDL response: %w", err)
	}

	return protocol.ParseUIDLResponse(resp)
}

// Quit sends the QUIT command and closes the connection.
func (c *POP3Client) Quit() error {
	if c.conn == nil {
		return nil
	}

	// Send QUIT command (ignore errors)
	_, _ = c.conn.Write([]byte(protocol.QUIT()))

	// Read response (ignore errors)
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = protocol.ReadResponse(c.reader)

	return c.conn.Close()
}

// Close closes the connection without sending QUIT.
func (c *POP3Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
