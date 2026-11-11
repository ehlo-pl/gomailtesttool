package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
	tlsutil "github.com/ehlo-pl/gomailtesttool/internal/common/tls"
)

// ehloName is the name announced in EHLO (same as the sendmail action).
const ehloName = "smtptool.local"

// rawSender implements send.Sender over SMTP. Every Send opens a fresh
// connection, which is what a mail-flow probe wants to exercise.
type rawSender struct {
	config *Config
	log    *slog.Logger
}

// OpenSender returns a quiet SMTP sender for config. Unlike SendMail it prints
// nothing and writes no CSV rows. The connection is opened per Send.
// With UseMX the MX lookup domain is taken from the first recipient's domain
// when Host is empty.
func OpenSender(config *Config, log *slog.Logger) (send.Sender, error) {
	if config == nil {
		return nil, errors.New("smtp: nil config")
	}
	if config.Host == "" && !config.UseMX {
		return nil, errors.New("smtp: host is required (or use-mx)")
	}
	if config.SMTPS && config.StartTLS {
		return nil, errors.New("smtp: smtps and starttls are mutually exclusive")
	}
	if config.StartTLS && config.NoStartTLS {
		return nil, errors.New("smtp: starttls and no-starttls are mutually exclusive")
	}
	cfg := *config
	cfg.VerboseMode = false // the SMTP client prints the dialogue to stdout in verbose mode
	if cfg.SMTPS && !cfg.PortExplicit && cfg.Port == 587 {
		cfg.Port = 465
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &rawSender{config: &cfg, log: log}, nil
}

// wantsStartTLS mirrors the sendmail action: upgrade when forced, or
// opportunistically on the usual submission/relay ports.
func wantsStartTLS(c *Config) bool {
	if c.SMTPS || c.NoStartTLS {
		return false
	}
	if c.StartTLS {
		return true
	}
	switch c.Port {
	case 25, 587, 2525, 2526, 1025:
		return true
	}
	return false
}

// Send connects, upgrades to TLS when available, authenticates when
// credentials are configured and submits m.Raw.
func (s *rawSender) Send(ctx context.Context, m send.Message) (send.Result, error) {
	var res send.Result
	if m.From == "" || len(m.To) == 0 {
		return res, errors.New("smtp: envelope sender and at least one recipient are required")
	}
	cfg := *s.config
	host := cfg.Host
	if host == "" && cfg.UseMX {
		_, domain, ok := cutAddress(m.To[0])
		if !ok {
			return res, fmt.Errorf("smtp: cannot derive MX domain from recipient %q", m.To[0])
		}
		host = domain
	}

	client := NewSMTPClient(host, cfg.Port, &cfg)
	if err := client.Connect(ctx); err != nil {
		return res, fmt.Errorf("smtp connect %s:%d: %w", host, cfg.Port, err)
	}
	defer func() { _ = client.Close() }()
	res.RemoteHost = client.GetHost()

	caps, err := client.EHLO(ehloName)
	if err != nil {
		return res, err
	}

	if wantsStartTLS(&cfg) {
		switch {
		case caps.SupportsSTARTTLS():
			v := tlsutil.ParseTLSVersion(cfg.TLSVersion)
			if _, err := client.StartTLS(&tls.Config{
				ServerName:         client.GetHost(),
				InsecureSkipVerify: cfg.SkipVerify, //nolint:gosec // explicit operator opt-in (skipverify)
				MinVersion:         v,
				MaxVersion:         v,
			}); err != nil {
				return res, fmt.Errorf("STARTTLS failed: %w", err)
			}
			if caps, err = client.EHLO(ehloName); err != nil {
				return res, fmt.Errorf("EHLO on encrypted connection failed: %w", err)
			}
		case cfg.StartTLS:
			return res, errors.New("STARTTLS required but not offered by the server")
		}
	}
	if st := client.GetTLSState(); st != nil {
		res.TLSVersion = tlsutil.TLSVersionString(st.Version)
	}

	if cfg.Username != "" && (cfg.Password != "" || cfg.AccessToken != "") {
		method := selectAuthMechanism([]string{cfg.AuthMethod}, caps.GetAuthMechanisms(), cfg.AccessToken != "")
		if method == "" {
			return res, errors.New("no compatible authentication mechanism found")
		}
		if err := client.Auth(cfg.Username, cfg.Password, cfg.AccessToken, []string{method}); err != nil {
			return res, fmt.Errorf("smtp authentication (%s) failed for %s: %w", method, security.MaskUsername(cfg.Username), err)
		}
	}

	resp, err := client.SendMail(m.From, m.To, m.Raw)
	if err != nil {
		return res, err
	}
	res.ServerResponse = resp
	s.log.Debug("smtp message accepted", "host", res.RemoteHost, "port", cfg.Port, "tls", res.TLSVersion, "response", resp)
	return res, nil
}

func (s *rawSender) Close() error { return nil }

// cutAddress splits local@domain.
func cutAddress(addr string) (local, domain string, ok bool) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == '@' {
			return addr[:i], addr[i+1:], i > 0 && i < len(addr)-1
		}
	}
	return "", "", false
}
