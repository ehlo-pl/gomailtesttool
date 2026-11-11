// Package engine sends probe messages and checks their arrival on the
// configured schedule. It owns the probe state machine:
//
//	pending -> delivered | late | lost        (send accepted)
//	pending -> send_failed                    (send rejected)
//
// A mailbox that cannot be checked never turns a probe into "lost": the
// check attempt is retried and the checker outage is reported on its own.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/eventlog"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/probe"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/common/retry"
	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// Opener opens protocol sessions for named endpoints.
type Opener interface {
	Sender(ctx context.Context, endpoint string) (send.Sender, error)
	Fetcher(ctx context.Context, endpoint string, withDelete bool) (fetch.Session, error)
}

const (
	// checkRetryDelay is the wait before re-checking after a checker failure.
	checkRetryDelay = time.Minute
	// sinceMargin widens the fetch window before the oldest pending probe
	// to tolerate clock differences between this host and the mail server.
	sinceMargin = 15 * time.Minute
	// maxIdle bounds the sleep of the run loop.
	maxIdle = time.Minute
)

// Engine runs the flows of a plan.
type Engine struct {
	Plan      *config.Plan
	Store     *store.Store
	Open      Opener
	Log       *slog.Logger
	Events    eventlog.Sink
	Now       func() time.Time
	Retention time.Duration
	// SendRetries / RetryDelay control retries of transient send errors.
	SendRetries int
	RetryDelay  time.Duration

	health      map[string]string // flow -> store.Health*
	checkerDown map[string]bool   // check endpoint -> currently failing
	senders     map[string]send.Sender
}

// New returns an engine with defaults applied.
func New(plan *config.Plan, st *store.Store, open Opener, log *slog.Logger, events eventlog.Sink) *Engine {
	if events == nil {
		events = eventlog.Nop{}
	}
	return &Engine{
		Plan: plan, Store: st, Open: open, Log: log, Events: events,
		Now: time.Now, Retention: config.DefaultRetention, SendRetries: 2, RetryDelay: 2 * time.Second,
		health: map[string]string{}, checkerDown: map[string]bool{}, senders: map[string]send.Sender{},
	}
}

// Init restores state after a restart: flow health comes from the latest
// finished probes (no events are raised for it), probes that were stored
// but never submitted are closed, and probes of removed flows are dropped.
func (e *Engine) Init(ctx context.Context) error {
	now := e.Now()
	unsent, err := e.Store.UnsentProbes(ctx)
	if err != nil {
		return err
	}
	for _, p := range unsent {
		if err := e.Store.MarkSendFailed(ctx, p.ID, now, "interrupted before the message was submitted"); err != nil {
			return err
		}
	}
	pending, err := e.Store.PendingProbes(ctx)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if e.Plan.Flow(p.Flow) == nil {
			e.Log.Warn("pending probe of a flow that is no longer configured; closing as lost", "flow", p.Flow, "probe", p.ID)
			if err := e.Store.MarkLost(ctx, p.ID, p.Attempts, now); err != nil {
				return err
			}
		}
	}
	for _, f := range e.Plan.Flows {
		last, err := e.Store.LastProbe(ctx, f.Name, true)
		if err != nil {
			return err
		}
		e.health[f.Name] = store.HealthUnknown
		if last != nil {
			e.health[f.Name] = store.HealthOf(last.State)
		}
	}
	return nil
}

// Close releases cached senders.
func (e *Engine) Close() {
	for name, s := range e.senders {
		_ = s.Close()
		delete(e.senders, name)
	}
}

// ------------------------------------------------------------------- send

