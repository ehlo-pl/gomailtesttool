// Package engine runs the warm-up of every configured domain: it plans each
// day, sends at the planned times, finds the messages in the target
// mailboxes, engages with them, reads bounces and holds a ramp that goes
// wrong.
//
// Message states:
//
//	pending -> delivered | late | lost (-> late)   (submission accepted)
//	pending -> send_failed                         (submission rejected)
package engine

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/content"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/inspect"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// Opener opens protocol sessions for named endpoints.
type Opener interface {
	Sender(ctx context.Context, endpoint string) (send.Sender, error)
	Mailbox(ctx context.Context, endpoint string) (mailbox.Session, error)
	Fetcher(ctx context.Context, endpoint string) (fetch.Session, error)
}

// checkDelays are the waits before each look for a sent message.
var checkDelays = []time.Duration{5 * time.Minute, 150 * time.Second, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute}

const (
	tick         = 30 * time.Second
	slotGrace    = 15 * time.Minute // a slot older than this is missed, never sent late
	contentRetry = 2 * time.Minute
	checkRetry   = 5 * time.Minute
	engageRetry  = time.Hour
	giveUpAfter  = 72 * time.Hour
	lostRecheck  = 24 * time.Hour
	bouncePoll   = 10 * time.Minute
	bounceWindow = 7 * 24 * time.Hour
)

// Engine runs the domains of a configuration.
type Engine struct {
	Config  *config.Config
	Store   *store.Store
	Open    Opener
	Content content.Source
	Log     *slog.Logger
	Now     func() time.Time
	Rand    *rand.Rand

	contentWait map[string]time.Time // domain -> next content fetch
	nextBounce  time.Time
}

// New returns an engine with defaults applied.
func New(cfg *config.Config, st *store.Store, open Opener, log *slog.Logger) *Engine {
	return &Engine{
		Config: cfg, Store: st, Open: open, Content: content.Web{}, Log: log, Now: time.Now,
		Rand:        rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		contentWait: map[string]time.Time{},
	}
}

