package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Format is an output format.
type Format string

// Supported formats.
const (
	FormatText Format = "text"
	FormatMD   Format = "md"
	FormatCSV  Format = "csv"
	FormatJSON Format = "json"
	FormatHTML Format = "html"
)

// ParseFormat validates a format name.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case FormatText, FormatMD, FormatCSV, FormatJSON, FormatHTML:
		return f, nil
	case "markdown":
		return FormatMD, nil
	}
	return "", fmt.Errorf("invalid format %q (text, md, csv, json, html)", s)
}

// Render writes the summary in the given format. For CSV, section selects
// one table (see CSVSections); use WriteCSVDir to write all of them.
func Render(w io.Writer, s *Summary, f Format, section string) error {
	switch f {
	case FormatText:
		return renderText(w, s)
	case FormatMD:
		return renderMarkdown(w, s)
	case FormatCSV:
		return renderCSV(w, s, section)
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	case FormatHTML:
		return renderHTML(w, s)
	}
	return fmt.Errorf("unsupported format %q", f)
}

// ----------------------------------------------------------------- tables

// table is a format-neutral representation of one report section.
type table struct {
	Key, Title string
	Header     []string
	Rows       [][]string
}

func metricCols() []string {
	return []string{"Messages", "Pass", "Fail", "Pass %", "DKIM %", "SPF %", "Quarantine", "Reject", "Reports"}
}

func metricVals(m Metrics) []string {
	return []string{
		itoa(m.Messages), itoa(m.Pass), itoa(m.Fail), ftoa(m.PassPct), ftoa(m.DKIMPct), ftoa(m.SPFPct),
		itoa(m.Quarantine), itoa(m.Reject), strconv.Itoa(m.Reports),
	}
}

// CSVSections lists section keys accepted by --section.
var CSVSections = []string{"periods", "domains", "reporters", "sources", "failing", "reasons", "policy_changes"}

func tables(s *Summary) []table {
	periods := table{Key: "periods", Title: "DMARC results by " + string(s.Period),
		Header: append(append([]string{"Period"}, metricCols()...), "Δ Pass %")}
	for _, p := range s.Periods {
		delta := ""
		if p.DeltaPassPct != nil {
			delta = fmt.Sprintf("%+.2f", *p.DeltaPassPct)
		}
		periods.Rows = append(periods.Rows, append(append([]string{p.Label}, metricVals(p.Metrics)...), delta))
	}

	domains := table{Key: "domains", Title: "By header From domain",
		Header: append([]string{"Domain", "Policy"}, metricCols()...)}
	for _, d := range s.Domains {
		domains.Rows = append(domains.Rows, append([]string{d.Domain, d.Policy}, metricVals(d.Metrics)...))
	}

	reporters := table{Key: "reporters", Title: "By reporting organization",
		Header: append([]string{"Reporter"}, metricCols()...)}
	for _, r := range s.Reporters {
		reporters.Rows = append(reporters.Rows, append([]string{r.Org}, metricVals(r.Metrics)...))
	}

	sources := table{Key: "sources", Title: "Top sending sources",
		Header: []string{"Source IP", "PTR", "Header From", "Messages", "Pass", "Fail", "Pass %", "Reporters"}}
	for _, r := range s.TopSources {
		sources.Rows = append(sources.Rows, []string{r.IP, r.PTR, strings.Join(r.HeaderFroms, " "),
			itoa(r.Messages), itoa(r.Pass), itoa(r.Fail), ftoa(r.PassPct), strconv.Itoa(r.Reporters)})
	}

	failing := table{Key: "failing", Title: "DMARC failures (actionable)",
		Header: []string{"Source IP", "PTR", "Header From", "Messages", "Disposition", "DKIM auth", "SPF auth", "Reasons", "Reporters", "Hint"}}
	for _, f := range s.Failing {
		failing.Rows = append(failing.Rows, []string{f.IP, f.PTR, f.HeaderFrom, itoa(f.Messages), f.Disposition,
			f.DKIM, f.SPF, f.Reasons, f.Reporters, f.Hint})
	}

	reasons := table{Key: "reasons", Title: "Policy override reasons", Header: []string{"Reason", "Messages"}}
	for _, r := range s.Reasons {
		reasons.Rows = append(reasons.Rows, []string{r.Reason, itoa(r.Messages)})
	}

	changes := table{Key: "policy_changes", Title: "Published policy changes", Header: []string{"Domain", "Seen", "From", "To"}}
	for _, c := range s.PolicyChanges {
		changes.Rows = append(changes.Rows, []string{c.Domain, c.SeenAt.Format("2006-01-02"), c.From, c.To})
	}
	return []table{periods, domains, reporters, sources, failing, reasons, changes}
}

func rangeText(s *Summary) string {
	from, to := "beginning", "now"
	if !s.From.IsZero() {
		from = s.From.Format("2006-01-02")
	}
	if !s.To.IsZero() {
		to = s.To.Format("2006-01-02")
	}
	return from + " → " + to
}