// SendProbe sends one probe for the flow. The probe row is stored before
// submission, so a crash never leaves an untracked message in flight.
// A rejected submission is recorded as send_failed and returned as error.
func (e *Engine) SendProbe(ctx context.Context, f *config.Flow) (*store.Probe, error) {
	token, err := probe.NewToken()
	if err != nil {
		return nil, err
	}
	now := e.Now()
	raw, msgID, err := probe.Build(f.Name, token, f.From, f.To, now)
	if err != nil {
		return nil, fmt.Errorf("build probe: %w", err)
	}
	p := &store.Probe{Flow: f.Name, Token: token, MessageID: msgID, CreatedAt: now}
	if err := e.Store.InsertProbe(ctx, p); err != nil {
		return nil, err
	}

	var res send.Result
	sendErr := retry.RetryWithBackoff(ctx, e.SendRetries, e.RetryDelay, func() error {
		s, err := e.sender(ctx, f.SendEndpoint)
		if err != nil {
			return err
		}
		res, err = s.Send(ctx, send.Message{From: f.From, To: f.To, Raw: raw})
		if err != nil {
			e.dropSender(f.SendEndpoint) // re-open (re-authenticate) next time
		}
		return err
	})
	now = e.Now()
	if sendErr != nil {
		if err := e.Store.MarkSendFailed(ctx, p.ID, now, sendErr.Error()); err != nil {
			return p, errors.Join(sendErr, err)
		}
		p.State, p.SendError = store.StateSendFailed, sendErr.Error()
		e.Log.Error("probe send failed", "flow", f.Name, "endpoint", f.SendEndpoint, "error", sendErr)
		e.setHealth(ctx, f.Name, p, fmt.Sprintf("sending via %s failed: %v", f.SendEndpoint, sendErr))
		return p, fmt.Errorf("flow %s: send via %s: %w", f.Name, f.SendEndpoint, sendErr)
	}
	next := now.Add(f.CheckOffset(0))
	if err := e.Store.MarkSent(ctx, p.ID, now, next, res.ServerResponse, res.RemoteHost, res.TLSVersion); err != nil {
		return p, err
	}
	p.SentAt, p.NextCheckAt = now, next
	e.Log.Info("probe sent", "flow", f.Name, "probe", p.ID, "via", f.SendEndpoint, "host", res.RemoteHost,
		"tls", res.TLSVersion, "first_check", next.Format(time.RFC3339))
	return p, nil
}

