package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Formats accepted by the renderers.
const (
	FormatText = "text"
	FormatCSV  = "csv"
	FormatJSON = "json"
)

// ValidFormat reports whether f is a known format.
func ValidFormat(f string) bool { return f == FormatText || f == FormatCSV || f == FormatJSON }

// clean removes control characters (server-supplied error text must not
// drive the terminal) and collapses whitespace.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// csvSafe neutralizes spreadsheet formula injection.
func csvSafe(s string) string {
	s = clean(s)
	if s != "" && strings.ContainsRune("=+-@", rune(s[0])) {
		return "'" + s
	}
	return s
}

func ts(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

func secs(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// RenderStatus writes the flow statuses.
func RenderStatus(w io.Writer, statuses []FlowStatus, format string) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(statuses)
	case FormatCSV:
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"flow", "health", "last_state", "last_sent", "last_success", "last_latency_seconds", "pending", "detail"})
		for _, s := range statuses {
			lat := ""
			if s.LatencySec != nil {
				lat = secs(*s.LatencySec)
			}
			_ = cw.Write([]string{csvSafe(s.Flow), s.Health, s.LastState, ts(s.LastSent), ts(s.LastSuccess), lat,
				strconv.Itoa(s.Pending), csvSafe(s.Detail)})
		}
		cw.Flush()
		return cw.Error()
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FLOW\tHEALTH\tLAST RESULT\tLATENCY\tLAST SENT\tLAST SUCCESS\tPENDING\tDETAIL")
	for _, s := range statuses {
		lat, state := "-", s.LastState
		if s.LatencySec != nil {
			lat = secs(*s.LatencySec) + "s"
		}
		if state == "" {
			state = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", clean(s.Flow), strings.ToUpper(s.Health), state, lat,
			ts(s.LastSent), ts(s.LastSuccess), s.Pending, clean(s.Detail))
	}
	return tw.Flush()
}

// RenderSummary writes the period report.
func RenderSummary(w io.Writer, s *Summary, format string) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	case FormatCSV:
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"flow", "sent", "delivered", "late", "lost", "send_failed", "pending", "success_percent",
			"latency_min_seconds", "latency_avg_seconds", "latency_p95_seconds", "latency_max_seconds"})
		for _, f := range s.Flows {
			_ = cw.Write([]string{csvSafe(f.Flow), strconv.Itoa(f.Sent), strconv.Itoa(f.Delivered), strconv.Itoa(f.Late),
				strconv.Itoa(f.Lost), strconv.Itoa(f.SendFailed), strconv.Itoa(f.Pending), secs(f.SuccessPct),
				secs(f.LatencyMin), secs(f.LatencyAvg), secs(f.LatencyP95), secs(f.LatencyMax)})
		}
		cw.Flush()
		return cw.Error()
	}

	if s.From != nil || s.To != nil {
		fmt.Fprintf(w, "Period: %s .. %s\n\n", ts(s.From), ts(s.To))
	}
	if len(s.Flows) == 0 {
		_, err := fmt.Fprintln(w, "No probes in the selected period.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FLOW\tSENT\tDELIVERED\tLATE\tLOST\tSEND FAILED\tPENDING\tSUCCESS\tLATENCY min/avg/p95/max (s)")
	for _, f := range s.Flows {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%.1f%%\t%s / %s / %s / %s\n", clean(f.Flow), f.Sent, f.Delivered,
			f.Late, f.Lost, f.SendFailed, f.Pending, f.SuccessPct,
			secs(f.LatencyMin), secs(f.LatencyAvg), secs(f.LatencyP95), secs(f.LatencyMax))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(s.Failures) > 0 {
		fmt.Fprintf(w, "\nLatest problems (%d):\n", len(s.Failures))
		tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "TIME\tFLOW\tSTATE\tDETAIL")
		for _, f := range s.Failures {
			at := f.At
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ts(&at), clean(f.Flow), f.State, clean(f.Detail))
		}
		return tw.Flush()
	}
	return nil
}
