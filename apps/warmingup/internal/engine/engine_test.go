//go:build !integration

package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/content"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/schedule"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// world fakes the mail system: what is submitted appears in the recipient's
// mailbox in the configured placement.
type world struct {
	now       time.Time
	placement map[string]mailbox.Placement // target endpoint; "" = never arrives
	delivered map[string]map[string]bool   // target -> Message-ID
	sent      []sent
	sendErr   error
	read      map[string]bool
	rescued   map[string]bool
	bounceDSN []string // raw reports
	contentN  int
	contentOK bool
}

type sent struct {
	at     time.Time
	to     string
	domain string
}

func (w *world) Next(context.Context, []string, []string, map[string]bool, *rand.Rand) (content.Item, error) {
	if !w.contentOK {
		return content.Item{}, errors.New("offline")
	}
	w.contentN++
	return content.Item{Key: fmt.Sprint("wikipedia:", w.contentN), Source: "wikipedia", Subject: "S", Text: "T", Attribution: "A"}, nil
}

func (w *world) Sender(context.Context, string) (send.Sender, error) { return w, nil }
func (w *world) Close() error                                        { return nil }

func (w *world) Send(_ context.Context, m send.Message) (send.Result, error) {
	if w.sendErr != nil {
		return send.Result{}, w.sendErr
	}
	target, _, _ := strings.Cut(m.To[0], "@")
	_, rest, _ := strings.Cut(string(m.Raw), "Message-ID: <")
	id, _, _ := strings.Cut(rest, ">")
	if w.delivered[target] == nil {
		w.delivered[target] = map[string]bool{}
	}
	w.delivered[target][id] = true
	_, domain, _ := strings.Cut(id, "@")
	w.sent = append(w.sent, sent{at: w.now, to: target, domain: domain})
	return send.Result{ServerResponse: "250 ok"}, nil
}

type box struct {
	w    *world
	name string
}

func (w *world) Mailbox(_ context.Context, name string) (mailbox.Session, error) {
	return &box{w, name}, nil
}
func (b *box) Folders() (string, string) { return "INBOX", "Junk" }
func (b *box) Close() error              { return nil }
func (b *box) MarkRead(_ context.Context, id string) error {
	b.w.read[id] = true
	return nil
}
func (b *box) Rescue(_ context.Context, id string) error {
	if b.w.placement[b.name] == mailbox.Junk {
		b.w.rescued[id] = true
	}
	return nil
}
func (b *box) Locate(_ context.Context, id string) (*mailbox.Message, error) {
	p := b.w.placement[b.name]
	if p == "" || !b.w.delivered[b.name][id] {
		return nil, nil
	}
	return &mailbox.Message{Placement: p, Folder: string(p), Header: []byte("Authentication-Results: mx; spf=pass\r\n\r\n")}, nil
}

type bounces struct{ w *world }

func (w *world) Fetcher(context.Context, string) (fetch.Session, error) { return bounces{w}, nil }
func (b bounces) Delete(context.Context, string) error                  { return errors.New("must not delete") }
func (b bounces) Close() error                                          { return nil }
func (b bounces) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	for i, raw := range b.w.bounceDSN {
		key := fmt.Sprint(i)
		if c.SkipKey != nil && c.SkipKey(key) {
			continue
		}
		if err := sink(ctx, fetch.RawMessage{Key: key, Raw: []byte(raw)}); err != nil {
			return err
		}
	}
	return nil
}

func domain(t *testing.T, name string, targets ...string) *config.Domain {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	days := map[time.Weekday]bool{time.Monday: true, time.Tuesday: true, time.Wednesday: true, time.Thursday: true, time.Friday: true}
	d := &config.Domain{
		Name: name, Enabled: true, SenderEndpoint: "mta", From: "news@" + name, FromAddress: "news@" + name,
		BounceEndpoint: "bounce", Location: loc, Start: time.Date(2026, 12, 7, 8, 0, 0, 0, loc),
		Schedule: schedule.Params{Location: loc, Business: &schedule.Hours{Start: 480, End: 1020, Days: days},
			RampDays: 30, StartPerDay: 1, BusinessPerHour: 3, OffMin: 1, OffMax: 2, MinGap: 10 * time.Minute},
		Hold:      config.Hold{Window: 10, MinSamples: 5, BadRatio: 0.3, LateRatio: 0.5, DSN: 3, SendFailures: 3},
		Engage:    config.Engage{MinDelay: 10 * time.Minute, MaxDelay: time.Hour},
		WarnAfter: 5 * time.Minute,
	}
	for _, tg := range targets {
		d.Targets = append(d.Targets, config.Target{Endpoint: tg, Address: tg + "@x.test", Engage: true})
	}
	return d
}

