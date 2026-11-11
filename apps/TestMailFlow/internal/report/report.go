// Package report derives per-flow status and statistics from the probe
// history and renders them as text, CSV or JSON.
package report

import (
	"context"
	"sort"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
)

// HealthStale means no probe was sent recently: the service is not running.
const HealthStale = "stale"

// FlowStatus is the current state of one flow.
type FlowStatus struct {
	Flow        string     `json:"flow"`
	Health      string     `json:"health"` // ok, degraded, failing, stale, unknown
	LastState   string     `json:"last_state,omitempty"`
	LastSent    *time.Time `json:"last_sent,omitempty"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
	LatencySec  *float64   `json:"last_latency_seconds,omitempty"`
	Pending     int        `json:"pending"`
	Detail      string     `json:"detail,omitempty"`
}

// Status computes the status of every configured flow.
func Status(ctx context.Context, st *store.Store, plan *config.Plan, now time.Time) ([]FlowStatus, error) {
	pending, err := st.PendingProbes(ctx)
	if err != nil {
		return nil, err
	}
	pendingBy := map[string]int{}
	checkErr := map[string]string{}
	for _, p := range pending {
		pendingBy[p.Flow]++
		if p.CheckError != "" {
			checkErr[p.Flow] = p.CheckError
		}
	}

	out := make([]FlowStatus, 0, len(plan.Flows))
	for _, f := range plan.Flows {
		fs := FlowStatus{Flow: f.Name, Health: store.HealthUnknown, Pending: pendingBy[f.Name]}
		newest, err := st.LastProbe(ctx, f.Name, false)
		if err != nil {
			return nil, err
		}
		finished, err := st.LastProbe(ctx, f.Name, true)
		if err != nil {
			return nil, err
		}
		if finished != nil {
			fs.Health, fs.LastState = store.HealthOf(finished.State), finished.State
			switch finished.State {
			case store.StateSendFailed:
				fs.Detail = "send failed: " + finished.SendError
			case store.StateLost:
				fs.Detail = "probe sent " + finished.SentAt.Format(time.RFC3339) + " did not arrive"
			case store.StateDelivered, store.StateLate:
				sec := finished.Latency.Seconds()
				fs.LatencySec = &sec
			}
		}
		if newest != nil {
			t := newest.CreatedAt
			fs.LastSent = &t
			if now.Sub(newest.CreatedAt) > 2*f.Interval+time.Minute {
				fs.Health = HealthStale
				fs.Detail = "no probe sent since " + newest.CreatedAt.Format(time.RFC3339) + " (is the service running?)"
			}
		}
		if ce := checkErr[f.Name]; ce != "" && fs.Health != HealthStale {
			fs.Detail = "mailbox check failing: " + ce
			if fs.Health == store.HealthOK || fs.Health == store.HealthUnknown {
				fs.Health = store.HealthDegraded
			}
		}
		last, err := st.LastSuccess(ctx, f.Name)
		if err != nil {
			return nil, err
		}
		if !last.IsZero() {
			fs.LastSuccess = &last
		}
		out = append(out, fs)
	}
	return out, nil
}

// Worst returns an exit-code style severity: 0 all ok, 1 degraded/unknown, 2 failing/stale.
func Worst(statuses []FlowStatus) int {
	worst := 0
	for _, s := range statuses {
		sev := 0
		switch s.Health {
		case store.HealthOK:
		case store.HealthFailing, HealthStale:
			sev = 2
		default:
			sev = 1
		}
		if sev > worst {
			worst = sev
		}
	}
	return worst
}

// FlowStats summarizes the probes of one flow over a period.
type FlowStats struct {
	Flow       string  `json:"flow"`
	Sent       int     `json:"sent"`
	Delivered  int     `json:"delivered"`
	Late       int     `json:"late"`
	Lost       int     `json:"lost"`
	SendFailed int     `json:"send_failed"`
	Pending    int     `json:"pending"`
	SuccessPct float64 `json:"success_percent"` // (delivered+late) / finished
	LatencyMin float64 `json:"latency_min_seconds"`
	LatencyAvg float64 `json:"latency_avg_seconds"`
	LatencyP95 float64 `json:"latency_p95_seconds"`
	LatencyMax float64 `json:"latency_max_seconds"`
}

// Failure is one unsuccessful or late probe.
type Failure struct {
	Flow   string    `json:"flow"`
	At     time.Time `json:"at"`
	State  string    `json:"state"`
	Detail string    `json:"detail,omitempty"`
}

// Summary is a report over a period.
type Summary struct {
	From     *time.Time  `json:"from,omitempty"`
	To       *time.Time  `json:"to,omitempty"`
	Flows    []FlowStats `json:"flows"`
	Failures []Failure   `json:"failures"`
}

// Summarize aggregates probes (oldest first) into per-flow statistics and
// the newest maxFailures problem probes.
func Summarize(probes []*store.Probe, maxFailures int) *Summary {
	byFlow := map[string]*FlowStats{}
	lat := map[string][]float64{}
	s := &Summary{Flows: []FlowStats{}, Failures: []Failure{}}
	for _, p := range probes {
		fs := byFlow[p.Flow]
		if fs == nil {
			fs = &FlowStats{Flow: p.Flow}
			byFlow[p.Flow] = fs
		}
		fs.Sent++
		var detail string
		switch p.State {
		case store.StateDelivered:
			fs.Delivered++
			lat[p.Flow] = append(lat[p.Flow], p.Latency.Seconds())
			continue
		case store.StatePending:
			fs.Pending++
			continue
		case store.StateLate:
			fs.Late++
			lat[p.Flow] = append(lat[p.Flow], p.Latency.Seconds())
			detail = "arrived after " + p.Latency.Round(time.Second).String()
		case store.StateLost:
			fs.Lost++
			detail = "not arrived"
		case store.StateSendFailed:
			fs.SendFailed++
			detail = p.SendError
		}
		s.Failures = append(s.Failures, Failure{Flow: p.Flow, At: p.CreatedAt, State: p.State, Detail: detail})
	}
	for name, fs := range byFlow {
		if finished := fs.Sent - fs.Pending; finished > 0 {
			fs.SuccessPct = 100 * float64(fs.Delivered+fs.Late) / float64(finished)
		}
		if v := lat[name]; len(v) > 0 {
			sort.Float64s(v)
			var sum float64
			for _, x := range v {
				sum += x
			}
			fs.LatencyMin, fs.LatencyMax, fs.LatencyAvg = v[0], v[len(v)-1], sum/float64(len(v))
			fs.LatencyP95 = v[percentileIndex(len(v), 95)]
		}
		s.Flows = append(s.Flows, *fs)
	}
	sort.Slice(s.Flows, func(i, j int) bool { return s.Flows[i].Flow < s.Flows[j].Flow })
	// newest failures first
	for i, j := 0, len(s.Failures)-1; i < j; i, j = i+1, j-1 {
		s.Failures[i], s.Failures[j] = s.Failures[j], s.Failures[i]
	}
	if maxFailures >= 0 && len(s.Failures) > maxFailures {
		s.Failures = s.Failures[:maxFailures]
	}
	return s
}

// percentileIndex is the nearest-rank index of percentile p in n sorted values.
func percentileIndex(n, p int) int {
	idx := (n*p+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return idx
}
