package smtp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/Azure/go-ntlmssp"
	"msgraphtool/internal/common/ratelimit"
	"msgraphtool/internal/smtp/protocol"
	smtptls "msgraphtool/internal/smtp/tls"
)

// SMTPClient wraps SMTP connection with enhanced diagnostics.
type SMTPClient struct {
	conn         net.Conn
	reader       *bufio.Reader
	host         string
	port         int
	config       *Config
	banner       string
	capabilities protocol.Capabilities
	limiter      *ratelimit.Limiter
	smtpClient   *smtp.Client         // Reusable stdlib client after STARTTLS or SMTPS
	tlsState     *tls.ConnectionState // Stored TLS state for SMTPS connections
	ctx          context.Context      // Context for cancellation propagation
}

// debugLogCommand logs an SMTP command being sent to the server.
func (c *SMTPClient) debugLogCommand(command string) {
	if c.config != nil && c.config.VerboseMode {
		// Remove trailing CRLF for display
		cmd := strings.TrimRight(command, "\r\n")
		fmt.Printf(">>> %s\n", cmd)
	}
}

// debugLogResponse logs an SMTP response received from the server.
func (c *SMTPClient) debugLogResponse(resp *protocol.SMTPResponse) {
	if c.config != nil && c.config.VerboseMode && resp != nil {
		if len(resp.Lines) == 1 {
			fmt.Printf("<<< %d %s\n", resp.Code, resp.Message)
		} else {
			// Multiline response
			for i, line := range resp.Lines {
				if i < len(resp.Lines)-1 {
					fmt.Printf("<<< %d-%s\n", resp.Code, line)
				} else {
					fmt.Printf("<<< %d %s\n", resp.Code, line)
				}
			}
		}
	}
}

// debugLogMessage logs a debug message.
func (c *SMTPClient) debugLogMessage(message string) {
	if c.config != nil && c.config.VerboseMode {
		fmt.Printf("... %s\n", message)
	}
}

// NewSMTPClient creates a new SMTP client.
func NewSMTPClient(host string, port int, config *Config) *SMTPClient {
	return &SMTPClient{
		host:    host,
		port:    port,
		config:  config,
		limiter: ratelimit.New(config.RateLimit),
	}
}

// Connect establishes a TCP connection and reads the banner.
// For SMTPS mode, performs immediate TLS handshake before reading banner.
func (c *SMTPClient) Connect(ctx context.Context) error {
	// Apply rate limiting
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Determine connection address (override or default to host)
	connectHost := c.host
	if c.config.ConnectAddress != "" {
		connectHost = c.config.ConnectAddress
		c.debugLogMessage(fmt.Sprintf("Using override connection address: %s (SNI will use: %s)", connectHost, c.host))
	}
	addr := net.JoinHostPort(connectHost, fmt.Sprintf("%d", c.port))

	// Use context-aware dialer
	dialer := &net.Dialer{
		Timeout: c.config.Timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// SMTPS mode: perform immediate TLS handshake before any SMTP protocol
	if c.config.SMTPS {
		c.debugLogMessage("SMTPS mode: Performing immediate TLS handshake...")

		tlsVersion := smtptls.ParseTLSVersion(c.config.TLSVersion)
		tlsConfig := &tls.Config{
			ServerName:         c.host,
			InsecureSkipVerify: c.config.SkipVerify,
			MinVersion:         tlsVersion,
			MaxVersion:         tlsVersion, // Force exact TLS version
		}

		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			// Close the underlying connection; log any close error in verbose mode
			// but return the TLS error as it's more relevant for diagnostics
			if closeErr := conn.Close(); closeErr != nil {
				c.debugLogMessage(fmt.Sprintf("Warning: close error after TLS failure: %v", closeErr))
			}
			return fmt.Errorf("SMTPS TLS handshake failed: %w", err)
		}

		c.debugLogMessage("SMTPS TLS handshake completed successfully")

		conn = tlsConn
		state := tlsConn.ConnectionState()
		c.tlsState = &state

		// Create smtp.Client for later Auth/SendMail operations
		wrapper := &connWrapper{reader: bufio.NewReader(conn), conn: conn}
		c.smtpClient = &smtp.Client{Text: textproto.NewConn(wrapper)}
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// Read banner (220 response) with timeout
	resp, err := protocol.ReadResponseWithTimeout(c.reader, protocol.DefaultResponseTimeout)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to read banner: %w", err)
	}

	// Log banner response in debug mode
	c.debugLogResponse(resp)

	if !resp.IsSuccess() {
		c.conn.Close()
		return fmt.Errorf("unexpected banner response: %d %s", resp.Code, resp.Message)
	}

	c.banner = resp.Message

	// Store context for use in subsequent operations
	c.ctx = ctx

	return nil
}

