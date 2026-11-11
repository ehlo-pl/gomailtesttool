package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/report"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
)

func formatFlag(v *viper.Viper) (string, error) {
	f := strings.ToLower(strings.TrimSpace(v.GetString("format")))
	if !report.ValidFormat(f) {
		return "", fmt.Errorf("invalid --format %q (text, csv, json)", f)
	}
	return f, nil
}

// writeOut renders to --out (owner-only file) or to w.
func writeOut(w io.Writer, outPath string, render func(io.Writer) error) error {
	if outPath == "" {
		return render(w)
	}
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		return err
	}
	if err := export.WriteFile(outPath, buf.Bytes()); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}

func newStatusCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current health of every flow",
		Long: `Show, for each flow of the config file, the result of the latest finished probe,
its delivery latency, the last successful delivery and the number of pending probes.

Exit code (usable from monitoring agents): 0 all flows ok, 1 at least one degraded
or unknown, 2 at least one failing or stale (no recent probe: service not running).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v, setupOpts{})
			if err != nil {
				return err
			}
			defer e.Close()
			format, err := formatFlag(v)
			if err != nil {
				return err
			}
			plan, err := config.LoadPlan(e.cfg.ConfigFile)
			if err != nil {
				return err
			}
			statuses, err := report.Status(ctx, e.store, plan, time.Now())
			if err != nil {
				return err
			}
			if err := writeOut(cmd.OutOrStdout(), v.GetString("out"), func(w io.Writer) error {
				return report.RenderStatus(w, statuses, format)
			}); err != nil {
				return err
			}
			return exitCode(report.Worst(statuses))
		},
	}
	cmd.Flags().String("format", "text", "Output format: text, csv, json")
	cmd.Flags().String("out", "", "Write to this file instead of stdout")
	return cmd
}

func newReportCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Summarize probe results over a period",
		Long: `Per flow: probes sent, delivered, late, lost and failed to send, success rate and
delivery latency (min / avg / p95 / max), followed by the latest problems.

The period is --last (default 7d) or --from / --to (YYYY-MM-DD or RFC 3339; --to exclusive).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v, setupOpts{})
			if err != nil {
				return err
			}
			defer e.Close()
			format, err := formatFlag(v)
			if err != nil {
				return err
			}
			from, to, err := period(v, time.Now())
			if err != nil {
				return err
			}
			probes, err := e.store.ProbesBetween(ctx, from, to, strings.TrimSpace(v.GetString("flow")))
			if err != nil {
				return err
			}
			sum := report.Summarize(probes, v.GetInt("failures"))
			if !from.IsZero() {
				sum.From = &from
			}
			if !to.IsZero() {
				sum.To = &to
			}
			return writeOut(cmd.OutOrStdout(), v.GetString("out"), func(w io.Writer) error {
				return report.RenderSummary(w, sum, format)
			})
		},
	}
	f := cmd.Flags()
	f.String("last", "7d", "Report the last period, e.g. 24h, 7d, 4w")
	f.String("from", "", "Start of the period (YYYY-MM-DD or RFC 3339); overrides --last")
	f.String("to", "", "End of the period, exclusive (YYYY-MM-DD or RFC 3339)")
	f.String("flow", "", "Only this flow")
	f.Int("failures", 20, "Number of latest problem probes to list (text and json)")
	f.String("format", "text", "Output format: text, csv, json")
	f.String("out", "", "Write to this file instead of stdout")
	return cmd
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q (use YYYY-MM-DD or RFC 3339)", s)
}

func period(v *viper.Viper, now time.Time) (from, to time.Time, err error) {
	if s := strings.TrimSpace(v.GetString("to")); s != "" {
		if to, err = parseTime(s); err != nil {
			return from, to, fmt.Errorf("--to: %w", err)
		}
	}
	if s := strings.TrimSpace(v.GetString("from")); s != "" {
		if from, err = parseTime(s); err != nil {
			return from, to, fmt.Errorf("--from: %w", err)
		}
	} else if s := strings.TrimSpace(v.GetString("last")); s != "" {
		d, derr := config.ParseDuration(s)
		if derr != nil || d <= 0 {
			return from, to, fmt.Errorf("--last: invalid duration %q", s)
		}
		end := now
		if !to.IsZero() {
			end = to
		}
		from = end.Add(-d).UTC()
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return from, to, errors.New("--from must be before --to")
	}
	return from, to, nil
}

func newDBCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{Use: "db", Short: "Database maintenance"}
	run := func(fn func(ctx context.Context, e *env, cmd *cobra.Command) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v, setupOpts{})
			if err != nil {
				return err
			}
			defer e.Close()
			return fn(ctx, e, cmd)
		}
	}
	stats := &cobra.Command{
		Use: "stats", Short: "Show row counts and the latest events",
		RunE: run(func(ctx context.Context, e *env, cmd *cobra.Command) error {
			out := cmd.OutOrStdout()
			counts, err := e.store.TableCounts(ctx)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Database: %s\n", e.cfg.DBPath); err != nil {
				return err
			}
			tables := make([]string, 0, len(counts))
			for t := range counts {
				tables = append(tables, t)
			}
			sort.Strings(tables)
			for _, t := range tables {
				if _, err := fmt.Fprintf(out, "  %-10s %d\n", t, counts[t]); err != nil {
					return err
				}
			}
			events, err := e.store.RecentEvents(ctx, 10)
			if err != nil {
				return err
			}
			if len(events) > 0 {
				if _, err := fmt.Fprintln(out, "Latest events:"); err != nil {
					return err
				}
				for _, ev := range events {
					if _, err := fmt.Fprintf(out, "  %s  %-18s %-20s %s\n", ev.At.Format("2006-01-02 15:04:05Z"), ev.Kind, ev.Flow,
						strings.Join(strings.Fields(ev.Detail), " ")); err != nil {
						return err
					}
				}
			}
			return nil
		}),
	}
	vacuum := &cobra.Command{
		Use: "vacuum", Short: "Compact the database file",
		RunE: run(func(ctx context.Context, e *env, cmd *cobra.Command) error {
			if err := e.store.Vacuum(ctx); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Database compacted.")
			return err
		}),
	}
	prune := &cobra.Command{
		Use: "prune", Short: "Delete finished probes, seen keys and events older than --older-than",
		RunE: run(func(ctx context.Context, e *env, cmd *cobra.Command) error {
			d, err := config.ParseDuration(v.GetString("older-than"))
			if err != nil || d <= 0 {
				return fmt.Errorf("--older-than: invalid duration %q", v.GetString("older-than"))
			}
			n, err := e.store.Prune(ctx, time.Now().Add(-d))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted %d probes older than %s.\n", n, v.GetString("older-than"))
			return err
		}),
	}
	prune.Flags().String("older-than", "90d", "Age threshold, e.g. 30d")
	cmd.AddCommand(stats, vacuum, prune)
	return cmd
}
