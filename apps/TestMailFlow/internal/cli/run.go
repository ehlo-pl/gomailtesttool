package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/engine"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/eventlog"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/httpd"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/svc"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

func newRunCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run continuously: send probes at each flow's interval and check their arrival",
		Long: `Run all flows of the config file until interrupted (Ctrl+C / SIGTERM).
This is also the command the Windows service executes; under the service control
manager it logs to <datadir>/logs/testmailflow-YYYY-MM-DD.log instead of stderr.

Probes that were still pending when the previous run stopped are picked up again.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if svc.IsService() {
				return svc.Run(config.AppName, func(ctx context.Context) error {
					return runDaemon(ctx, cmd, v, true)
				})
			}
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			return runDaemon(ctx, cmd, v, false)
		},
	}
	cmd.Flags().String(config.KeyHTTP, "", "Listen address for /healthz and /metrics, e.g. 127.0.0.1:9925 (env: TMFHTTP; empty = disabled)")
	cmd.Flags().String(config.KeyRetention, "", "Keep finished probes this long, e.g. 90d (env: TMFRETENTION; default 90d)")
	return cmd
}

func runDaemon(ctx context.Context, cmd *cobra.Command, v *viper.Viper, asService bool) (err error) {
	events, evErr := eventlog.Open(config.AppName)
	defer func() { _ = events.Close() }()
	// A service has no console: make start-up failures visible in the event log.
	defer func() {
		if err != nil && asService {
			events.Report(eventlog.Error, eventlog.IDStopped, "testmailflow stopped with an error: "+err.Error())
		}
	}()

	e, err := setup(ctx, cmd, v, setupOpts{logToFile: asService})
	if err != nil {
		return err
	}
	defer e.Close()
	if evErr != nil {
		e.log.Warn("event log unavailable", "error", evErr)
	}
	plan, err := loadPlan(e.cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eng := engine.New(plan, e.store, &opener{plan: plan, log: e.log}, e.log, events)
	eng.Retention = e.cfg.Retention

	httpErr := make(chan error, 1)
	if e.cfg.HTTPAddr != "" {
		srv := &httpd.Server{Store: e.store, Plan: plan, Log: e.log}
		go func() {
			if err := srv.Serve(ctx, e.cfg.HTTPAddr); err != nil {
				httpErr <- err
				cancel()
			}
		}()
	}

	names := make([]string, 0, len(plan.Flows))
	for _, f := range plan.Flows {
		names = append(names, f.Name)
	}
	started := fmt.Sprintf("testmailflow %s started with %d flow(s): %s", version.Get(), len(plan.Flows), strings.Join(names, ", "))
	e.log.Info(started, "config", e.cfg.ConfigFile, "db", e.cfg.DBPath, "http", e.cfg.HTTPAddr)
	events.Report(eventlog.Info, eventlog.IDStarted, started)

	runErr := eng.Run(ctx)
	select {
	case herr := <-httpErr:
		runErr = errors.Join(runErr, herr)
	default:
	}
	if runErr != nil {
		e.log.Error("testmailflow stopped", "error", runErr)
		return runErr
	}
	e.log.Info("testmailflow stopped")
	events.Report(eventlog.Info, eventlog.IDStopped, "testmailflow stopped")
	return nil
}

func newOnceCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "once",
		Short: "Send one probe per flow now and follow its check schedule",
		Long: `Send a single probe for every flow (or the flows named with --flow) and, with
--wait (default), check for its arrival on the flow's schedule until it is found or lost.

Exit code: 0 all probes arrived in time, 1 at least one arrived late, 2 at least one
was lost or could not be sent.`,
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
			flows := plan.Flows
			if want := v.GetStringSlice("flow"); len(want) > 0 {
				flows = nil
				for _, name := range want {
					f := plan.Flow(name)
					if f == nil {
						return fmt.Errorf("flow %q is not defined in %s", name, e.cfg.ConfigFile)
					}
					flows = append(flows, f)
				}
			}

			eng := engine.New(plan, e.store, &opener{plan: plan, log: e.log}, e.log, eventlog.Nop{})
			defer eng.Close()
			out := cmd.OutOrStdout()
			var ids []int64
			worst := 0
			for _, f := range flows {
				p, err := eng.SendProbe(ctx, f)
				if err != nil {
					if _, writeErr := fmt.Fprintf(out, "%-24s SEND FAILED  %v\n", f.Name, err); writeErr != nil {
						return writeErr
					}
					worst = 2
					continue
				}
				if _, err := fmt.Fprintf(out, "%-24s sent         first check at %s\n", f.Name, p.NextCheckAt.Local().Format("15:04:05")); err != nil {
					return err
				}
				ids = append(ids, p.ID)
			}
			if !v.GetBool("wait") || len(ids) == 0 {
				return exitCode(worst)
			}
			if _, err := fmt.Fprintln(out, "Waiting for the probes to arrive (Ctrl+C to stop; pending probes stay in the database)..."); err != nil {
				return err
			}
			probes, err := eng.Follow(ctx, ids)
			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			for _, p := range probes {
				switch p.State {
				case store.StateDelivered:
					if _, err := fmt.Fprintf(out, "%-24s DELIVERED    in %s  hops=%d  %s\n", p.Flow, p.Latency.Round(time.Second), p.Hops, p.AuthResults); err != nil {
						return err
					}
				case store.StateLate:
					if _, err := fmt.Fprintf(out, "%-24s LATE         in %s  hops=%d  %s\n", p.Flow, p.Latency.Round(time.Second), p.Hops, p.AuthResults); err != nil {
						return err
					}
					worst = max(worst, 1)
				case store.StateLost:
					if _, err := fmt.Fprintf(out, "%-24s LOST         not found after %d checks\n", p.Flow, p.Attempts); err != nil {
						return err
					}
					worst = 2
				default:
					if _, err := fmt.Fprintf(out, "%-24s %s  %s\n", p.Flow, strings.ToUpper(p.State), p.CheckError); err != nil {
						return err
					}
					worst = max(worst, 1)
				}
			}
			return exitCode(worst)
		},
	}
	cmd.Flags().StringSlice("flow", nil, "Flow name to probe (repeatable; default: all flows)")
	cmd.Flags().Bool("wait", true, "Follow the check schedule until every probe is found or lost")
	return cmd
}

func exitCode(code int) error {
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}