// EHLO sends EHLO command and parses capabilities.
func (c *SMTPClient) EHLO(hostname string) (protocol.Capabilities, error) {
	// Apply rate limiting using stored context
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Send EHLO command
	cmd := protocol.EHLO(hostname)
	c.debugLogCommand(cmd)
	if _, err := c.conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("failed to send EHLO: %w", err)
	}

	// Read response with timeout
	resp, err := protocol.ReadResponseWithTimeout(c.reader, protocol.DefaultResponseTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to read EHLO response: %w", err)
	}

	c.debugLogResponse(resp)

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("EHLO failed: %d %s", resp.Code, resp.Message)
	}

	// Parse capabilities
	c.capabilities = protocol.ParseCapabilities(resp.Lines)

	return c.capabilities, nil
}

// StartTLS upgrades the connection to TLS.
func (c *SMTPClient) StartTLS(tlsConfig *tls.Config) (*tls.ConnectionState, error) {
	// Apply rate limiting using stored context
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Send STARTTLS command
	cmd := protocol.STARTTLS()
	c.debugLogCommand(cmd)
	if _, err := c.conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("failed to send STARTTLS: %w", err)
	}

	// Read response (expect 220) with timeout
	resp, err := protocol.ReadResponseWithTimeout(c.reader, protocol.DefaultResponseTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to read STARTTLS response: %w", err)
	}

	c.debugLogResponse(resp)

	if resp.Code != 220 {
		return nil, fmt.Errorf("STARTTLS failed: %d %s", resp.Code, resp.Message)
	}

	// Perform TLS handshake
	c.debugLogMessage("Performing TLS handshake...")
	tlsConn := tls.Client(c.conn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	c.debugLogMessage("TLS handshake completed successfully")

	// Update connection and reader
	c.conn = tlsConn
	c.reader = bufio.NewReader(tlsConn)

	// Create a reusable stdlib smtp.Client for Auth and SendMail
	// We do this here to avoid creating multiple clients with conflicting buffered readers
	wrapper := &connWrapper{
		reader: c.reader,
		conn:   c.conn,
	}
	c.smtpClient = &smtp.Client{Text: textproto.NewConn(wrapper)}

	// Get connection state and store it
	state := tlsConn.ConnectionState()
	c.tlsState = &state

	return &state, nil
}

// connWrapper wraps our existing buffered reader and connection into an io.ReadWriteCloser
// This allows us to reuse the existing buffer while creating a proper textproto.Conn
type connWrapper struct {
	reader *bufio.Reader
	conn   net.Conn
}

func (cw *connWrapper) Read(p []byte) (n int, err error) {
	return cw.reader.Read(p)
}

func (cw *connWrapper) Write(p []byte) (n int, err error) {
	return cw.conn.Write(p)
}

func (cw *connWrapper) Close() error {
	return nil // Don't close the underlying connection, we'll manage it ourselves
}

