package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/store"
)

// reportRow summarizes one domain, target and day.
type reportRow struct {
	Domain     string  `json:"domain"`
	Day        string  `json:"day"`
	Target     string  `json:"target"`
	Sent       int     `json:"sent"`
	Inbox      int     `json:"inbox"`
	Junk       int     `json:"junk"`
	Lost       int     `json:"lost"`
	Late       int     `json:"late"`
	Pending    int     `json:"pending"`
	SendFailed int     `json:"send_failed"`
	Bounced    int     `json:"bounced"`
	SPFPass    int     `json:"spf_pass"`
	DKIMPass   int     `json:"dkim_pass"`
	DMARCPass  int     `json:"dmarc_pass"`
	AvgLatency float64 `json:"avg_latency_seconds"`
}

type problem struct {
	Time    time.Time `json:"time"`
	Domain  string    `json:"domain"`
	Target  string    `json:"target"`
	Problem string    `json:"problem"`
}

type reportData struct {
	Rows     []*reportRow `json:"rows"`
	Problems []problem    `json:"problems"`
}

// maxProblems limits the problem list of the text report.
const maxProblems = 20

func buildReport(ctx context.Context, st *store.Store, domain string, since time.Time) (*reportData, error) {
	messages, err := st.Since(ctx, domain, since)
	if err != nil {
		return nil, err
	}
	data := &reportData{}
	rows := map[[3]string]*reportRow{}
	latencies := map[*reportRow][]time.Duration{}
	for _, m := range messages {
		key := [3]string{m.Domain, m.Day, m.Target}
		r := rows[key]
		if r == nil {
			r = &reportRow{Domain: m.Domain, Day: m.Day, Target: m.Target}
			rows[key] = r
			data.Rows = append(data.Rows, r)
		}
		add := func(text string) {
			data.Problems = append(data.Problems, problem{Time: m.CreatedAt, Domain: m.Domain, Target: m.Target, Problem: text})
		}
		if m.State == store.StateSendFailed {
			r.SendFailed++
			add("send failed: " + m.SendError)
			continue
		}
		r.Sent++
		switch m.State {
		case store.StatePending:
			r.Pending++
		case store.StateLost:
			r.Lost++
			add("lost: " + m.Subject)
		case store.StateLate:
			r.Late++
		}
		switch m.Placement {
		case "inbox":
			r.Inbox++
		case "junk":
			r.Junk++
			add(strings.TrimSpace("junk: " + m.Auth + " " + m.SpamInfo))
		}
		if !m.FoundAt.IsZero() {
			latencies[r] = append(latencies[r], m.Latency)
		}
		auth := " " + m.Auth + " "
		for verdict, n := range map[string]*int{"spf": &r.SPFPass, "dkim": &r.DKIMPass, "dmarc": &r.DMARCPass} {
			if strings.Contains(auth, " "+verdict+"=pass ") {
				*n++
			}
		}
		if m.CheckError != "" {
			add("check: " + m.CheckError)
		}
		bounces, err := st.Bounces(ctx, m.MessageID)
		if err != nil {
			return nil, err
		}
		if len(bounces) > 0 {
			r.Bounced++
			for _, b := range bounces {
				add(strings.TrimSpace(fmt.Sprintf("%s %s %s %s", b.Action, b.Status, b.RemoteMTA, b.Diagnostic)))
			}
		}
	}
	for r, list := range latencies {
		var sum time.Duration
		for _, l := range list {
			sum += l
		}
		r.AvgLatency = (sum / time.Duration(len(list))).Seconds()
	}
	slices.SortFunc(data.Rows, func(a, b *reportRow) int {
		return strings.Compare(a.Domain+a.Day+a.Target, b.Domain+b.Day+b.Target)
	})
	slices.SortStableFunc(data.Problems, func(a, b problem) int { return b.Time.Compare(a.Time) })
	return data, nil
}

var reportColumns = []string{"domain", "day", "target", "sent", "inbox", "junk", "lost", "late", "pending", "send_failed",
	"bounced", "spf_pass", "dkim_pass", "dmarc_pass", "avg_latency_s"}

func (r *reportRow) fields() []string {
	n := strconv.Itoa
	return []string{r.Domain, r.Day, r.Target, n(r.Sent), n(r.Inbox), n(r.Junk), n(r.Lost), n(r.Late), n(r.Pending),
		n(r.SendFailed), n(r.Bounced), n(r.SPFPass), n(r.DKIMPass), n(r.DMARCPass), strconv.FormatFloat(r.AvgLatency, 'f', 0, 64)}
}

func renderReport(out io.Writer, data *reportData, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "csv":
		w := csv.NewWriter(out)
		if err := w.Write(reportColumns); err != nil {
			return err
		}
		for _, r := range data.Rows {
			if err := w.Write(r.fields()); err != nil {
				return err
			}
		}
		w.Flush()
		return w.Error()
	case "text":
		if len(data.Rows) == 0 {
			_, err := fmt.Fprintln(out, "No messages in this period.")
			return err
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, strings.ToUpper(strings.Join(reportColumns, "\t"))); err != nil {
			return err
		}
		for _, r := range data.Rows {
			if _, err := fmt.Fprintln(w, strings.Join(r.fields(), "\t")); err != nil {
				return err
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if len(data.Problems) > 0 {
			if _, err := fmt.Fprintf(out, "\nLatest problems (%d total):\n", len(data.Problems)); err != nil {
				return err
			}
			for _, p := range data.Problems[:min(len(data.Problems), maxProblems)] {
				if _, err := fmt.Fprintf(out, "  %s  %s -> %s: %s\n", p.Time.Local().Format("01-02 15:04"), p.Domain, p.Target,
					oneLine(p.Problem, 200)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return fmt.Errorf("invalid format %q (text, csv, json)", format)
}

// oneLine keeps server-supplied text from breaking or flooding the terminal.
func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)), " ")
	if len(s) > limit {
		s = strings.ToValidUTF8(s[:limit], "") + "..."
	}
	return s
}
