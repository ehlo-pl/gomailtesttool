package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/inspect"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/endpoint"
)

// printer collects the first write error so commands can print freely.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) f(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func newRunCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the warm-up of all enabled domains until stopped",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if g.domain != "" {
				return errors.New("run handles all domains; disable one with 'enabled: false'")
			}
			e, err := setup(cmd.Context(), g, needSecrets)
			if err != nil {
				return err
			}
			defer e.Close()
			for _, d := range e.cfg.Domains {
				e.log.Info("domain loaded", "domain", d.Name, "enabled", d.Enabled, "targets", len(d.Targets),
					"start", d.Start.Format("2006-01-02 15:04"), "peak_per_day", d.Schedule.Peak())
			}
			err = e.engine().Run(cmd.Context())
			e.log.Info("stopped")
			return err
		},
	}
}

func newOnceCmd(g *globals) *cobra.Command {
	var target string
	var wait bool
	cmd := &cobra.Command{
		Use:   "once",
		Short: "Send one message to a target now and follow it",
		Long: `Send one real warm-up message now, outside the schedule, and wait for it.
Prints the placement, the authentication results and a reverse DNS check of the
IP address the receiver saw. Exit 0 inbox in time, 1 junk or late, 2 lost or failed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			e, err := setup(ctx, g, needSecrets)
			if err != nil {
				return err
			}
			defer e.Close()
			domains, err := e.domains(g.domain)
			if err != nil {
				return err
			}
			if len(domains) != 1 {
				return errors.New("several domains are configured: use --domain")
			}
			d := domains[0]
			t, ok := d.Target(strings.ToLower(target))
			if !ok {
				return fmt.Errorf("--target %q is not a target of %s", target, d.Name)
			}
			eng := e.engine()
			m, err := eng.Send(ctx, d, t)
			if err != nil {
				return err
			}
			p := &printer{w: cmd.OutOrStdout()}
			p.f("Sent %q to %s\n  Message-ID: %s\n  Server:     %s\n", m.Subject, m.Recipient, m.MessageID, oneLine(m.SendResponse, 200))
			if !wait {
				return p.err
			}
			p.f("Waiting for the message (first look after 5 minutes)...\n")
			if m, err = eng.Follow(ctx, m.ID); err != nil {
				return err
			}
			p.f("  State:      %s\n", m.State)
			if m.CheckError != "" {
				p.f("  Check:      %s\n", oneLine(m.CheckError, 300))
			}
			if m.Placement != "" {
				p.f("  Placement:  %s (%s)\n  Latency:    %s\n  Auth:       %s\n", m.Placement, m.Folder, m.Latency, m.Auth)
				if m.SpamInfo != "" {
					p.f("  Spam info:  %s\n", oneLine(m.SpamInfo, 400))
				}
				h := inspect.ParseHeaders([]byte(m.Headers))
				if h.ReturnPath != "" && !strings.EqualFold(h.ReturnPath, d.EnvelopeFrom()) {
					p.f("  WARNING:    the envelope sender arrived as %s, not %s: your mail server rewrites it,\n"+
						"              so bounces will not reach the bounce mailbox\n", oneLine(h.ReturnPath, 200), d.EnvelopeFrom())
				}
				describeSenderIP(ctx, p, h.SenderIP)
			}
			switch {
			case p.err != nil:
				return p.err
			case m.State == store.StateDelivered && m.Placement == "inbox":
				return nil
			case m.State == store.StateDelivered || m.State == store.StateLate:
				return &exitError{code: 1}
			}
			return &exitError{code: 2}
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "target endpoint name (required)")
	cmd.Flags().BoolVar(&wait, "wait", true, "follow the message until it arrives or is lost")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// describeSenderIP checks forward-confirmed reverse DNS of the sending MTA.
func describeSenderIP(ctx context.Context, p *printer, ip string) {
	if ip == "" {
		p.f("  Sender IP:  not named in the received headers\n")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		p.f("  Sender IP:  %s  WARNING: no PTR record\n", ip)
		return
	}
	for _, name := range names {
		addrs, err := net.DefaultResolver.LookupHost(ctx, name)
		if err == nil && slices.Contains(addrs, ip) {
			p.f("  Sender IP:  %s  PTR %s (forward-confirmed)\n", ip, strings.TrimSuffix(name, "."))
			return
		}
	}
	p.f("  Sender IP:  %s  WARNING: PTR %s does not resolve back to it\n", ip, strings.Join(names, ", "))
}

func newPlanCmd(g *globals) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show the projected volume per day and the load per mailbox",
		Long: `Project the coming days from the stored ramp state, assuming that the service
runs every day and nothing is held. The number of messages outside business
hours varies between its minimum and maximum.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if days < 1 || days > 366 {
				return errors.New("--days must be between 1 and 366")
			}
			e, err := setup(cmd.Context(), g, noSecrets)
			if err != nil {
				return err
			}
			defer e.Close()
			domains, err := e.domains(g.domain)
			if err != nil {
				return err
			}
			p := &printer{w: cmd.OutOrStdout()}
			load := map[string]map[string]int{} // mailbox -> date -> messages
			rng := rand.New(rand.NewPCG(1, 1))
			for _, d := range domains {
				r, err := e.store.DomainRamp(cmd.Context(), d.Name)
				if err != nil {
					return err
				}
				day := time.Now().In(d.Location)
				if day.Before(d.Start) {
					day = d.Start
				}
				p.f("\n%s  (peak %d per day, %d targets, enabled: %t)\n", d.Name, d.Schedule.Peak(), len(d.Targets), d.Enabled)
				w := tabwriter.NewWriter(p.w, 0, 4, 2, ' ', 0)
				table := &printer{w: w}
				table.f("DATE\tDAY\tRAMP\tBUSINESS\tOTHER\tTOTAL\n")
				ramp := r.Day
				for i := range days {
					date := day.AddDate(0, 0, i)
					key := date.Format("2006-01-02")
					if !d.End.IsZero() && key > d.End.Format("2006-01-02") {
						break
					}
					if key != r.LastAdvanced {
						ramp++
					}
					business, other := d.Schedule.Quota(ramp, date, rng)
					table.f("%s\t%s\t%d\t%d\t%d\t%d\n", key, date.Format("Mon"), ramp, business, other, business+other)
					for _, t := range d.Targets {
						if load[t.Address] == nil {
							load[t.Address] = map[string]int{}
						}
						share := (d.Schedule.Total(ramp) + len(d.Targets) - 1) / len(d.Targets)
						load[t.Address][key] += min(share, business+other)
					}
				}
				if err := errors.Join(table.err, w.Flush()); err != nil {
					return err
				}
			}
			p.f("\nHighest load per mailbox and day (all listed domains, upper bound):\n")
			addresses := make([]string, 0, len(load))
			for a := range load {
				addresses = append(addresses, a)
			}
			slices.Sort(addresses)
			for _, a := range addresses {
				highest := 0
				for _, n := range load[a] {
					highest = max(highest, n)
				}
				p.f("  %-40s %d\n", a, highest)
			}
			return p.err
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "number of days to project")
	return cmd
}

func newStatusCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show ramp days, holds and recent placement. Exit 1 target held, 2 domain held",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			e, err := setup(ctx, g, noSecrets)
			if err != nil {
				return err
			}
			defer e.Close()
			domains, err := e.domains(g.domain)
			if err != nil {
				return err
			}
			p := &printer{w: cmd.OutOrStdout()}
			code := 0
			for _, d := range domains {
				r, err := e.store.DomainRamp(ctx, d.Name)
				if err != nil {
					return err
				}
				today := time.Now().In(d.Location).Format("2006-01-02")
				slots, err := e.store.DaySlots(ctx, d.Name, today)
				if err != nil {
					return err
				}
				count := map[string]int{}
				for _, s := range slots {
					count[s.Status]++
				}
				p.f("%s  ramp day %d, today: %d sent, %d pending, %d missed, %d failed, %d skipped\n", d.Name, r.Day,
					count[store.SlotSent], count[store.SlotPending], count[store.SlotMissed], count[store.SlotFailed], count[store.SlotSkipped])
				if r.HeldReason != "" {
					code = 2
					p.f("  HELD (domain): %s\n", oneLine(r.HeldReason, 300))
				}
				for _, t := range d.Targets {
					tr, err := e.store.TargetRamp(ctx, d.Name, t.Endpoint)
					if err != nil {
						return err
					}
					last, err := e.store.Resolved(ctx, d.Name, t.Endpoint, time.Time{}, d.Hold.Window)
					if err != nil {
						return err
					}
					inbox, junk, lost := 0, 0, 0
					for _, m := range last {
						switch {
						case m.State == store.StateLost:
							lost++
						case m.Placement == "junk":
							junk++
						default:
							inbox++
						}
					}
					p.f("  %-16s ramp day %-3d last %d: %d inbox, %d junk, %d lost", t.Endpoint, tr.Day, len(last), inbox, junk, lost)
					if tr.HeldReason != "" {
						code = max(code, 1)
						p.f("  HELD: %s", oneLine(tr.HeldReason, 300))
					}
					p.f("\n")
				}
			}
			if p.err != nil {
				return p.err
			}
			if code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
}