// Auth performs SMTP authentication.
// For XOAUTH2, pass the OAuth2 access token in the accessToken parameter.
func (c *SMTPClient) Auth(username, password, accessToken string, mechanisms []string) error {
	// Apply rate limiting using stored context
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Determine which mechanism to use
	mechanism := selectAuthMechanism(mechanisms, c.capabilities.GetAuthMechanisms(), accessToken != "")
	if mechanism == "" {
		return fmt.Errorf("no compatible authentication mechanism found")
	}

	c.debugLogMessage(fmt.Sprintf("Starting authentication with mechanism: %s", mechanism))

	// Create appropriate auth
	var auth smtp.Auth
	switch mechanism {
	case "PLAIN":
		// Use our custom plainAuth instead of smtp.PlainAuth because
		// we manage TLS ourselves and smtp.PlainAuth would reject the
		// connection thinking it's unencrypted
		auth = &plainAuth{username, password}
	case "LOGIN":
		auth = &loginAuth{username, password}
	case "CRAM-MD5":
		auth = smtp.CRAMMD5Auth(username, password)
	case "XOAUTH2":
		auth = &xoauth2Auth{username, accessToken}
	case "NTLM":
		auth = &ntlmAuth{username: username, password: password}
	default:
		return fmt.Errorf("unsupported authentication mechanism: %s", mechanism)
	}

	// Use the reusable smtp.Client created after STARTTLS
	// If not available (no STARTTLS), create one now
	if c.smtpClient == nil {
		wrapper := &connWrapper{
			reader: c.reader,
			conn:   c.conn,
		}
		c.smtpClient = &smtp.Client{Text: textproto.NewConn(wrapper)}
	}

	c.debugLogMessage(fmt.Sprintf(">>> AUTH %s (credentials exchanged via SASL)", mechanism))

	// Call Hello to initialize the client state properly
	// This sends EHLO again, which is required by smtp.Client.Auth()
	if err := c.smtpClient.Hello(c.host); err != nil {
		c.debugLogMessage("<<< EHLO for auth failed")
		return fmt.Errorf("EHLO for auth failed: %w", err)
	}

	if err := c.smtpClient.Auth(auth); err != nil {
		c.debugLogMessage("<<< Authentication failed")
		return fmt.Errorf("authentication failed: %w", err)
	}

	c.debugLogMessage("<<< 235 Authentication successful")

	return nil
}