// Init closes messages that were stored but never submitted.
func (e *Engine) Init(ctx context.Context) error {
	unsent, err := e.Store.Unsent(ctx)
	if err != nil {
		return err
	}
	for _, m := range unsent {
		m.State, m.SendError, m.FinishedAt = store.StateSendFailed, "interrupted before the message was submitted", e.Now()
		if err := e.Store.Update(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Run ticks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	if err := e.Init(ctx); err != nil {
		return err
	}
	for {
		if err := e.Tick(ctx); err != nil && ctx.Err() == nil {
			e.Log.Error("cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(tick):
		}
	}
}

// Tick performs everything that is due. Errors returned are storage errors;
// mail server errors are logged and recorded.
func (e *Engine) Tick(ctx context.Context) error {
	var errs []error
	for _, d := range e.Config.Domains {
		if !d.Enabled {
			continue
		}
		if err := e.planDay(ctx, d); err != nil {
			errs = append(errs, fmt.Errorf("%s: plan: %w", d.Name, err))
			continue
		}
		if err := e.sendDue(ctx, d); err != nil {
			errs = append(errs, fmt.Errorf("%s: send: %w", d.Name, err))
		}
	}
	// Collection continues for disabled and finished domains.
	errs = append(errs, e.CheckDue(ctx), e.EngageDue(ctx))
	if now := e.Now(); !now.Before(e.nextBounce) {
		e.nextBounce = now.Add(bouncePoll)
		errs = append(errs, e.PollBounces(ctx))
	}
	for _, d := range e.Config.Domains {
		errs = append(errs, e.EvaluateHolds(ctx, d))
	}
	return errors.Join(errs...)
}

// ------------------------------------------------------------------- plan

// planDay creates the send times of the current date once, advancing the
// ramp day of everything that is not held.
func (e *Engine) planDay(ctx context.Context, d *config.Domain) error {
	now := e.Now().In(d.Location)
	today := now.Format("2006-01-02")
	if today < d.Start.Format("2006-01-02") || (!d.End.IsZero() && !now.Before(d.End)) {
		return nil
	}
	r, err := e.Store.DomainRamp(ctx, d.Name)
	if err != nil || r.LastAdvanced == today {
		return err
	}
	held := r.HeldReason != ""
	if !held || r.Day == 0 {
		r.Day++
	}
	r.LastAdvanced = today

	business, other := d.Schedule.Quota(r.Day, now, e.Rand)
	var times []time.Time
	dropped := 0
	for _, at := range d.Schedule.Slots(now, business, other, e.Rand) {
		if at.Before(d.Start) || (!d.End.IsZero() && !at.Before(d.End)) || at.Before(now.Add(-slotGrace)) {
			dropped++
			continue
		}
		times = append(times, at)
	}

	targets := make([]store.Ramp, len(d.Targets))
	for i, t := range d.Targets {
		if targets[i], err = e.Store.TargetRamp(ctx, d.Name, t.Endpoint); err != nil {
			return err
		}
		if targets[i].Day == 0 || (!held && targets[i].HeldReason == "") {
			targets[i].Day++
		}
	}
	if err := e.Store.AddSlots(ctx, d.Name, today, times, r); err != nil {
		return err
	}
	for i, t := range d.Targets {
		if err := e.Store.SaveTargetRamp(ctx, d.Name, t.Endpoint, targets[i]); err != nil {
			return err
		}
	}
	e.Log.Info("day planned", "domain", d.Name, "date", today, "ramp_day", r.Day, "messages", len(times),
		"outside_window", dropped, "held", held)
	return nil
}

// ------------------------------------------------------------------- send

func (e *Engine) sendDue(ctx context.Context, d *config.Domain) error {
	now := e.Now()
	slots, err := e.Store.DueSlots(ctx, d.Name, now)
	if err != nil {
		return err
	}
	for _, sl := range slots {
		if now.Sub(sl.At) > slotGrace {
			e.Log.Warn("send time missed", "domain", d.Name, "planned", sl.At.In(d.Location).Format("15:04"))
			if err := e.Store.SetSlot(ctx, sl.ID, store.SlotMissed); err != nil {
				return err
			}
			continue
		}
		if now.Before(e.contentWait[d.Name]) {
			return nil
		}
		target, ok, err := e.pickTarget(ctx, d, now)
		if err != nil {
			return err
		}
		if !ok {
			if err := e.Store.SetSlot(ctx, sl.ID, store.SlotSkipped); err != nil {
				return err
			}
			continue
		}
		m, err := e.Send(ctx, d, target)
		status := store.SlotSent
		switch {
		case m == nil && err != nil: // no content: keep the slot and retry
			e.contentWait[d.Name] = now.Add(contentRetry)
			e.Log.Warn("message not prepared; will retry", "domain", d.Name, "error", err)
			return nil
		case err != nil:
			status = store.SlotFailed
		}
		return e.Store.SetSlot(ctx, sl.ID, status) // one message per domain and tick
	}
	return nil
}

// pickTarget chooses the target with the fewest messages today among those
// below their share. A held target keeps the share of its held ramp day.
func (e *Engine) pickTarget(ctx context.Context, d *config.Domain, now time.Time) (config.Target, bool, error) {
	today, total, err := e.Store.DayCounts(ctx, d.Name, now.In(d.Location).Format("2006-01-02"))
	if err != nil {
		return config.Target{}, false, err
	}
	var eligible []config.Target
	for _, t := range d.Targets {
		r, err := e.Store.TargetRamp(ctx, d.Name, t.Endpoint)
		if err != nil {
			return config.Target{}, false, err
		}
		share := (d.Schedule.Total(r.Day) + len(d.Targets) - 1) / len(d.Targets)
		if today[t.Endpoint] < share {
			eligible = append(eligible, t)
		}
	}
	if len(eligible) == 0 {
		return config.Target{}, false, nil
	}
	best := slices.MinFunc(eligible, func(a, b config.Target) int {
		if c := today[a.Endpoint] - today[b.Endpoint]; c != 0 {
			return c
		}
		return total[a.Endpoint] - total[b.Endpoint]
	})
	return best, true, nil
}

// Send sends one message to target now. A nil message with an error means
// nothing was submitted or recorded.
func (e *Engine) Send(ctx context.Context, d *config.Domain, target config.Target) (*store.Message, error) {
	used, err := e.Store.UsedContent(ctx)
	if err != nil {
		return nil, err
	}
	item, err := e.Content.Next(ctx, d.ContentSources, d.Categories, used, e.Rand)
	if err != nil {
		return nil, fmt.Errorf("fetch content: %w", err)
	}
	id := make([]byte, 16)
	if _, err := crand.Read(id); err != nil {
		return nil, err
	}
	_, idDomain, _ := strings.Cut(d.FromAddress, "@")
	now := e.Now()
	messageID := hex.EncodeToString(id) + "@" + idDomain
	raw, err := content.Build(content.Message{From: d.From, To: target.Address, MessageID: messageID,
		Date: now.In(d.Location), Item: item})
	if err != nil {
		return nil, fmt.Errorf("build message: %w", err)
	}
	r, err := e.Store.DomainRamp(ctx, d.Name)
	if err != nil {
		return nil, err
	}
	m := &store.Message{Domain: d.Name, Target: target.Endpoint, Day: now.In(d.Location).Format("2006-01-02"), RampDay: r.Day,
		MessageID: messageID, Recipient: target.Address, Subject: item.Subject, ContentKey: item.Key, CreatedAt: now}
	if err := e.Store.Insert(ctx, m); err != nil {
		return nil, err
	}

	res, sendErr := e.submit(ctx, d, send.Message{From: d.EnvelopeFrom(), To: []string{target.Address}, Raw: raw})
	now = e.Now()
	if sendErr != nil {
		m.State, m.SendError, m.FinishedAt = store.StateSendFailed, sendErr.Error(), now
		e.Log.Error("send failed", "domain", d.Name, "target", target.Endpoint, "error", sendErr)
	} else {
		m.SentAt, m.SendResponse, m.NextCheckAt = now, res.ServerResponse, now.Add(checkDelays[0])
		e.Log.Info("sent", "domain", d.Name, "target", target.Endpoint, "message_id", messageID, "ramp_day", m.RampDay,
			"host", res.RemoteHost, "tls", res.TLSVersion)
	}
	if err := e.Store.Update(ctx, m); err != nil {
		return m, errors.Join(sendErr, err)
	}
	return m, sendErr
}

// submit makes a single attempt: retrying a submission can send a message
// twice, which would break the planned volume.
func (e *Engine) submit(ctx context.Context, d *config.Domain, m send.Message) (send.Result, error) {
	s, err := e.Open.Sender(ctx, d.SenderEndpoint)
	if err != nil {
		return send.Result{}, err
	}
	defer func() { _ = s.Close() }()
	return s.Send(ctx, m)
}

// ------------------------------------------------------------------ check

func byTarget(list []*store.Message) (names []string, groups map[string][]*store.Message) {
	groups = map[string][]*store.Message{}
	for _, m := range list {
		if _, ok := groups[m.Target]; !ok {
			names = append(names, m.Target)
		}
		groups[m.Target] = append(groups[m.Target], m)
	}
	return names, groups
}

// CheckDue looks for every message whose check time has come, with one
// session per target mailbox.
func (e *Engine) CheckDue(ctx context.Context) error {
	due, err := e.Store.DueChecks(ctx, e.Now())
	if err != nil {
		return err
	}
	names, groups := byTarget(due)
	for _, name := range names {
		sess, openErr := e.Open.Mailbox(ctx, name)
		if openErr != nil {
			e.Log.Error("mailbox check failed", "target", name, "error", openErr)
		}
		for _, m := range groups[name] {
			if err := e.check(ctx, sess, openErr, m); err != nil {
				return err
			}
		}
		if sess != nil {
			_ = sess.Close()
		}
	}
	return nil
}

func (e *Engine) check(ctx context.Context, sess mailbox.Session, openErr error, m *store.Message) error {
	var found *mailbox.Message
	err := openErr
	if err == nil {
		found, err = sess.Locate(ctx, m.MessageID)
	}
	now := e.Now()
	switch {
	case err != nil:
		// An unreadable mailbox says nothing about the mail.
		m.CheckError = err.Error()
		wait := checkRetry
		if now.Sub(m.SentAt) > lostRecheck {
			wait = time.Hour
		}
		m.NextCheckAt = now.Add(wait)
		if now.Sub(m.SentAt) > giveUpAfter {
			m.NextCheckAt = time.Time{}
		}
	case found != nil:
		e.arrived(m, found, now)
	case m.State == store.StateLost:
		m.NextCheckAt, m.CheckError = time.Time{}, "" // the late recheck found nothing either
	default:
		m.CheckError = ""
		var offset time.Duration
		next := time.Time{}
		for i, delay := range checkDelays {
			offset += delay
			if i > m.Attempts && m.SentAt.Add(offset).After(now) {
				next = m.SentAt.Add(offset)
				m.Attempts = i
				break
			}
		}
		if next.IsZero() {
			m.Attempts = len(checkDelays)
			m.State, m.FinishedAt, m.NextCheckAt = store.StateLost, now, now.Add(lostRecheck)
			e.Log.Warn("message lost", "domain", m.Domain, "target", m.Target, "message_id", m.MessageID)
		} else {
			m.NextCheckAt = next
		}
	}
	return e.Store.Update(ctx, m)
}

// arrived records the first observed placement, before anything is moved.
func (e *Engine) arrived(m *store.Message, found *mailbox.Message, now time.Time) {
	h := inspect.ParseHeaders(found.Header)
	at := h.Arrived
	if at.IsZero() {
		at = found.Received
	}
	if at.IsZero() {
		at = now
	}
	d := e.Config.Domain(m.Domain)
	warn := 5 * time.Minute
	if d != nil {
		warn = d.WarnAfter
	}
	wasLost := m.State == store.StateLost
	m.Latency = max(at.Sub(m.SentAt), 0)
	m.State = store.StateDelivered
	if wasLost || m.Latency > warn {
		m.State = store.StateLate
	}
	m.FoundAt, m.FinishedAt, m.NextCheckAt, m.CheckError = now, now, time.Time{}, ""
	m.Placement, m.Folder = string(found.Placement), found.Folder
	m.Auth, m.SpamInfo, m.Headers = h.Auth, h.Spam, string(found.Header)
	if d != nil {
		if t, ok := d.Target(m.Target); ok && t.Engage {
			wait := d.Engage.MinDelay
			if span := d.Engage.MaxDelay - d.Engage.MinDelay; span > 0 {
				wait += time.Duration(e.Rand.Int64N(int64(span)))
			}
			m.EngageAt = now.Add(wait)
		}
	}
	e.Log.Info("arrived", "domain", m.Domain, "target", m.Target, "placement", m.Placement, "latency", m.Latency.Round(time.Second),
		"auth", m.Auth)
}

// ----------------------------------------------------------------- engage

// EngageDue marks due messages read and moves them out of junk.
func (e *Engine) EngageDue(ctx context.Context) error {
	due, err := e.Store.DueEngagements(ctx, e.Now())
	if err != nil {
		return err
	}
	names, groups := byTarget(due)
	for _, name := range names {
		sess, openErr := e.Open.Mailbox(ctx, name)
		for _, m := range groups[name] {
			actErr := openErr
			if actErr == nil {
				actErr = sess.MarkRead(ctx, m.MessageID)
			}
			if actErr == nil {
				actErr = sess.Rescue(ctx, m.MessageID)
			}
			now := e.Now()
			switch {
			case actErr == nil:
				m.EngagedAt, m.CheckError = now, ""
				e.Log.Info("marked read", "domain", m.Domain, "target", name, "rescued", m.Placement == string(mailbox.Junk))
			case now.Sub(m.FoundAt) > giveUpAfter:
				m.EngagedAt, m.CheckError = now, "engagement abandoned: "+actErr.Error()
			default:
				m.EngageAt, m.CheckError = now.Add(engageRetry), "engagement: "+actErr.Error()
				e.Log.Warn("engagement failed", "target", name, "error", actErr)
			}
			if err := e.Store.Update(ctx, m); err != nil {
				return err
			}
		}
		if sess != nil {
			_ = sess.Close()
		}
	}
	return nil
}

// ---------------------------------------------------------------- bounces

// PollBounces reads delivery status notifications from the bounce mailboxes.
// Nothing is deleted there.
func (e *Engine) PollBounces(ctx context.Context) error {
	done := map[string]bool{}
	var errs []error
	for _, d := range e.Config.Domains {
		name := d.BounceEndpoint
		if name == "" || done[name] {
			continue
		}
		done[name] = true
		if err := e.pollBounces(ctx, name); err != nil {
			e.Log.Error("reading the bounce mailbox failed", "endpoint", name, "error", err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) pollBounces(ctx context.Context, name string) error {
	sess, err := e.Open.Fetcher(ctx, name)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()
	crit := fetch.Criteria{
		Since: e.Now().Add(-bounceWindow),
		SkipKey: func(key string) bool {
			seen, err := e.Store.IsSeen(ctx, name, key)
			return err == nil && seen
		},
	}
	return sess.Fetch(ctx, crit, func(ctx context.Context, raw fetch.RawMessage) error {
		if report, ok := inspect.ParseDSN(raw.Raw); ok {
			for _, id := range report.MessageIDs {
				m, err := e.Store.ByMessageID(ctx, id)
				if err != nil {
					return err
				}
				if m == nil {
					continue
				}
				if err := e.Store.AddBounce(ctx, store.Bounce{MessageID: id, At: e.Now(), Action: report.Action,
					Status: report.Status, RemoteMTA: report.RemoteMTA, Diagnostic: report.Diagnostic}); err != nil {
					return err
				}
				e.Log.Warn("delivery report", "domain", m.Domain, "target", m.Target, "action", report.Action,
					"status", report.Status, "diagnostic", report.Diagnostic)
			}
		}
		return e.Store.MarkSeen(ctx, name, raw.Key, e.Now())
	})
}

// ------------------------------------------------------------------ holds

// EvaluateHolds holds targets with bad results, and the whole domain when
// submissions keep failing.
func (e *Engine) EvaluateHolds(ctx context.Context, d *config.Domain) error {
	r, err := e.Store.DomainRamp(ctx, d.Name)
	if err != nil {
		return err
	}
	if r.HeldReason == "" {
		last, err := e.Store.LastSent(ctx, d.Name, "", r.ResumedAt, d.Hold.SendFailures)
		if err != nil {
			return err
		}
		if len(last) == d.Hold.SendFailures && !slices.ContainsFunc(last, func(m *store.Message) bool { return m.State != store.StateSendFailed }) {
			r.HeldReason = fmt.Sprintf("%d submissions in a row failed: %s", len(last), last[0].SendError)
			e.Log.Error("domain held", "domain", d.Name, "reason", r.HeldReason)
			if err := e.Store.SaveDomainRamp(ctx, d.Name, r); err != nil {
				return err
			}
		}
	}
	for _, t := range d.Targets {
		tr, err := e.Store.TargetRamp(ctx, d.Name, t.Endpoint)
		if err != nil {
			return err
		}
		if tr.HeldReason != "" {
			continue
		}
		reason, err := e.holdReason(ctx, d, t.Endpoint, tr.ResumedAt)
		if err != nil {
			return err
		}
		if reason == "" {
			continue
		}
		tr.HeldReason = reason
		e.Log.Error("target held", "domain", d.Name, "target", t.Endpoint, "reason", reason)
		if err := e.Store.SaveTargetRamp(ctx, d.Name, t.Endpoint, tr); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) holdReason(ctx context.Context, d *config.Domain, target string, since time.Time) (string, error) {
	resolved, err := e.Store.Resolved(ctx, d.Name, target, since, d.Hold.Window)
	if err != nil {
		return "", err
	}
	if n := len(resolved); n >= d.Hold.MinSamples {
		bad, late := 0, 0
		for _, m := range resolved {
			if m.State == store.StateLost || m.Placement == string(mailbox.Junk) {
				bad++
			}
			if m.State == store.StateLate {
				late++
			}
		}
		switch {
		case float64(bad)/float64(n) > d.Hold.BadRatio:
			return fmt.Sprintf("%d of the last %d messages went to junk or were lost", bad, n), nil
		case float64(late)/float64(n) > d.Hold.LateRatio:
			return fmt.Sprintf("%d of the last %d messages were late", late, n), nil
		}
	}
	// Deferral reports arrive hours late, so the newest messages usually have
	// none yet: count reports within the window rather than a run of them.
	last, err := e.Store.LastSent(ctx, d.Name, target, since, max(d.Hold.Window, d.Hold.DSN))
	if err != nil {
		return "", err
	}
	reported := 0
	var latest store.Bounce
	for _, m := range last {
		reports, err := e.Store.Bounces(ctx, m.MessageID)
		if err != nil {
			return "", err
		}
		if len(reports) == 0 {
			continue
		}
		if reported == 0 {
			latest = reports[len(reports)-1]
		}
		reported++
	}
	if reported < d.Hold.DSN {
		return "", nil
	}
	return fmt.Sprintf("%d of the last %d messages bounced or were deferred (%s %s)", reported, len(last), latest.Status, latest.Diagnostic), nil
}

// Resume clears holds of a domain: of one target, or of everything when
// target is "". Only results after the resume count again.
func (e *Engine) Resume(ctx context.Context, d *config.Domain, target string) error {
	now := e.Now()
	if target == "" {
		r, err := e.Store.DomainRamp(ctx, d.Name)
		if err != nil {
			return err
		}
		r.HeldReason, r.ResumedAt = "", now
		if err := e.Store.SaveDomainRamp(ctx, d.Name, r); err != nil {
			return err
		}
	}
	for _, t := range d.Targets {
		if target != "" && !strings.EqualFold(target, t.Endpoint) {
			continue
		}
		r, err := e.Store.TargetRamp(ctx, d.Name, t.Endpoint)
		if err != nil {
			return err
		}
		r.HeldReason, r.ResumedAt = "", now
		if err := e.Store.SaveTargetRamp(ctx, d.Name, t.Endpoint, r); err != nil {
			return err
		}
	}
	return nil
}

// Follow performs due checks until none of the message is pending (used by
// "once").
func (e *Engine) Follow(ctx context.Context, id int64) (*store.Message, error) {
	for {
		m, err := e.Store.Get(ctx, id)
		if err != nil || m == nil || m.State != store.StatePending {
			return m, err
		}
		select {
		case <-ctx.Done():
			return m, ctx.Err()
		case <-time.After(max(m.NextCheckAt.Sub(e.Now()), time.Second)):
		}
		if err := e.CheckDue(ctx); err != nil {
			return m, err
		}
	}
}
