//go:build !integration

package report

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
)

func probes() []*store.Probe {
	t0 := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	mk := func(i int, flow, state string, lat time.Duration, sendErr string) *store.Probe {
		return &store.Probe{Flow: flow, State: state, Latency: lat, SendError: sendErr, CreatedAt: t0.Add(time.Duration(i) * time.Minute)}
	}
	return []*store.Probe{
		mk(0, "a", store.StateDelivered, 10*time.Second, ""),
		mk(1, "a", store.StateDelivered, 20*time.Second, ""),
		mk(2, "a", store.StateLate, 400*time.Second, ""),
		mk(3, "a", store.StateLost, 0, ""),
		mk(4, "a", store.StatePending, 0, ""),
		mk(5, "b", store.StateSendFailed, 0, "=cmd|' /C calc'!A0\x1b[31m"),
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize(probes(), 2)
	if len(s.Flows) != 2 || s.Flows[0].Flow != "a" || s.Flows[1].Flow != "b" {
		t.Fatalf("flows = %+v", s.Flows)
	}
	a := s.Flows[0]
	if a.Sent != 5 || a.Delivered != 2 || a.Late != 1 || a.Lost != 1 || a.Pending != 1 {
		t.Errorf("counts = %+v", a)
	}
	if a.SuccessPct != 75 {
		t.Errorf("SuccessPct = %v, want 75 (3 of 4 finished)", a.SuccessPct)
	}
	if a.LatencyMin != 10 || a.LatencyMax != 400 || a.LatencyP95 != 400 {
		t.Errorf("latency = min %v max %v p95 %v", a.LatencyMin, a.LatencyMax, a.LatencyP95)
	}
	if len(s.Failures) != 2 || s.Failures[0].Flow != "b" || s.Failures[1].State != store.StateLost {
		t.Errorf("failures = %+v, want newest two (send_failed, lost)", s.Failures)
	}
}

func TestRenderSummary_EscapesUntrustedText(t *testing.T) {
	s := Summarize(probes(), 10)
	for _, format := range []string{FormatText, FormatCSV, FormatJSON} {
		var buf bytes.Buffer
		if err := RenderSummary(&buf, s, format); err != nil {
			t.Fatalf("RenderSummary(%s) error = %v", format, err)
		}
		if format == FormatText && strings.ContainsRune(buf.String(), 0x1b) {
			t.Errorf("text output contains a raw escape character")
		}
		if !strings.Contains(buf.String(), "a") {
			t.Errorf("%s output lacks flow a: %s", format, buf.String())
		}
	}
	if got := csvSafe("=1+1"); got != "'=1+1" {
		t.Errorf("csvSafe() = %q", got)
	}
}

func TestStatus(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer func() { _ = st.Close() }()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	plan := &config.Plan{Flows: []*config.Flow{
		{Name: "good", Interval: 15 * time.Minute},
		{Name: "bad", Interval: 15 * time.Minute},
		{Name: "stopped", Interval: 15 * time.Minute},
		{Name: "new", Interval: 15 * time.Minute},
	}}
	add := func(flow, token string, created time.Time) *store.Probe {
		p := &store.Probe{Flow: flow, Token: token, MessageID: token, CreatedAt: created}
		if err := st.InsertProbe(ctx, p); err != nil {
			t.Fatalf("InsertProbe() error = %v", err)
		}
		if err := st.MarkSent(ctx, p.ID, created, created.Add(5*time.Minute), "250", "h", ""); err != nil {
			t.Fatalf("MarkSent() error = %v", err)
		}
		return p
	}
	g := add("good", "t1", now.Add(-10*time.Minute))
	_ = st.MarkArrived(ctx, g.ID, store.StateDelivered, now.Add(-9*time.Minute), now.Add(-5*time.Minute), 30*time.Second, 3, "spf=pass", 1)
	b := add("bad", "t2", now.Add(-50*time.Minute))
	_ = st.MarkLost(ctx, b.ID, 5, now.Add(-12*time.Minute))
	add("bad", "t3", now.Add(-5*time.Minute)) // still pending
	s := add("stopped", "t4", now.Add(-3*time.Hour))
	_ = st.MarkArrived(ctx, s.ID, store.StateDelivered, now.Add(-3*time.Hour), now.Add(-3*time.Hour), time.Second, 1, "", 1)

	statuses, err := Status(ctx, st, plan, now)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	want := map[string]string{"good": store.HealthOK, "bad": store.HealthFailing, "stopped": HealthStale, "new": store.HealthUnknown}
	for _, fs := range statuses {
		if fs.Health != want[fs.Flow] {
			t.Errorf("flow %s health = %s, want %s", fs.Flow, fs.Health, want[fs.Flow])
		}
		if fs.Flow == "bad" && fs.Pending != 1 {
			t.Errorf("flow bad pending = %d, want 1", fs.Pending)
		}
		if fs.Flow == "good" && (fs.LatencySec == nil || *fs.LatencySec != 30) {
			t.Errorf("flow good latency = %v, want 30", fs.LatencySec)
		}
	}
	if got := Worst(statuses); got != 2 {
		t.Errorf("Worst() = %d, want 2", got)
	}
	var buf bytes.Buffer
	if err := RenderStatus(&buf, statuses, FormatText); err != nil || !strings.Contains(buf.String(), "FAILING") {
		t.Errorf("RenderStatus() = %q, %v", buf.String(), err)
	}
}