func newReportCmd(g *globals) *cobra.Command {
	var last, format string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Placement, bounces and authentication by domain, day and mailbox",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := endpoint.ParseDuration(last)
			if err != nil || d <= 0 {
				return fmt.Errorf("--last: invalid duration %q", last)
			}
			e, err := setup(cmd.Context(), g, noSecrets)
			if err != nil {
				return err
			}
			defer e.Close()
			if _, err := e.domains(g.domain); err != nil {
				return err
			}
			data, err := buildReport(cmd.Context(), e.store, strings.ToLower(g.domain), time.Now().Add(-d))
			if err != nil {
				return err
			}
			return renderReport(cmd.OutOrStdout(), data, format)
		},
	}
	cmd.Flags().StringVar(&last, "last", "7d", "period, e.g. 24h, 7d, 4w")
	cmd.Flags().StringVar(&format, "format", "text", "text, csv or json")
	return cmd
}

func newResumeCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "resume [target]",
		Short: "Clear the hold of one target, or all holds",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := setup(cmd.Context(), g, noSecrets)
			if err != nil {
				return err
			}
			defer e.Close()
			domains, err := e.domains(g.domain)
			if err != nil {
				return err
			}
			target := ""
			if len(args) == 1 {
				target = strings.ToLower(args[0])
				if !slices.ContainsFunc(domains, func(d *config.Domain) bool { _, ok := d.Target(target); return ok }) {
					return fmt.Errorf("%q is not a target", target)
				}
			}
			for _, d := range domains {
				if err := e.engine().Resume(cmd.Context(), d, target); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: resumed %s\n", d.Name, orAll(target)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func orAll(target string) string {
	if target == "" {
		return "everything"
	}
	return target
}

func newAuthCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "auth <endpoint>",
		Short: "Sign in once to an Outlook.com mailbox (oauth: microsoft)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadConfig(g, needSecrets)
			if err != nil {
				return err
			}
			ep, err := e.open.endpoint(strings.ToLower(args[0]))
			if err != nil {
				return err
			}
			if !strings.EqualFold(ep.String("oauth"), config.OAuthMicrosoft) {
				return fmt.Errorf("endpoint %q does not have 'oauth: microsoft'", ep.Name)
			}
			a, err := e.open.account(ep)
			if err != nil {
				return err
			}
			if err := a.Login(cmd.Context(), cmd.OutOrStdout()); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Signed in; token stored in %s\n"+
				"The service account must be able to read and write this file.\n", a.TokenFile)
			return err
		},
	}
}
