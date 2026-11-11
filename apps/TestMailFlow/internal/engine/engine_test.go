//go:build !integration

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/eventlog"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// world is a fake mail system: messages accepted by the sender are placed
// in the mailbox after deliverAfter (or never, when drop is set).
type world struct {
	now          time.Time
	deliverAfter time.Duration
	drop         bool
	sendErr      error
	fetchErr     error

	mailbox []fetch.RawMessage // delivered messages (Received = delivery time)
	queue   []queued
	deleted []string
	events  []string
	fetches int
	seq     int
}

type queued struct {
	at  time.Time
	raw []byte
}

func (w *world) clock() time.Time { return w.now }

// advance moves the clock and delivers queued messages that are due.
func (w *world) advance(d time.Duration) {
	w.now = w.now.Add(d)
	rest := w.queue[:0]
	for _, q := range w.queue {
		if q.at.After(w.now) {
			rest = append(rest, q)
			continue
		}
		w.seq++
		// The receiving server stamps a Received header, as real mail has.
		raw := "Received: from relay by mbx; " + q.at.Format(time.RFC1123Z) + "\r\n" + string(q.raw)
		w.mailbox = append(w.mailbox, fetch.RawMessage{Key: fmt.Sprintf("k%d", w.seq), Raw: []byte(raw)})
	}
	w.queue = rest
}

// Report implements eventlog.Sink.
func (w *world) Report(_ eventlog.Level, id uint32, msg string) {
	w.events = append(w.events, fmt.Sprintf("%d %s", id, msg))
}
func (w *world) Close() error { return nil }

type fakeSender struct{ w *world }

func (s fakeSender) Send(_ context.Context, m send.Message) (send.Result, error) {
	if s.w.sendErr != nil {
		return send.Result{}, s.w.sendErr
	}
	if !s.w.drop {
		s.w.queue = append(s.w.queue, queued{at: s.w.now.Add(s.w.deliverAfter), raw: m.Raw})
	}
	return send.Result{ServerResponse: "250 queued", RemoteHost: "fake"}, nil
}
func (fakeSender) Close() error { return nil }

type fakeSession struct{ w *world }

func (s fakeSession) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	s.w.fetches++
	for _, m := range s.w.mailbox {
		if c.SkipKey != nil && c.SkipKey(m.Key) {
			continue
		}
		if err := sink(ctx, m); err != nil {
			return err
		}
	}
	return nil
}
func (s fakeSession) Delete(_ context.Context, key string) error {
	s.w.deleted = append(s.w.deleted, key)
	return nil
}
func (fakeSession) Close() error { return nil }

type fakeOpener struct{ w *world }

func (o fakeOpener) Sender(context.Context, string) (send.Sender, error) { return fakeSender(o), nil }
func (o fakeOpener) Fetcher(context.Context, string, bool) (fetch.Session, error) {
	if o.w.fetchErr != nil {
		return nil, o.w.fetchErr
	}
	return fakeSession(o), nil
}

func testPlan() *config.Plan {
	return &config.Plan{
		Endpoints: map[string]*config.Endpoint{
			"out": {Name: "out", Protocol: "smtp"},
			"in":  {Name: "in", Protocol: "imap"},
		},
		Flows: []*config.Flow{{
			Name: "f1", SendEndpoint: "out", From: "a@example.com", To: []string{"b@example.net"},
			CheckEndpoint: "in", Delete: true, Interval: 15 * time.Minute,
			Checks:    []time.Duration{5 * time.Minute, 150 * time.Second, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute},
			WarnAfter: 5 * time.Minute,
		}},
	}
}

func newTestEngine(t *testing.T) (*Engine, *world, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	w := &world{now: time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)}
	e := New(testPlan(), st, fakeOpener{w}, slog.New(slog.NewTextHandler(io.Discard, nil)), w)
	e.Now = w.clock
	e.SendRetries = 0
	if err := e.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return e, w, st
}

func mustProbe(t *testing.T, st *store.Store, id int64) *store.Probe {
	t.Helper()
	p, err := st.ProbeByID(context.Background(), id)
	if err != nil || p == nil {
		t.Fatalf("ProbeByID(%d) = %v, %v", id, p, err)
	}
	return p
}

