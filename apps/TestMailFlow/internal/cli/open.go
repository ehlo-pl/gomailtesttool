package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/gmail"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/imap"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/pop3"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// opener builds protocol sessions from the endpoints of a plan, reusing the
// gomailtest protocol packages and their ConfigFromViper.
type opener struct {
	plan *config.Plan
	log  *slog.Logger
}

func (o *opener) endpoint(name string) (*config.Endpoint, error) {
	ep, ok := o.plan.Endpoints[name]
	if !ok {
		return nil, fmt.Errorf("endpoint %q is not defined", name)
	}
	return ep, nil
}

func smtpConfig(ep *config.Endpoint) *smtp.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	cfg := smtp.ConfigFromViper(v)
	cfg.VerboseMode = false
	return cfg
}

func msgraphConfig(ep *config.Endpoint) *msgraph.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	cfg := msgraph.ConfigFromViper(v)
	cfg.VerboseMode = false
	return cfg
}

func imapConfig(ep *config.Endpoint) *imap.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	cfg := imap.ConfigFromViper(v)
	if cfg.IMAPS && !v.IsSet("port") {
		cfg.Port = 993
	}
	cfg.VerboseMode = false
	return cfg
}

func pop3Config(ep *config.Endpoint) *pop3.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	cfg := pop3.ConfigFromViper(v)
	if cfg.POP3S && !v.IsSet("port") {
		cfg.Port = 995
	}
	cfg.VerboseMode = false
	return cfg
}

func gmailConfig(ep *config.Endpoint) *gmail.Config {
	v := ep.Viper()
	v.SetDefault("maxretries", 3)
	cfg := gmail.ConfigFromViper(v)
	cfg.VerboseMode = false
	return cfg
}

// Sender implements engine.Opener.
func (o *opener) Sender(ctx context.Context, name string) (send.Sender, error) {
	ep, err := o.endpoint(name)
	if err != nil {
		return nil, err
	}
	switch ep.Protocol {
	case "smtp":
		return smtp.OpenSender(smtpConfig(ep), o.log)
	case "msgraph":
		return msgraph.OpenSender(ctx, msgraphConfig(ep), o.log)
	}
	return nil, fmt.Errorf("endpoint %q: protocol %s cannot send", name, ep.Protocol)
}

// Fetcher implements engine.Opener.
func (o *opener) Fetcher(ctx context.Context, name string, withDelete bool) (fetch.Session, error) {
	ep, err := o.endpoint(name)
	if err != nil {
		return nil, err
	}
	switch ep.Protocol {
	case "msgraph":
		return msgraph.OpenFetchSession(ctx, msgraphConfig(ep), o.log)
	case "gmail":
		return gmail.OpenFetchSession(ctx, gmailConfig(ep), o.log, withDelete)
	case "imap":
		return imap.OpenFetchSession(ctx, imapConfig(ep), o.log)
	case "pop3":
		return pop3.OpenFetchSession(ctx, pop3Config(ep), o.log)
	}
	return nil, fmt.Errorf("endpoint %q: protocol %s cannot be checked", name, ep.Protocol)
}

// checkEndpoints validates the settings of every endpoint used by a flow
// before any network work.
func checkEndpoints(plan *config.Plan) error {
	var errs []error
	for _, name := range plan.EndpointNames() {
		if err := checkEndpoint(plan.Endpoints[name]); err != nil {
			errs = append(errs, fmt.Errorf("endpoint %s (%s): %w", name, plan.Endpoints[name].Protocol, err))
		}
	}
	return errors.Join(errs...)
}

func checkEndpoint(ep *config.Endpoint) error {
	hostAuth := func(host, user, pass, token string, port int) error {
		if host == "" {
			return errors.New("'host' is required")
		}
		if err := validation.ValidateHostname(host); err != nil {
			return fmt.Errorf("invalid host: %w", err)
		}
		if err := validation.ValidatePort(port); err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		if user == "" {
			return errors.New("'username' is required")
		}
		if pass == "" && token == "" {
			return errors.New("'password' or 'accesstoken' is required")
		}
		return nil
	}
	switch ep.Protocol {
	case "smtp":
		c := smtpConfig(ep)
		if c.Host == "" && !c.UseMX {
			return errors.New("'host' is required (or 'use-mx: true' to deliver to the recipient's MX)")
		}
		if c.Host != "" {
			if err := validation.ValidateHostname(c.Host); err != nil {
				return fmt.Errorf("invalid host: %w", err)
			}
		}
		if err := validation.ValidatePort(c.Port); err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		if c.SMTPS && c.StartTLS {
			return errors.New("'smtps' and 'starttls' are mutually exclusive")
		}
		if c.StartTLS && c.NoStartTLS {
			return errors.New("'starttls' and 'no-starttls' are mutually exclusive")
		}
	case "msgraph":
		c := msgraphConfig(ep)
		if c.Mailbox == "" {
			return errors.New("'mailbox' is required")
		}
		if err := validation.ValidateEmail(c.Mailbox); err != nil {
			return fmt.Errorf("invalid mailbox: %w", err)
		}
		if c.BearerToken != "" {
			return nil
		}
		if err := validation.ValidateGUID(c.TenantID, "tenantid"); err != nil {
			return err
		}
		if err := validation.ValidateGUID(c.ClientID, "clientid"); err != nil {
			return err
		}
		if c.Delegated {
			return errors.New("delegated (interactive) authentication cannot be used by an unattended service")
		}
		if c.Secret == "" && c.PfxPath == "" && c.Thumbprint == "" {
			return errors.New("one of 'secret', 'pfx' or 'thumbprint' is required (or 'bearertoken')")
		}
	case "imap":
		c := imapConfig(ep)
		return hostAuth(c.Host, c.Username, c.Password, c.AccessToken, c.Port)
	case "pop3":
		c := pop3Config(ep)
		return hostAuth(c.Host, c.Username, c.Password, c.AccessToken, c.Port)
	case "gmail":
		c := gmailConfig(ep)
		if c.OAuth {
			return errors.New("interactive OAuth cannot be used by an unattended service; use a service account ('credentials') or 'bearertoken'")
		}
		if c.CredentialsFile == "" && c.BearerToken == "" {
			return errors.New("'credentials' (service account file) or 'bearertoken' is required")
		}
	}
	return nil
}