func (e *Engine) sender(ctx context.Context, endpoint string) (send.Sender, error) {
	if s, ok := e.senders[endpoint]; ok {
		return s, nil
	}
	s, err := e.Open.Sender(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	e.senders[endpoint] = s
	return s, nil
}

func (e *Engine) dropSender(endpoint string) {
	if s, ok := e.senders[endpoint]; ok {
		_ = s.Close()
		delete(e.senders, endpoint)
	}
}

// ------------------------------------------------------------------ check

type groupKey struct{ endpoint, folder string }

// CheckDue checks the mailboxes of all probes whose next check time has
// come. One session per mailbox/folder resolves every pending probe there.
func (e *Engine) CheckDue(ctx context.Context) error {
	pending, err := e.Store.PendingProbes(ctx)
	if err != nil {
		return err
	}
	now := e.Now()
	groups := map[groupKey][]*store.Probe{}
	due := map[groupKey]bool{}
	for _, p := range pending {
		f := e.Plan.Flow(p.Flow)
		if f == nil {
			continue
		}
		k := groupKey{f.CheckEndpoint, f.Folder}
		groups[k] = append(groups[k], p)
		if !p.NextCheckAt.After(now) {
			due[k] = true
		}
	}
	keys := make([]groupKey, 0, len(due))
	for k := range due {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].endpoint+"\x00"+keys[i].folder < keys[j].endpoint+"\x00"+keys[j].folder
	})
	var errs []error
	for _, k := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.checkGroup(ctx, k, groups[k]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// checkGroup fetches probe messages from one mailbox folder and updates
// the pending probes that are checked there. It returns only storage errors;
// mailbox errors are recorded on the probes.
func (e *Engine) checkGroup(ctx context.Context, k groupKey, pending []*store.Probe) error {
	withDelete := false
	oldest := pending[0].SentAt
	for _, p := range pending {
		if f := e.Plan.Flow(p.Flow); f != nil && f.Delete {
			withDelete = true
		}
		if p.SentAt.Before(oldest) {
			oldest = p.SentAt
		}
	}

	arrived := map[int64]bool{}
	var toDelete []string
	var storeErr error

	checkErr := func() error {
		sess, err := e.Open.Fetcher(ctx, k.endpoint, withDelete)
		if err != nil {
			return err
		}
		crit := fetch.Criteria{
			Folder:  k.folder,
			Since:   oldest.Add(-sinceMargin),
			Subject: probe.SubjectMarker,
			SkipKey: func(key string) bool {
				seen, err := e.Store.IsSeen(ctx, k.endpoint, key)
				return err == nil && seen
			},
		}
		fetchErr := sess.Fetch(ctx, crit, func(ctx context.Context, m fetch.RawMessage) error {
			del, err := e.handleMessage(ctx, k.endpoint, m, arrived)
			if err != nil {
				storeErr = err
				return err
			}
			if del {
				toDelete = append(toDelete, m.Key)
			}
			return nil
		})
		// Delete after Fetch has finished: deleting while paging would shift
		// unseen messages out of the result set.
		for _, key := range toDelete {
			if err := sess.Delete(ctx, key); err != nil {
				e.Log.Warn("deleting probe message failed", "endpoint", k.endpoint, "error", err)
			}
		}
		closeErr := sess.Close() // POP3 commits deletions here
		if fetchErr != nil {
			return fetchErr
		}
		if closeErr != nil && len(toDelete) > 0 {
			e.Log.Warn("closing mailbox session failed; deletions may not be committed", "endpoint", k.endpoint, "error", closeErr)
		}
		return nil
	}()
	if storeErr != nil {
		return storeErr
	}

	now := e.Now()
	if checkErr != nil {
		e.checkerFailed(ctx, k.endpoint, checkErr)
	} else {
		e.checkerOK(ctx, k.endpoint)
	}

	for _, p := range pending {
		if arrived[p.ID] || p.NextCheckAt.After(now) {
			continue
		}
		f := e.Plan.Flow(p.Flow)
		if checkErr != nil {
			// The attempt is not consumed: an unreachable mailbox says nothing about the mail.
			if err := e.Store.MarkCheckError(ctx, p.ID, checkErr.Error(), now.Add(checkRetryDelay)); err != nil {
				return err
			}
			continue
		}
		// This check covers every attempt scheduled up to now (after a
		// service outage several may have passed).
		attempts := p.Attempts + 1
		for attempts < len(f.Checks) && !p.SentAt.Add(f.CheckOffset(attempts)).After(now) {
			attempts++
		}
		if attempts >= len(f.Checks) {
			if err := e.Store.MarkLost(ctx, p.ID, attempts, now); err != nil {
				return err
			}
			p.State, p.Attempts = store.StateLost, attempts
			e.Log.Error("probe lost", "flow", p.Flow, "probe", p.ID, "sent", p.SentAt.Format(time.RFC3339), "checks", attempts)
			e.setHealth(ctx, p.Flow, p, fmt.Sprintf("probe sent %s did not arrive in %s within %s (%d checks)",
				p.SentAt.Format(time.RFC3339), k.endpoint, now.Sub(p.SentAt).Round(time.Second), attempts))
			continue
		}
		next := p.SentAt.Add(f.CheckOffset(attempts))
		if err := e.Store.MarkNotFound(ctx, p.ID, attempts, next); err != nil {
			return err
		}
		e.Log.Info("probe not yet arrived", "flow", p.Flow, "probe", p.ID, "attempt", attempts, "next_check", next.Format(time.RFC3339))
	}
	return nil
}

// handleMessage processes one fetched message and reports whether the
// server copy should be deleted.
func (e *Engine) handleMessage(ctx context.Context, endpoint string, m fetch.RawMessage, arrived map[int64]bool) (bool, error) {
	now := e.Now()
	if err := e.Store.MarkSeen(ctx, endpoint, m.Key, now); err != nil {
		return false, err
	}
	found, ok := probe.Match(m.Raw)
	if !ok {
		return false, nil
	}
	p, err := e.Store.ProbeByToken(ctx, found.Token)
	if err != nil {
		return false, err
	}
	if p == nil {
		// Probe of another testmailflow instance or database: leave it alone.
		return false, nil
	}
	f := e.Plan.Flow(p.Flow)
	del := f != nil && f.Delete
	if p.SentAt.IsZero() || (p.State != store.StatePending && p.State != store.StateLost) {
		return del, nil // duplicate delivery of an already resolved probe
	}

	at := found.Arrived
	if at.IsZero() {
		at = m.Received
	}
	if at.IsZero() {
		at = now
	}
	latency := at.Sub(p.SentAt)
	if latency < 0 {
		latency = 0
	}
	wasLost := p.State == store.StateLost
	state := store.StateDelivered
	if wasLost || (f != nil && latency > f.WarnAfter) {
		state = store.StateLate
	}
	attempts := p.Attempts
	if !wasLost {
		attempts++
	}
	if err := e.Store.MarkArrived(ctx, p.ID, state, at, now, latency, found.Hops, found.AuthResults, attempts); err != nil {
		return false, err
	}
	arrived[p.ID] = true
	p.State, p.ArrivedAt, p.Latency = state, at, latency
	e.Log.Info("probe arrived", "flow", p.Flow, "probe", p.ID, "state", state, "latency", latency.Round(time.Second),
		"hops", found.Hops, "auth", found.AuthResults)
	if wasLost {
		// Arrived after it was reported lost: keep the history accurate,
		// but the flow health follows newer probes.
		return del, nil
	}
	detail := fmt.Sprintf("probe arrived after %s", latency.Round(time.Second))
	if state == store.StateLate && f != nil {
		detail += fmt.Sprintf(" (threshold %s)", f.WarnAfter)
	}
	e.setHealth(ctx, p.Flow, p, detail)
	return del, nil
}

// ----------------------------------------------------------------- events

// setHealth updates the flow health from a finished probe and raises an
// event only when the health changes.
func (e *Engine) setHealth(ctx context.Context, flow string, p *store.Probe, detail string) {
	newH, oldH := store.HealthOf(p.State), e.health[flow]
	e.health[flow] = newH
	if newH == oldH {
		return
	}
	switch newH {
	case store.HealthOK:
		if oldH == store.HealthDegraded || oldH == store.HealthFailing {
			e.emit(ctx, eventlog.Info, eventlog.IDFlowRecovered, flow, "flow_recovered", detail)
		}
	case store.HealthDegraded:
		e.emit(ctx, eventlog.Warning, eventlog.IDProbeLate, flow, "probe_late", detail)
	case store.HealthFailing:
		if p.State == store.StateSendFailed {
			e.emit(ctx, eventlog.Error, eventlog.IDSendFailed, flow, "send_failed", detail)
		} else {
			e.emit(ctx, eventlog.Error, eventlog.IDProbeLost, flow, "probe_lost", detail)
		}
	}
}

func (e *Engine) checkerFailed(ctx context.Context, endpoint string, err error) {
	e.Log.Error("mailbox check failed", "endpoint", endpoint, "error", err)
	if e.checkerDown[endpoint] {
		return
	}
	e.checkerDown[endpoint] = true
	e.emit(ctx, eventlog.Error, eventlog.IDCheckerFailing, "", "checker_failing",
		fmt.Sprintf("cannot check mailbox endpoint %s: %v", endpoint, err))
}

func (e *Engine) checkerOK(ctx context.Context, endpoint string) {
	if !e.checkerDown[endpoint] {
		return
	}
	e.checkerDown[endpoint] = false
	e.emit(ctx, eventlog.Info, eventlog.IDCheckerRecovered, "", "checker_recovered",
		fmt.Sprintf("mailbox endpoint %s can be checked again", endpoint))
}

// emit writes an event to the OS event log and the events table.
func (e *Engine) emit(ctx context.Context, level eventlog.Level, id uint32, flow, kind, detail string) {
	msg := detail
	if flow != "" {
		msg = "Flow " + flow + ": " + detail
	}
	e.Events.Report(level, id, msg)
	if err := e.Store.AddEvent(ctx, store.Event{At: e.Now(), Flow: flow, Kind: kind, Detail: detail}); err != nil {
		e.Log.Warn("recording event failed", "error", err)
	}
}

// -------------------------------------------------------------------- run

// Run sends probes at each flow's interval and performs due checks until
// ctx is cancelled. Pending probes from a previous run are resumed.
func (e *Engine) Run(ctx context.Context) error {
	if err := e.Init(ctx); err != nil {
		return err
	}
	defer e.Close()

	start := e.Now()
	nextSend := map[string]time.Time{}
	for i, f := range e.Plan.Flows {
		nextSend[f.Name] = start.Add(time.Duration(i) * 2 * time.Second) // stagger the first round
	}
	nextPrune := start

	for {
		now := e.Now()
		for _, f := range e.Plan.Flows {
			if nextSend[f.Name].After(now) {
				continue
			}
			nextSend[f.Name] = now.Add(f.Interval)
			if _, err := e.SendProbe(ctx, f); err != nil && ctx.Err() != nil {
				return nil
			}
		}
		if err := e.CheckDue(ctx); err != nil && ctx.Err() == nil {
			e.Log.Error("check cycle failed", "error", err)
		}
		if !nextPrune.After(now) {
			nextPrune = now.Add(24 * time.Hour)
			if n, err := e.Store.Prune(ctx, now.Add(-e.Retention)); err != nil {
				e.Log.Warn("pruning old probes failed", "error", err)
			} else if n > 0 {
				e.Log.Info("pruned old probes", "count", n)
			}
		}

		wake := e.Now().Add(maxIdle)
		for _, t := range nextSend {
			if t.Before(wake) {
				wake = t
			}
		}
		if pending, err := e.Store.PendingProbes(ctx); err == nil {
			for _, p := range pending {
				if p.NextCheckAt.Before(wake) {
					wake = p.NextCheckAt
				}
			}
		}
		d := wake.Sub(e.Now())
		if d < time.Second {
			d = time.Second
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d):
		}
	}
}

// Follow performs due checks until none of the given probes is pending
// (used by "once --wait"). It returns the final probe rows.
func (e *Engine) Follow(ctx context.Context, ids []int64) ([]*store.Probe, error) {
	for {
		var out []*store.Probe
		wake := time.Time{}
		for _, id := range ids {
			p, err := e.Store.ProbeByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if p == nil {
				continue
			}
			out = append(out, p)
			if p.State == store.StatePending && (wake.IsZero() || p.NextCheckAt.Before(wake)) {
				wake = p.NextCheckAt
			}
		}
		if wake.IsZero() {
			return out, nil
		}
		d := wake.Sub(e.Now())
		if d < time.Second {
			d = time.Second
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(d):
		}
		if err := e.CheckDue(ctx); err != nil {
			return out, err
		}
	}
}