func TestEngine_DeliveredOnFirstCheck(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.deliverAfter = 40 * time.Second

	p, err := e.SendProbe(ctx, e.Plan.Flows[0])
	if err != nil {
		t.Fatalf("SendProbe() error = %v", err)
	}
	// Before the first check time nothing is fetched.
	w.advance(4 * time.Minute)
	if err := e.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	if w.fetches != 0 {
		t.Fatalf("fetches = %d before the first check time, want 0", w.fetches)
	}
	w.advance(time.Minute)
	if err := e.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	got := mustProbe(t, st, p.ID)
	if got.State != store.StateDelivered || got.Latency != 40*time.Second || got.Attempts != 1 || got.Hops != 1 {
		t.Errorf("probe = state %s latency %s attempts %d hops %d; want delivered 40s 1 1", got.State, got.Latency, got.Attempts, got.Hops)
	}
	if len(w.deleted) != 1 {
		t.Errorf("deleted = %v, want the probe message deleted", w.deleted)
	}
	if len(w.events) != 0 {
		t.Errorf("events = %v, want none for a first healthy probe", w.events)
	}
}

func TestEngine_LateOnThirdCheckThenRecovers(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.deliverAfter = 9 * time.Minute // after check 2 (7m30s), before check 3 (12m30s)

	p, _ := e.SendProbe(ctx, e.Plan.Flows[0])
	for _, step := range []time.Duration{5 * time.Minute, 150 * time.Second, 5 * time.Minute} {
		w.advance(step)
		if err := e.CheckDue(ctx); err != nil {
			t.Fatalf("CheckDue() error = %v", err)
		}
	}
	got := mustProbe(t, st, p.ID)
	if got.State != store.StateLate || got.Attempts != 3 || got.Latency != 9*time.Minute {
		t.Fatalf("probe = state %s attempts %d latency %s; want late 3 9m", got.State, got.Attempts, got.Latency)
	}
	if len(w.events) != 1 || !strings.HasPrefix(w.events[0], fmt.Sprint(eventlog.IDProbeLate)) {
		t.Fatalf("events = %v, want one probe-late event", w.events)
	}

	// A following fast probe raises exactly one recovery event.
	w.deliverAfter = 10 * time.Second
	p2, _ := e.SendProbe(ctx, e.Plan.Flows[0])
	w.advance(5 * time.Minute)
	if err := e.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	if got := mustProbe(t, st, p2.ID); got.State != store.StateDelivered {
		t.Fatalf("second probe state = %s, want delivered", got.State)
	}
	if len(w.events) != 2 || !strings.HasPrefix(w.events[1], fmt.Sprint(eventlog.IDFlowRecovered)) {
		t.Errorf("events = %v, want late then recovered", w.events)
	}
}

func TestEngine_LostAfterAllChecks(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.drop = true

	f := e.Plan.Flows[0]
	p, _ := e.SendProbe(ctx, f)
	for i, step := range f.Checks {
		w.advance(step)
		if err := e.CheckDue(ctx); err != nil {
			t.Fatalf("CheckDue() error = %v", err)
		}
		got := mustProbe(t, st, p.ID)
		wantState := store.StatePending
		if i == len(f.Checks)-1 {
			wantState = store.StateLost
		}
		if got.State != wantState || got.Attempts != i+1 {
			t.Fatalf("after check %d: state %s attempts %d; want %s %d", i+1, got.State, got.Attempts, wantState, i+1)
		}
	}
	if w.fetches != len(f.Checks) {
		t.Errorf("fetches = %d, want %d", w.fetches, len(f.Checks))
	}
	if len(w.events) != 1 || !strings.HasPrefix(w.events[0], fmt.Sprint(eventlog.IDProbeLost)) {
		t.Errorf("events = %v, want one probe-lost event", w.events)
	}

	// A second lost probe does not repeat the event (no state change).
	p2, _ := e.SendProbe(ctx, f)
	w.advance(f.Deadline())
	if err := e.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	if got := mustProbe(t, st, p2.ID); got.State != store.StateLost {
		t.Fatalf("second probe state = %s, want lost (one check covers all elapsed attempts)", got.State)
	}
	if len(w.events) != 1 {
		t.Errorf("events = %v, want still one", w.events)
	}
}

func TestEngine_SendFailure(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.sendErr = errors.New("550 5.7.1 relay denied")

	p, err := e.SendProbe(ctx, e.Plan.Flows[0])
	if err == nil || !strings.Contains(err.Error(), "relay denied") {
		t.Fatalf("SendProbe() error = %v, want relay denied", err)
	}
	got := mustProbe(t, st, p.ID)
	if got.State != store.StateSendFailed || !strings.Contains(got.SendError, "relay denied") {
		t.Errorf("probe = state %s error %q", got.State, got.SendError)
	}
	if len(w.events) != 1 || !strings.HasPrefix(w.events[0], fmt.Sprint(eventlog.IDSendFailed)) {
		t.Errorf("events = %v, want one send-failed event", w.events)
	}
	if pending, _ := st.PendingProbes(ctx); len(pending) != 0 {
		t.Errorf("pending = %d, want 0", len(pending))
	}
}