// ------------------------------------------------------------------- text

func renderText(w io.Writer, s *Summary) error {
	o := s.Overall
	fmt.Fprintf(w, "DMARC aggregate report summary\n")
	fmt.Fprintf(w, "Range: %s (report begin, %s)   Period: %s   Generated: %s\n\n",
		rangeText(s), s.Timezone, s.Period, s.Generated.Format(time.RFC3339))
	if o.Messages == 0 {
		fmt.Fprintln(w, "No DMARC records found for the selected range.")
		return nil
	}
	fmt.Fprintf(w, "Messages: %d   DMARC pass: %d (%.2f%%)   fail: %d\n", o.Messages, o.Pass, o.PassPct, o.Fail)
	fmt.Fprintf(w, "DKIM aligned pass: %.2f%%   SPF aligned pass: %.2f%%\n", o.DKIMPct, o.SPFPct)
	fmt.Fprintf(w, "Disposition: none %d / quarantine %d / reject %d\n", o.None, o.Quarantine, o.Reject)
	fmt.Fprintf(w, "Reports: %d from %d reporters, %d source IPs\n", o.Reports, o.Reporters, o.SourceIPs)

	for _, t := range tables(s) {
		if len(t.Rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n== %s ==\n", t.Title)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join(t.Header, "\t"))
		for _, r := range t.Rows {
			fmt.Fprintln(tw, strings.Join(sanitizeCells(r), "\t"))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// sanitizeCells strips control characters (reporter-supplied strings are untrusted
// and must not inject terminal escape sequences or break table layout).
func sanitizeCells(r []string) []string {
	out := make([]string, len(r))
	for i, c := range r {
		out[i] = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return ' '
			}
			return r
		}, c)
	}
	return out
}

// --------------------------------------------------------------- markdown

func renderMarkdown(w io.Writer, s *Summary) error {
	o := s.Overall
	fmt.Fprintf(w, "# DMARC aggregate report summary\n\n")
	fmt.Fprintf(w, "- **Range:** %s (by report begin, %s)\n- **Period:** %s\n- **Generated:** %s\n\n",
		rangeText(s), s.Timezone, s.Period, s.Generated.Format(time.RFC3339))
	if o.Messages == 0 {
		_, err := fmt.Fprintln(w, "_No DMARC records found for the selected range._")
		return err
	}
	fmt.Fprintf(w, "| Messages | DMARC pass | Pass %% | DKIM %% | SPF %% | None | Quarantine | Reject | Reports | Reporters | Source IPs |\n")
	fmt.Fprintf(w, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	fmt.Fprintf(w, "| %d | %d | %.2f | %.2f | %.2f | %d | %d | %d | %d | %d | %d |\n",
		o.Messages, o.Pass, o.PassPct, o.DKIMPct, o.SPFPct, o.None, o.Quarantine, o.Reject, o.Reports, o.Reporters, o.SourceIPs)
	for _, t := range tables(s) {
		if len(t.Rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## %s\n\n| %s |\n|%s\n", t.Title, strings.Join(t.Header, " | "), strings.Repeat("---|", len(t.Header)))
		for _, r := range t.Rows {
			cells := sanitizeCells(r)
			for i := range cells {
				cells[i] = mdEscape(cells[i])
			}
			fmt.Fprintf(w, "| %s |\n", strings.Join(cells, " | "))
		}
	}
	return nil
}

func mdEscape(s string) string {
	return strings.NewReplacer("|", `\|`, "<", "&lt;", ">", "&gt;", "`", "\\`").Replace(s)
}

// -------------------------------------------------------------------- CSV

func renderCSV(w io.Writer, s *Summary, section string) error {
	if section == "" {
		section = "periods"
	}
	for _, t := range tables(s) {
		if t.Key == section {
			return writeCSV(w, t)
		}
	}
	return fmt.Errorf("unknown CSV section %q (%s)", section, strings.Join(CSVSections, ", "))
}

func writeCSV(w io.Writer, t table) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(t.Header); err != nil {
		return err
	}
	for _, r := range t.Rows {
		cells := make([]string, len(r))
		for i, c := range r {
			cells[i] = csvSafe(c)
		}
		if err := cw.Write(cells); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvSafe neutralizes spreadsheet formula injection from untrusted values.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return s // plain signed numbers such as "-1.50" are fine
		}
		return "'" + s
	}
	return s
}

// WriteCSVDir writes every non-empty section to dir/<section>.csv (0600).
// It returns the files written.
func WriteCSVDir(dir string, s *Summary) ([]string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	var written []string
	for _, t := range tables(s) {
		p := filepath.Join(dir, t.Key+".csv")
		f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return written, err
		}
		err = writeCSV(f, t)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return written, err
		}
		written = append(written, p)
	}
	return written, nil
}

// ---------------------------------------------------------------- helpers

func itoa(n int64) string   { return strconv.FormatInt(n, 10) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }
