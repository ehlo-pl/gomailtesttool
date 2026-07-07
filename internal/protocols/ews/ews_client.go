package ews

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Azure/go-ntlmssp"
	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
)

const (
	maxResponseBodyBytes = 4 << 20 // 4 MiB

	soapEnvelope = `<?xml version="1.0" encoding="utf-8" ?>
<soap:Envelope xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
    xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types"
    xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Header>
    <t:RequestServerVersion Version="Exchange2013_SP1" />
  </soap:Header>
  <soap:Body>
%s
  </soap:Body>
</soap:Envelope>`

	autodiscoverEnvelope = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
    xmlns:a="http://schemas.microsoft.com/exchange/2010/Autodiscover">
  <soap:Header>
    <a:RequestedServerVersion>Exchange2013</a:RequestedServerVersion>
  </soap:Header>
  <soap:Body>
    <a:GetUserSettingsRequestMessage>
      <a:Request>
        <a:Users>
          <a:User>
            <a:Mailbox>%s</a:Mailbox>
          </a:User>
        </a:Users>
        <a:RequestedSettings>
          <a:Setting>InternalEwsUrl</a:Setting>
          <a:Setting>ExternalEwsUrl</a:Setting>
          <a:Setting>UserDisplayName</a:Setting>
          <a:Setting>ActiveDirectoryServer</a:Setting>
        </a:RequestedSettings>
      </a:Request>
    </a:GetUserSettingsRequestMessage>
  </soap:Body>
</soap:Envelope>`
)

// EWSClient performs HTTP/SOAP requests against an Exchange EWS endpoint.
type EWSClient struct {
	httpClient      *http.Client
	config          *Config
	ewsURL          string
	autodiscoverURL string
}

// NewEWSClient builds an EWSClient with the appropriate transport based on config.
func NewEWSClient(config *Config) (*EWSClient, error) {
	tlsCfg, err := buildTLSConfig(config)
	if err != nil {
		return nil, err
	}

	baseTransport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	// Resolve to a specific address family if --ipv4/--ipv6 was requested
	if config.IPv4Only || config.IPv6Only {
		dialer := &net.Dialer{}
		baseTransport.DialContext = func(ctx context.Context, dialNetwork, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return dialer.DialContext(ctx, dialNetwork, addr)
			}
			host, err = network.ResolveForDial(ctx, host, config.IPv4Only, config.IPv6Only)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, dialNetwork, net.JoinHostPort(host, port))
		}
	}

	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		baseTransport.Proxy = http.ProxyURL(proxyURL)
	}

	var transport http.RoundTripper = baseTransport
	if strings.EqualFold(config.AuthMethod, "NTLM") {
		transport = ntlmssp.Negotiator{RoundTripper: baseTransport}
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	scheme := "https"
	if config.Port == 80 {
		scheme = "http"
	}

	hostPort := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	ewsURL := fmt.Sprintf("%s://%s%s", scheme, hostPort, config.EWSPath)
	autodiscoverURL := fmt.Sprintf("%s://%s%s", scheme, hostPort, config.AutodiscoverPath)

	return &EWSClient{
		httpClient:      httpClient,
		config:          config,
		ewsURL:          ewsURL,
		autodiscoverURL: autodiscoverURL,
	}, nil
}

// Probe performs an HTTP GET to the EWS endpoint without credentials.
// Returns the HTTP response (or nil on network error) and any connection-level error.
// HTTP 401/403 are valid "server alive" responses — only network errors are returned as error.
func (c *EWSClient) Probe(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.ewsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// drain body so connection can be reused, then close
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp, nil
}

// SendSOAP wraps body in an EWS SOAP envelope, applies auth, and POSTs to the EWS URL.
func (c *EWSClient) SendSOAP(ctx context.Context, soapBody string) ([]byte, error) {
	payload := fmt.Sprintf(soapEnvelope, soapBody)
	req, err := http.NewRequestWithContext(ctx, "POST", c.ewsURL, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `""`)
	c.applyAuth(req)

	if c.config.Mailbox != "" {
		req.Header.Set("X-AnchorMailbox", c.config.Mailbox)
	}

	if c.config.VerboseMode {
		fmt.Printf(">>> POST %s\n", c.ewsURL)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if c.config.VerboseMode {
		fmt.Printf("<<< HTTP %s\n", resp.Status)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}

	return respBody, nil
}

// SendAutodiscover POSTs a GetUserSettings SOAP request to the Autodiscover endpoint.
func (c *EWSClient) SendAutodiscover(ctx context.Context, email string) ([]byte, error) {
	var escapedEmail strings.Builder
	if err := xml.EscapeText(&escapedEmail, []byte(email)); err != nil {
		return nil, fmt.Errorf("escaping email: %w", err)
	}
	payload := fmt.Sprintf(autodiscoverEnvelope, escapedEmail.String())
	req, err := http.NewRequestWithContext(ctx, "POST", c.autodiscoverURL, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"http://schemas.microsoft.com/exchange/2010/Autodiscover/Autodiscover/GetUserSettings"`)
	c.applyAuth(req)

	if c.config.VerboseMode {
		fmt.Printf(">>> POST %s\n", c.autodiscoverURL)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if c.config.VerboseMode {
		fmt.Printf("<<< HTTP %s\n", resp.Status)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}

	return respBody, nil
}

// EWSUrl returns the configured EWS endpoint URL.
func (c *EWSClient) EWSUrl() string { return c.ewsURL }

// AutodiscoverUrl returns the configured Autodiscover endpoint URL.
func (c *EWSClient) AutodiscoverUrl() string { return c.autodiscoverURL }

// applyAuth sets the appropriate Authorization header on the request.
func (c *EWSClient) applyAuth(req *http.Request) {
	switch strings.ToUpper(c.config.AuthMethod) {
	case "NTLM":
		// ntlmssp.Negotiator transport reads Basic auth credentials and performs NTLM handshake
		req.SetBasicAuth(c.config.Username, c.config.Password)
	case "BEARER":
		req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
	default: // Basic
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}
}

// buildTLSConfig returns the tls.Config for the given Config.
func buildTLSConfig(config *Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: config.SkipVerify, //nolint:gosec // user-controlled flag with warning shown in validateConfiguration
		MinVersion:         tlsutil.ParseTLSVersion(config.TLSVersion),
	}
	return tlsCfg, nil
}