func TestEngine_CheckerOutageIsNotLostMail(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.deliverAfter = 30 * time.Second
	f := e.Plan.Flows[0]

	p, _ := e.SendProbe(ctx, f)
	w.fetchErr = errors.New("authentication failed")
	// The mailbox stays unreachable far beyond the whole schedule.
	for elapsed := time.Duration(0); elapsed < f.Deadline()+10*time.Minute; elapsed += 5 * time.Minute {
		w.advance(5 * time.Minute)
		if err := e.CheckDue(ctx); err != nil {
			t.Fatalf("CheckDue() error = %v", err)
		}
	}
	got := mustProbe(t, st, p.ID)
	if got.State != store.StatePending || got.Attempts != 0 || !strings.Contains(got.CheckError, "authentication failed") {
		t.Fatalf("probe = state %s attempts %d check_error %q; want pending 0 with error", got.State, got.Attempts, got.CheckError)
	}
	if len(w.events) != 1 || !strings.HasPrefix(w.events[0], fmt.Sprint(eventlog.IDCheckerFailing)) {
		t.Fatalf("events = %v, want exactly one checker-failing event", w.events)
	}

	// Once the mailbox is reachable the probe is found with its true latency.
	w.fetchErr = nil
	w.advance(time.Minute)
	if err := e.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	got = mustProbe(t, st, p.ID)
	if got.State != store.StateDelivered || got.Latency != 30*time.Second || got.CheckError != "" {
		t.Errorf("probe = state %s latency %s check_error %q; want delivered 30s", got.State, got.Latency, got.CheckError)
	}
	if len(w.events) != 2 || !strings.HasPrefix(w.events[1], fmt.Sprint(eventlog.IDCheckerRecovered)) {
		t.Errorf("events = %v, want checker failing then recovered", w.events)
	}
}

func TestEngine_RestartResumesPendingProbes(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.deliverAfter = 20 * time.Second
	p, _ := e.SendProbe(ctx, e.Plan.Flows[0])

	// A probe row stored but never submitted (crash between insert and send).
	orphan := &store.Probe{Flow: "f1", Token: strings.Repeat("ab", 16), MessageID: "x@y", CreatedAt: w.now}
	if err := st.InsertProbe(ctx, orphan); err != nil {
		t.Fatalf("InsertProbe() error = %v", err)
	}

	// "Restart": a new engine on the same database, 20 minutes later.
	w.advance(20 * time.Minute)
	e2 := New(testPlan(), st, fakeOpener{w}, e.Log, w)
	e2.Now = w.clock
	if err := e2.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got := mustProbe(t, st, orphan.ID); got.State != store.StateSendFailed {
		t.Errorf("orphan state = %s, want send_failed", got.State)
	}
	if err := e2.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	got := mustProbe(t, st, p.ID)
	if got.State != store.StateDelivered || got.Latency != 20*time.Second {
		t.Errorf("probe = state %s latency %s; want delivered 20s", got.State, got.Latency)
	}
}

func TestEngine_IgnoresForeignAndSeenMessages(t *testing.T) {
	ctx := context.Background()
	e, w, st := newTestEngine(t)
	w.drop = true
	w.mailbox = []fetch.RawMessage{
		{Key: "other", Raw: []byte("Subject: hello\r\n\r\nnot a probe")},
		{Key: "foreign", Raw: []byte("Subject: [TMF] f1 " + strings.Repeat("0", 32) + "\r\n\r\nprobe of another instance")},
	}
	p, _ := e.SendProbe(ctx, e.Plan.Flows[0])
	w.advance(5 * time.Minute)
	if err := e.CheckDue(ctx); err != nil {
		t.Fatalf("CheckDue() error = %v", err)
	}
	if got := mustProbe(t, st, p.ID); got.State != store.StatePending || got.Attempts != 1 {
		t.Errorf("probe = state %s attempts %d; want pending 1", got.State, got.Attempts)
	}
	if len(w.deleted) != 0 {
		t.Errorf("deleted = %v, want foreign messages left alone", w.deleted)
	}
	for _, key := range []string{"other", "foreign"} {
		if seen, _ := st.IsSeen(ctx, "in", key); !seen {
			t.Errorf("message %s not remembered as seen", key)
		}
	}
}
