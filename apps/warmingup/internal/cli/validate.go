package cli

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
)

func newValidateCmd(g *globals) *cobra.Command {
	var connect bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check the configuration, the DNS records and optionally the mailboxes",
		Long: `Check the configuration files and the SPF, DKIM and DMARC records of every
domain. With --connect, also sign in to every target and bounce mailbox and show
the inbox and junk folders used. The sending server is tested by 'once'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			e, err := loadConfig(g, secrets(connect))
			if err != nil {
				return err
			}
			domains, err := e.domains(g.domain)
			if err != nil {
				return err
			}
			p := &printer{w: cmd.OutOrStdout()}
			var failed []error
			checked := map[string]bool{}
			for _, d := range domains {
				p.f("%s (%s, enabled: %t)\n", d.Name, d.File, d.Enabled)
				p.f("  from %s, envelope %s, peak %d per day, start %s\n", d.From, d.EnvelopeFrom(), d.Schedule.Peak(),
					d.Start.Format("2006-01-02 15:04 MST"))
				if d.BounceEndpoint == "" {
					p.f("  WARNING: no bounce mailbox; provider deferrals and rejections will go unnoticed\n")
				}
				checkDNS(ctx, p, d)
				if !connect {
					continue
				}
				for _, t := range d.Targets {
					if checked[t.Endpoint] {
						continue
					}
					checked[t.Endpoint] = true
					sess, err := e.open.Mailbox(ctx, t.Endpoint)
					if err != nil {
						failed = append(failed, err)
						p.f("  target %-14s FAILED: %s\n", t.Endpoint, oneLine(err.Error(), 600))
						continue
					}
					inbox, junk := sess.Folders()
					p.f("  target %-14s ok: %s, inbox %q, junk %q\n", t.Endpoint, t.Address, inbox, junk)
					_ = sess.Close()
				}
				if name := d.BounceEndpoint; name != "" && !checked[name] {
					checked[name] = true
					sess, err := e.open.Fetcher(ctx, name)
					if err != nil {
						failed = append(failed, err)
						p.f("  bounce %-14s FAILED: %s\n", name, oneLine(err.Error(), 600))
						continue
					}
					p.f("  bounce %-14s ok\n", name)
					_ = sess.Close()
				}
			}
			if len(failed) > 0 {
				return errors.Join(p.err, errors.New("some mailboxes cannot be used"))
			}
			p.f("Configuration is valid.\n")
			return p.err
		},
	}
	cmd.Flags().BoolVar(&connect, "connect", false, "sign in to the target and bounce mailboxes")
	return cmd
}

// checkDNS reports the sender authentication records. Problems are warnings:
// split DNS or an offline host must not block the other checks.
func checkDNS(ctx context.Context, p *printer, d *config.Domain) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	txt := func(label, name, prefix string) {
		records, err := net.DefaultResolver.LookupTXT(ctx, name)
		for _, r := range records {
			if strings.HasPrefix(strings.ToLower(r), strings.ToLower(prefix)) {
				p.f("  %-6s %s\n", label, oneLine(r, 100))
				return
			}
		}
		if err != nil {
			p.f("  %-6s WARNING: %s: %v\n", label, name, err)
			return
		}
		p.f("  %-6s WARNING: no %s record at %s\n", label, label, name)
	}
	_, envelope, _ := strings.Cut(d.EnvelopeFrom(), "@")
	_, from, _ := strings.Cut(d.FromAddress, "@")
	txt("SPF", envelope, "v=spf1")
	txt("DMARC", "_dmarc."+from, "v=DMARC1")
	if !strings.EqualFold(envelope, from) {
		records, _ := net.DefaultResolver.LookupTXT(ctx, "_dmarc."+from)
		if strings.Contains(strings.ReplaceAll(strings.ToLower(strings.Join(records, "")), " ", ""), "aspf=s") {
			p.f("  DMARC  WARNING: aspf=s requires the envelope domain %s to equal %s; SPF will not count for DMARC\n", envelope, from)
		}
	}
	if d.DKIMSelector == "" {
		p.f("  DKIM   not checked: set dkim_selector\n")
		return
	}
	// "v=" is optional in DKIM records; "p=" is not.
	name := d.DKIMSelector + "._domainkey." + from
	records, err := net.DefaultResolver.LookupTXT(ctx, name)
	if err == nil && strings.Contains(strings.Join(records, ""), "p=") {
		p.f("  DKIM   key published at %s\n", name)
		return
	}
	p.f("  DKIM   WARNING: no key at %s %v\n", name, err)
}
