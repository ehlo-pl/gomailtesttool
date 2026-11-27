package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/msoauth"
	"github.com/ehlo-pl/gomailtesttool/internal/common/endpoint"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/imap"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/jmap"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/pop3"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// opener builds protocol sessions from endpoints, reusing the gomailtest
// protocol packages and their ConfigFromViper.
type opener struct {
	cfg *config.Config
	log *slog.Logger
}

func (o *opener) endpoint(name string) (*endpoint.Endpoint, error) {
	ep, ok := o.cfg.Endpoints[name]
	if !ok {
		return nil, fmt.Errorf("endpoint %q is not defined", name)
	}
	return ep, nil
}

func smtpConfig(ep *endpoint.Endpoint) *smtp.Config {
	v := ep.Viper()
	cfg := smtp.ConfigFromViper(v)
	cfg.VerboseMode = false
	return cfg
}

func imapConfig(ep *endpoint.Endpoint) *imap.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	// "mailbox" is not a setting here: both inbox and junk are searched.
	cfg := imap.ConfigFromViper(v)
	if cfg.IMAPS && !v.IsSet("port") {
		cfg.Port = 993
	}
	cfg.VerboseMode = false
	return cfg
}

func pop3Config(ep *endpoint.Endpoint) *pop3.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	cfg := pop3.ConfigFromViper(v)
	if cfg.POP3S && !v.IsSet("port") {
		cfg.Port = 995
	}
	cfg.VerboseMode = false
	return cfg
}

func jmapConfig(ep *endpoint.Endpoint) *jmap.Config {
	cfg := jmap.ConfigFromViper(ep.Viper())
	cfg.VerboseMode = false
	return cfg
}

func (o *opener) account(ep *endpoint.Endpoint) (*msoauth.Account, error) {
	return msoauth.New(o.cfg.DataDir, ep.Name, ep.String("clientid"), ep.String("tenantid"))
}

// imapWithToken adds a fresh access token for endpoints with "oauth".
func (o *opener) imapWithToken(ctx context.Context, ep *endpoint.Endpoint) (*imap.Config, error) {
	cfg := imapConfig(ep)
	if ep.String("oauth") == "" {
		return cfg, nil
	}
	a, err := o.account(ep)
	if err != nil {
		return nil, err
	}
	if cfg.AccessToken, err = a.AccessToken(ctx); err != nil {
		return nil, fmt.Errorf("endpoint %s: %w", ep.Name, err)
	}
	cfg.AuthMethod = "XOAUTH2"
	return cfg, nil
}

// Sender implements engine.Opener.
func (o *opener) Sender(_ context.Context, name string) (send.Sender, error) {
	ep, err := o.endpoint(name)
	if err != nil {
		return nil, err
	}
	return smtp.OpenSender(smtpConfig(ep), o.log)
}

// Mailbox implements engine.Opener.
func (o *opener) Mailbox(ctx context.Context, name string) (mailbox.Session, error) {
	ep, err := o.endpoint(name)
	if err != nil {
		return nil, err
	}
	switch ep.Protocol {
	case "imap":
		cfg, err := o.imapWithToken(ctx, ep)
		if err != nil {
			return nil, err
		}
		return imap.OpenMailbox(ctx, cfg, ep.String("junk"))
	case "jmap":
		return jmap.OpenMailbox(ctx, jmapConfig(ep))
	}
	return nil, fmt.Errorf("endpoint %q: protocol %s cannot be a target", name, ep.Protocol)
}

// Fetcher implements engine.Opener (bounce mailboxes).
func (o *opener) Fetcher(ctx context.Context, name string) (fetch.Session, error) {
	ep, err := o.endpoint(name)
	if err != nil {
		return nil, err
	}
	switch ep.Protocol {
	case "imap":
		cfg, err := o.imapWithToken(ctx, ep)
		if err != nil {
			return nil, err
		}
		return imap.OpenFetchSession(ctx, cfg, o.log)
	case "pop3":
		return pop3.OpenFetchSession(ctx, pop3Config(ep), o.log)
	}
	return nil, fmt.Errorf("endpoint %q: protocol %s cannot read bounces", name, ep.Protocol)
}

// checkEndpoints validates the settings of every endpoint before any
// network work.
func checkEndpoints(cfg *config.Config) error {
	var errs []error
	for name, ep := range cfg.Endpoints {
		if err := checkEndpoint(ep); err != nil {
			errs = append(errs, fmt.Errorf("endpoint %s (%s): %w", name, ep.Protocol, err))
		}
	}
	return errors.Join(errs...)
}

func checkEndpoint(ep *endpoint.Endpoint) error {
	host := func(h string, port int) error {
		if h == "" {
			return errors.New("'host' is required")
		}
		if err := validation.ValidateHostname(h); err != nil {
			return fmt.Errorf("invalid host: %w", err)
		}
		return validation.ValidatePort(port)
	}
	login := func(user, secret string) error {
		if user == "" {
			return errors.New("'username' is required")
		}
		if secret == "" {
			return errors.New("'password' or 'accesstoken' is required")
		}
		return nil
	}
	switch ep.Protocol {
	case "smtp":
		c := smtpConfig(ep)
		if c.SMTPS && c.StartTLS {
			return errors.New("'smtps' and 'starttls' are mutually exclusive")
		}
		if c.UseMX {
			return errors.New("'use-mx' is not supported: warm-up mail must leave through the site's own mail server")
		}
		return host(c.Host, c.Port)
	case "imap":
		c := imapConfig(ep)
		if err := host(c.Host, c.Port); err != nil {
			return err
		}
		if !c.IMAPS && !c.StartTLS {
			return errors.New("'imaps: true' or 'starttls: true' is required")
		}
		switch oauth := strings.ToLower(ep.String("oauth")); oauth {
		case "":
			return login(c.Username, c.Password+c.AccessToken)
		case config.OAuthMicrosoft:
			if c.Username == "" || ep.String("clientid") == "" {
				return errors.New("'username' and 'clientid' are required with oauth: microsoft")
			}
			return nil
		default:
			return fmt.Errorf("oauth: %q is not supported (microsoft)", oauth)
		}
	case "pop3":
		c := pop3Config(ep)
		if err := host(c.Host, c.Port); err != nil {
			return err
		}
		if !c.POP3S && !c.StartTLS {
			return errors.New("'pop3s: true' or 'starttls: true' is required")
		}
		return login(c.Username, c.Password+c.AccessToken)
	case "jmap":
		c := jmapConfig(ep)
		if err := host(c.Host, c.Port); err != nil {
			return err
		}
		if c.AccessToken == "" && (c.Username == "" || c.Password == "") {
			return errors.New("'accesstoken' (API token) is required")
		}
	}
	return nil
}
