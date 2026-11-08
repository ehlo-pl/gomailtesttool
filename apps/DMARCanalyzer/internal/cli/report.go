package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/report"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
)

func newReportCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Summarize stored DMARC reports over periods",
		Long: `Aggregate stored DMARC records and print a summary.

Records are attributed to periods by the report's date_range begin. Sections:
results by period (with trend), by header From domain (with published policy),
by reporter, top sending sources, actionable DMARC failures, override reasons and
published policy changes.

Examples:
  dmarcanalyzer report --last 30d
  dmarcanalyzer report --period week --from 2026-09-01 --to 2026-11-01 --domain example.com
  dmarcanalyzer report --format html --out dmarc.html --resolve
  dmarcanalyzer report --format csv --section failing --out failing.csv
  dmarcanalyzer report --format csv --section all --out ./csv/`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v)
			if err != nil {
				return err
			}
			defer e.Close()
			ro, err := reportOptionsFromViper(v, time.Now())
			if err != nil {
				return err
			}
			return runReport(ctx, e, ro, cmd.OutOrStdout())
		},
	}
	addReportFlags(cmd.Flags())
	return cmd
}

// reportOpts are the parsed report flags.
type reportOpts struct {
	filter  store.RecordFilter
	opts    report.Options
	format  report.Format
	section string
	out     string
	resolve bool
}

func addReportFlags(f *pflag.FlagSet) {
	f.String("from", "", "Start (report begin >=): YYYY-MM-DD, RFC 3339 or relative (30d)")
	f.String("to", "", "End (report begin <), exclusive: YYYY-MM-DD or RFC 3339")
	f.String("last", "", "Shortcut for --from <now - value>, e.g. 7d, 4w, 90d")
	f.String("period", "month", "Bucket size: day, week, month, quarter, year, total")
	f.StringSlice("domain", nil, "Only these domains (policy domain or header From, includes subdomains; repeatable)")
	f.StringSlice("org", nil, "Only reports from these reporter organizations (repeatable)")
	f.Int("top", 20, "Rows in the source and failure lists")
	f.Bool("failing-only", false, "Source list shows only sources with DMARC failures")
	f.Bool("resolve", false, "Reverse-DNS (PTR) lookup of listed source IPs (cached 7 days)")
	f.String("format", "text", "Output format: text, md, csv, json, html")
	f.String("section", "", "CSV section: "+strings.Join(report.CSVSections, ", ")+" or all (default periods)")
	f.String("out", "", "Write to file (or directory for --format csv --section all) instead of stdout")
	f.String("tz", "UTC", "Time zone for period boundaries (IANA name, e.g. Europe/Warsaw)")
}

// reportOptionsFromViper reads report settings (flags bound into v, with the
// config file "dmarc:" section as defaults).
func reportOptionsFromViper(v *viper.Viper, now time.Time) (*reportOpts, error) {
	get := func(n string) string { return strings.TrimSpace(v.GetString(n)) }
	loc, err := time.LoadLocation(get("tz"))
	if err != nil {
		return nil, fmt.Errorf("--tz: %w", err)
	}
	period, err := report.ParsePeriod(get("period"))
	if err != nil {
		return nil, err
	}
	format, err := report.ParseFormat(get("format"))
	if err != nil {
		return nil, err
	}
	if get("from") != "" && get("last") != "" {
		return nil, errors.New("use either --from or --last, not both")
	}
	from, err := report.ParseSince(get("from")+get("last"), now, loc)
	if err != nil {
		return nil, err
	}
	var to time.Time
	if s := get("to"); s != "" {
		if to, err = report.ParseDate(s, loc); err != nil {
			return nil, err
		}
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return nil, errors.New("--to must be after --from")
	}
	top := v.GetInt("top")
	if top < 1 || top > 10000 {
		return nil, errors.New("--top must be between 1 and 10000")
	}
	domains := v.GetStringSlice("domain")
	orgs := v.GetStringSlice("org")
	failingOnly := v.GetBool("failing-only")
	resolve := v.GetBool("resolve")
	section := get("section")
	if section != "" && format != report.FormatCSV {
		return nil, errors.New("--section applies to --format csv only")
	}
	if section == "all" && get("out") == "" {
		return nil, errors.New("--section all needs --out <directory>")
	}
	return &reportOpts{
		filter:  store.RecordFilter{From: from, To: to, Domains: domains, Orgs: orgs},
		opts:    report.Options{From: from, To: to, Period: period, Location: loc, Top: top, FailingOnly: failingOnly},
		format:  format,
		section: section,
		out:     get("out"),
		resolve: resolve,
	}, nil
}

func runReport(ctx context.Context, e *env, ro *reportOpts, stdout io.Writer) error {
	rows, err := e.store.Records(ctx, ro.filter)
	if err != nil {
		return err
	}
	e.log.Debug("records loaded", "count", len(rows))
	s := report.Build(ctx, rows, ro.opts)
	if ro.resolve {
		s.ApplyPTR(resolvePTRs(ctx, e.store, s.IPs()))
	}

	if ro.format == report.FormatCSV && ro.section == "all" {
		files, err := report.WriteCSVDir(ro.out, s)
		for _, f := range files {
			if _, writeErr := fmt.Fprintln(stdout, "Wrote", f); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	if ro.out == "" {
		return report.Render(stdout, s, ro.format, ro.section)
	}
	f, err := os.OpenFile(ro.out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := report.Render(f, s, ro.format, ro.section); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "Wrote", ro.out)
	return err
}

// resolvePTRs looks up PTR names concurrently, using and filling the cache.
func resolvePTRs(ctx context.Context, st *store.Store, ips []string) map[string]string {
	const ttl = 7 * 24 * time.Hour
	out := make(map[string]string, len(ips))
	var todo []string
	for _, ip := range ips {
		if ptr, ok, err := st.PTR(ctx, ip, ttl); err == nil && ok {
			out[ip] = ptr
		} else {
			todo = append(todo, ip)
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for _, ip := range todo {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			names, err := net.DefaultResolver.LookupAddr(lctx, ip)
			ptr := ""
			if err == nil && len(names) > 0 {
				ptr = strings.TrimSuffix(names[0], ".")
			}
			mu.Lock()
			out[ip] = ptr
			mu.Unlock()
		}(ip)
	}
	wg.Wait()
	for _, ip := range todo { // cache sequentially (single DB connection)
		_ = st.SetPTR(ctx, ip, out[ip])
	}
	return out
}
