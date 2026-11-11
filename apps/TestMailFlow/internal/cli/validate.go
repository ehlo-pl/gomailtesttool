package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

func newValidateCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check the config file and print the resolved endpoints and flows",
		Long: `Parse and validate the config file without sending anything, and print the plan
with secrets masked. With --connect, also open every checked mailbox (and authenticate
Microsoft Graph senders) to verify credentials and permissions.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v, setupOpts{})
			if err != nil {
				return err
			}
			defer e.Close()
			plan, err := loadPlan(e.cfg)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Config:    %s\nData dir:  %s\nDatabase:  %s\nHTTP:      %s\nRetention: %s\n\n",
				e.cfg.ConfigFile, e.cfg.DataDir, e.cfg.DBPath, orDash(e.cfg.HTTPAddr), e.cfg.Retention); err != nil {
				return err
			}

			if _, err := fmt.Fprintln(out, "Endpoints:"); err != nil {
				return err
			}
			for _, name := range plan.EndpointNames() {
				ep := plan.Endpoints[name]
				if _, err := fmt.Fprintf(out, "  %-16s %-8s %s\n", name, ep.Protocol, strings.Join(ep.MaskedSettings(), " ")); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out, "\nFlows:"); err != nil {
				return err
			}
			for _, f := range plan.Flows {
				to := make([]string, len(f.To))
				for i, a := range f.To {
					to[i] = security.MaskEmail(a)
				}
				if _, err := fmt.Fprintf(out, "  %s\n", f.Name); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "    send   via %s  from %s  to %s\n", f.SendEndpoint, security.MaskEmail(f.From), strings.Join(to, ", ")); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "    check  via %s  folder %s  delete=%t\n", f.CheckEndpoint, orDash(f.Folder), f.Delete); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "    every %s; checks after %s; late after %s; lost after %s\n",
					f.Interval, offsets(f), f.WarnAfter, f.Deadline()); err != nil {
					return err
				}
			}
			if !v.GetBool("connect") {
				if _, err := fmt.Fprintln(out, "\nConfiguration is valid. (Use --connect to test the connections.)"); err != nil {
					return err
				}
				return nil
			}
			if _, err := fmt.Fprintln(out, "\nConnections:"); err != nil {
				return err
			}
			op := &opener{plan: plan, log: e.log}
			failed := 0
			checked := map[string]bool{}
			for _, f := range plan.Flows {
				if !checked["c:"+f.CheckEndpoint] {
					checked["c:"+f.CheckEndpoint] = true
					sess, err := op.Fetcher(ctx, f.CheckEndpoint, f.Delete)
					if err == nil {
						err = sess.Close()
					}
					n, err := printConn(cmd, "check", f.CheckEndpoint, err)
					if err != nil {
						return err
					}
					failed += n
				}
				if !checked["s:"+f.SendEndpoint] {
					checked["s:"+f.SendEndpoint] = true
					if plan.Endpoints[f.SendEndpoint].Protocol == "smtp" {
						if _, err := fmt.Fprintf(out, "  send   %-16s skipped (SMTP connects when a probe is sent; use 'once')\n", f.SendEndpoint); err != nil {
							return err
						}
						continue
					}
					s, err := op.Sender(ctx, f.SendEndpoint)
					if err == nil {
						err = s.Close()
					}
					n, err := printConn(cmd, "send", f.SendEndpoint, err)
					if err != nil {
						return err
					}
					failed += n
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d endpoint(s) failed the connection test", failed)
			}
			return nil
		},
	}
	cmd.Flags().Bool("connect", false, "Also open the mailbox sessions to verify credentials")
	return cmd
}

func printConn(cmd *cobra.Command, role, name string, err error) (int, error) {
	if err != nil {
		_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "  %-6s %-16s FAILED: %v\n", role, name, err)
		return 1, writeErr
	}
	_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "  %-6s %-16s ok\n", role, name)
	return 0, writeErr
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// offsets lists the cumulative check times of a flow, e.g. "5m0s, 7m30s, ...".
func offsets(f *config.Flow) string {
	parts := make([]string, len(f.Checks))
	for i := range f.Checks {
		parts[i] = f.CheckOffset(i).Round(time.Second).String()
	}
	return strings.Join(parts, ", ")
}