// SendMail sends an email message.
func (c *SMTPClient) SendMail(from string, to []string, data []byte) error {
	// Apply rate limiting using stored context
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Use the reusable smtp.Client created after STARTTLS (or Auth)
	// If not available, create one now
	if c.smtpClient == nil {
		wrapper := &connWrapper{
			reader: c.reader,
			conn:   c.conn,
		}
		c.smtpClient = &smtp.Client{Text: textproto.NewConn(wrapper)}
	}
	smtpClient := c.smtpClient

	// MAIL FROM
	c.debugLogMessage(fmt.Sprintf(">>> MAIL FROM:<%s>", from))
	if err := smtpClient.Mail(from); err != nil {
		c.debugLogMessage(fmt.Sprintf("<<< MAIL FROM failed: %v", err))
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	c.debugLogMessage("<<< 250 Sender OK")

	// RCPT TO
	for _, recipient := range to {
		c.debugLogMessage(fmt.Sprintf(">>> RCPT TO:<%s>", recipient))
		if err := smtpClient.Rcpt(recipient); err != nil {
			c.debugLogMessage(fmt.Sprintf("<<< RCPT TO failed: %v", err))
			return fmt.Errorf("RCPT TO failed for %s: %w", recipient, err)
		}
		c.debugLogMessage("<<< 250 Recipient OK")
	}

	// DATA
	c.debugLogMessage(">>> DATA")
	w, err := smtpClient.Data()
	if err != nil {
		c.debugLogMessage(fmt.Sprintf("<<< DATA failed: %v", err))
		return fmt.Errorf("DATA command failed: %w", err)
	}
	c.debugLogMessage("<<< 354 Start mail input; end with <CRLF>.<CRLF>")

	// Send message body
	if c.config.VerboseMode {
		// Show first few lines of message in debug mode
		lines := strings.Split(string(data), "\n")
		if len(lines) > 5 {
			c.debugLogMessage(fmt.Sprintf("... Sending message (%d bytes, %d lines):", len(data), len(lines)))
			for i := 0; i < 3; i++ {
				c.debugLogMessage(fmt.Sprintf("    %s", strings.TrimRight(lines[i], "\r")))
			}
			c.debugLogMessage("    ...")
		}
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	c.debugLogMessage(">>> . (end of message)")
	if err := w.Close(); err != nil {
		c.debugLogMessage(fmt.Sprintf("<<< Message send failed: %v", err))
		return fmt.Errorf("failed to close DATA: %w", err)
	}
	c.debugLogMessage("<<< 250 Message accepted for delivery")

	return nil
}

// Close closes the connection.
func (c *SMTPClient) Close() error {
	if c.conn != nil {
		// Send QUIT
		cmd := protocol.QUIT()
		c.debugLogCommand(cmd)
		_, _ = c.conn.Write([]byte(cmd))
		// Note: We don't wait for the response as the connection is being closed
		c.debugLogMessage("<<< 221 Closing connection")
		return c.conn.Close()
	}
	return nil
}

// GetBanner returns the server banner.
func (c *SMTPClient) GetBanner() string {
	return c.banner
}

// GetCapabilities returns the server capabilities.
func (c *SMTPClient) GetCapabilities() protocol.Capabilities {
	return c.capabilities
}

// IsEncrypted returns true if the connection is using TLS (either SMTPS or STARTTLS).
func (c *SMTPClient) IsEncrypted() bool {
	return c.tlsState != nil
}

// GetTLSState returns the stored TLS connection state (for SMTPS connections).
func (c *SMTPClient) GetTLSState() *tls.ConnectionState {
	return c.tlsState
}

// selectAuthMechanism selects the best authentication mechanism.
// If hasAccessToken is true, XOAUTH2 is preferred when available.
func selectAuthMechanism(requested []string, available []string, hasAccessToken bool) string {
	// If specific mechanism requested
	if len(requested) > 0 && requested[0] != "auto" {
		for _, req := range requested {
			for _, avail := range available {
				if strings.EqualFold(req, avail) {
					return strings.ToUpper(req)
				}
			}
		}
		return ""
	}

	// Auto-select: prefer XOAUTH2 if access token provided, otherwise prefer stronger mechanisms
	var preferenceOrder []string
	if hasAccessToken {
		preferenceOrder = []string{"XOAUTH2", "CRAM-MD5", "NTLM", "PLAIN", "LOGIN"}
	} else {
		preferenceOrder = []string{"CRAM-MD5", "NTLM", "PLAIN", "LOGIN"}
	}

	for _, preferred := range preferenceOrder {
		for _, avail := range available {
			if strings.EqualFold(preferred, avail) {
				return preferred
			}
		}
	}

	return ""
}

// plainAuth implements PLAIN authentication without TLS requirement checks.
// We use this instead of smtp.PlainAuth because we manage TLS ourselves
// and the stdlib smtp.Client doesn't know we've already upgraded the connection.
type plainAuth struct {
	username string
	password string
}

func (a *plainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// PLAIN auth sends: \0username\0password
	resp := []byte("\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *plainAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	// PLAIN is a single-step authentication, no further responses needed
	return nil, nil
}

// loginAuth implements LOGIN authentication.
type loginAuth struct {
	username string
	password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		prompt := strings.ToLower(string(fromServer))
		if strings.Contains(prompt, "username") {
			return []byte(a.username), nil
		} else if strings.Contains(prompt, "password") {
			return []byte(a.password), nil
		}
	}
	return nil, nil
}

// xoauth2Auth implements XOAUTH2 authentication (OAuth 2.0 bearer token).
// Token format: user=<email>\x01auth=Bearer <token>\x01\x01
type xoauth2Auth struct {
	username    string
	accessToken string
}

func (a *xoauth2Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// XOAUTH2 token format: "user=" + email + "\x01" + "auth=Bearer " + token + "\x01\x01"
	token := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.accessToken)
	return "XOAUTH2", []byte(token), nil
}

func (a *xoauth2Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	// XOAUTH2 is single-step, but server may send error JSON on failure
	// Return empty to signal we have nothing more to send
	return nil, nil
}

// ntlmAuth implements NTLM (NTLMv2) authentication for SMTP.
// The three-step exchange: negotiate (type 1) → challenge (type 2) → authenticate (type 3).
// Username may be in DOMAIN\user format; the library handles domain extraction automatically.
type ntlmAuth struct {
	username string
	password string
}

func (a *ntlmAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	negotiate, err := ntlmssp.NewNegotiateMessage("", "")
	if err != nil {
		return "", nil, fmt.Errorf("NTLM negotiate: %w", err)
	}
	return "NTLM", negotiate, nil
}

func (a *ntlmAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	authenticate, err := ntlmssp.NewAuthenticateMessage(fromServer, a.username, a.password, nil)
	if err != nil {
		return nil, fmt.Errorf("NTLM authenticate: %w", err)
	}
	return authenticate, nil
}