func setup(t *testing.T, domains ...*config.Domain) (*Engine, *world) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	w := &world{now: domains[0].Start.Add(-time.Hour), contentOK: true, placement: map[string]mailbox.Placement{},
		delivered: map[string]map[string]bool{}, read: map[string]bool{}, rescued: map[string]bool{}}
	for _, d := range domains {
		for _, tg := range d.Targets {
			w.placement[tg.Endpoint] = mailbox.Inbox
		}
	}
	e := New(&config.Config{Domains: domains}, st, w, slog.New(slog.DiscardHandler))
	e.Content, e.Now, e.Rand = w, func() time.Time { return w.now }, rand.New(rand.NewPCG(5, 6))
	return e, w
}

// run advances the clock in five minute steps.
func run(t *testing.T, e *Engine, w *world, until time.Time) {
	t.Helper()
	for w.now.Before(until) {
		if err := e.Tick(context.Background()); err != nil {
			t.Fatalf("Tick at %s: %v", w.now, err)
		}
		w.now = w.now.Add(5 * time.Minute)
	}
}

func ramp(t *testing.T, e *Engine, domain, target string) store.Ramp {
	t.Helper()
	var r store.Ramp
	var err error
	if target == "" {
		r, err = e.Store.DomainRamp(context.Background(), domain)
	} else {
		r, err = e.Store.TargetRamp(context.Background(), domain, target)
	}
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRampAndCampaignWindow(t *testing.T) {
	d := domain(t, "example.com", "a", "b", "c")
	d.End = d.Start.AddDate(0, 0, 35)
	e, w := setup(t, d)

	run(t, e, w, d.Start.Add(-time.Minute))
	if len(w.sent) != 0 {
		t.Fatalf("sent %d before the campaign start", len(w.sent))
	}
	run(t, e, w, d.End.AddDate(0, 0, 3))

	perDay := map[string]int{}
	for _, s := range w.sent {
		if s.at.Before(d.Start) || !s.at.Before(d.End) {
			t.Fatalf("sent outside the campaign: %s", s.at)
		}
		perDay[s.at.In(d.Location).Format("2006-01-02")]++
	}
	if n := perDay["2026-12-07"]; n != 1 {
		t.Errorf("first day = %d messages, want 1", n)
	}
	if n := perDay["2027-01-06"]; n < 27 || n > 29 { // Wednesday, ramp day 31
		t.Errorf("peak day = %d messages, want 28-29", n)
	}
	if n := perDay["2027-01-09"]; n < 1 || n > 2 { // Saturday
		t.Errorf("weekend = %d messages, want 1-2", n)
	}
	for _, tg := range []string{"a", "b", "c"} {
		if r := ramp(t, e, d.Name, tg); r.HeldReason != "" {
			t.Errorf("target %s held: %s", tg, r.HeldReason)
		}
	}
	counts := map[string]int{}
	for _, s := range w.sent {
		counts[s.to]++
	}
	if counts["a"]-counts["c"] > 3 || counts["c"]-counts["a"] > 3 {
		t.Errorf("uneven spread over targets: %v", counts)
	}
	if len(w.read) == 0 {
		t.Error("nothing was marked read")
	}
}

func TestJunkHoldsOnlyThatTargetAndResume(t *testing.T) {
	d := domain(t, "example.com", "good", "bad")
	e, w := setup(t, d)
	w.placement["bad"] = mailbox.Junk

	run(t, e, w, d.Start.AddDate(0, 0, 25))
	good, bad := ramp(t, e, d.Name, "good"), ramp(t, e, d.Name, "bad")
	if bad.HeldReason == "" || good.HeldReason != "" {
		t.Fatalf("holds: bad=%q good=%q", bad.HeldReason, good.HeldReason)
	}
	if good.Day != 26 || bad.Day >= good.Day {
		t.Errorf("ramp days: good %d, bad %d", good.Day, bad.Day)
	}
	if len(w.rescued) == 0 {
		t.Error("junk was not rescued")
	}
	m, err := e.Store.Resolved(context.Background(), d.Name, "bad", time.Time{}, 1)
	if err != nil || len(m) != 1 || m[0].Placement != "junk" {
		t.Errorf("first placement must stay junk after rescue: %v %v", m, err)
	}
	// The held target keeps its smaller share.
	last := d.Start.AddDate(0, 0, 24).Format("2006-01-02")
	today, _, err := e.Store.DayCounts(context.Background(), d.Name, last)
	if err != nil {
		t.Fatal(err)
	}
	if share := (d.Schedule.Total(bad.Day) + 1) / 2; today["bad"] > share {
		t.Errorf("held target got %d, share %d", today["bad"], share)
	}

	if err := e.Resume(context.Background(), d, "bad"); err != nil {
		t.Fatal(err)
	}
	w.placement["bad"] = mailbox.Inbox
	run(t, e, w, d.Start.AddDate(0, 0, 27))
	if r := ramp(t, e, d.Name, "bad"); r.HeldReason != "" || r.Day <= bad.Day {
		t.Errorf("after resume = %+v", r)
	}
}

func TestLostThenLate(t *testing.T) {
	d := domain(t, "example.com", "a")
	e, w := setup(t, d)
	w.placement["a"] = ""
	run(t, e, w, d.Start.Add(12*time.Hour))
	msgs, err := e.Store.Since(context.Background(), d.Name, time.Time{})
	if err != nil || len(msgs) != 1 || msgs[0].State != store.StateLost {
		t.Fatalf("messages = %+v, %v", msgs, err)
	}
	w.placement["a"] = mailbox.Inbox
	run(t, e, w, w.now.Add(25*time.Hour))
	m, err := e.Store.Get(context.Background(), msgs[0].ID)
	if err != nil || m.State != store.StateLate || m.Placement != "inbox" {
		t.Errorf("after the late recheck: %+v, %v", m, err)
	}
}

func TestBouncesHold(t *testing.T) {
	d := domain(t, "example.com", "a")
	e, w := setup(t, d)
	w.placement["a"] = ""
	ctx := context.Background()
	for range 3 {
		m, err := e.Send(ctx, d, d.Targets[0])
		if err != nil {
			t.Fatal(err)
		}
		w.now = w.now.Add(time.Minute)
		w.bounceDSN = append(w.bounceDSN, "Content-Type: multipart/report; report-type=delivery-status; boundary=b\r\n\r\n"+
			"--b\r\nContent-Type: message/delivery-status\r\n\r\nReporting-MTA: dns; x\r\n\r\nAction: delayed\r\nStatus: 4.7.0\r\n\r\n"+
			"--b\r\nContent-Type: text/rfc822-headers\r\n\r\nMessage-ID: <"+m.MessageID+">\r\n\r\n--b--\r\n")
	}
	// A newer message without a report yet must not hide the older ones.
	if _, err := e.Send(ctx, d, d.Targets[0]); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(e.PollBounces(ctx), e.PollBounces(ctx), e.EvaluateHolds(ctx, d)); err != nil {
		t.Fatal(err)
	}
	if r := ramp(t, e, d.Name, "a"); !strings.Contains(r.HeldReason, "4.7.0") {
		t.Errorf("hold reason = %q", r.HeldReason)
	}
}

func TestSubmissionFailuresHoldDomainAndTwoDomainsAreIndependent(t *testing.T) {
	broken, fine := domain(t, "broken.test", "a"), domain(t, "fine.test", "a")
	e, w := setup(t, broken, fine)
	ctx := context.Background()
	w.sendErr = errors.New("connection refused")
	for range 3 {
		if _, err := e.Send(ctx, broken, broken.Targets[0]); err == nil {
			t.Fatal("want error")
		}
		w.now = w.now.Add(time.Second)
	}
	w.sendErr = nil
	if err := errors.Join(e.EvaluateHolds(ctx, broken), e.EvaluateHolds(ctx, fine)); err != nil {
		t.Fatal(err)
	}
	if ramp(t, e, broken.Name, "").HeldReason == "" || ramp(t, e, fine.Name, "").HeldReason != "" {
		t.Fatal("only the failing domain must be held")
	}
	run(t, e, w, broken.Start.AddDate(0, 0, 5))
	if b, f := ramp(t, e, broken.Name, "").Day, ramp(t, e, fine.Name, "").Day; b != 1 || f != 6 {
		t.Errorf("ramp days: held %d, other %d; want 1 and 6", b, f)
	}
}

func TestMissedSlotsAreNotBurst(t *testing.T) {
	d := domain(t, "example.com", "a", "b")
	d.Schedule.RampDays, d.Schedule.StartPerDay = 1, 29
	e, w := setup(t, d)
	w.now = d.Start
	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.now = d.Start.Add(7 * time.Hour) // the service was down all day
	run(t, e, w, w.now.Add(10*time.Minute))
	if len(w.sent) > 2 {
		t.Errorf("sent %d messages in a burst after downtime", len(w.sent))
	}
}

func TestNoContentSendsNothing(t *testing.T) {
	d := domain(t, "example.com", "a")
	e, w := setup(t, d)
	w.contentOK = false
	run(t, e, w, d.Start.Add(12*time.Hour))
	msgs, err := e.Store.Since(context.Background(), "", time.Time{})
	if err != nil || len(msgs) != 0 || len(w.sent) != 0 {
		t.Errorf("messages = %d, sent = %d, %v", len(msgs), len(w.sent), err)
	}
}
