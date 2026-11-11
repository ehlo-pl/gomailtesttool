// Package httpd serves the read-only monitoring endpoints of testmailflow:
// /healthz (JSON, 200 or 503) and /metrics (Prometheus text format).
package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/report"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

// Server is the monitoring HTTP server.
type Server struct {
	Store *store.Store
	Plan  *config.Plan
	Log   *slog.Logger
	Now   func() time.Time
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /metrics", s.metrics)
	return mux
}

// Serve listens on addr until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("http listen %s: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}
	s.Log.Info("http endpoint listening", "addr", ln.Addr().String())
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	statuses, err := report.Status(r.Context(), s.Store, s.Plan, s.now())
	if err != nil {
		http.Error(w, "status unavailable", http.StatusInternalServerError)
		s.Log.Error("healthz failed", "error", err)
		return
	}
	overall := "ok"
	code := http.StatusOK
	switch report.Worst(statuses) {
	case 1:
		overall = "degraded"
	case 2:
		overall = "failing"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  overall,
		"version": version.Get(),
		"time":    s.now().UTC().Format(time.RFC3339),
		"flows":   statuses,
	})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statuses, err := report.Status(ctx, s.Store, s.Plan, s.now())
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}
	counts, err := s.Store.CountsByState(ctx)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}
	var b strings.Builder
	b.WriteString("# HELP tmf_probes_total Probes stored per flow and state.\n# TYPE tmf_probes_total gauge\n")
	for _, c := range counts {
		fmt.Fprintf(&b, "tmf_probes_total{flow=%q,state=%q} %d\n", c.Flow, c.State, c.Count)
	}
	b.WriteString("# HELP tmf_flow_up 1 when the latest finished probe of the flow arrived in time.\n# TYPE tmf_flow_up gauge\n")
	for _, st := range statuses {
		up := 0
		if st.Health == store.HealthOK {
			up = 1
		}
		fmt.Fprintf(&b, "tmf_flow_up{flow=%q} %d\n", st.Flow, up)
	}
	b.WriteString("# HELP tmf_flow_health Flow health: 0 ok, 1 degraded or unknown, 2 failing or stale.\n# TYPE tmf_flow_health gauge\n")
	for _, st := range statuses {
		fmt.Fprintf(&b, "tmf_flow_health{flow=%q} %d\n", st.Flow, report.Worst([]report.FlowStatus{st}))
	}
	b.WriteString("# HELP tmf_pending_probes Probes sent and not yet found.\n# TYPE tmf_pending_probes gauge\n")
	for _, st := range statuses {
		fmt.Fprintf(&b, "tmf_pending_probes{flow=%q} %d\n", st.Flow, st.Pending)
	}
	b.WriteString("# HELP tmf_last_latency_seconds Delivery latency of the latest arrived probe.\n# TYPE tmf_last_latency_seconds gauge\n")
	for _, st := range statuses {
		if st.LatencySec != nil {
			fmt.Fprintf(&b, "tmf_last_latency_seconds{flow=%q} %.3f\n", st.Flow, *st.LatencySec)
		}
	}
	b.WriteString("# HELP tmf_last_success_timestamp_seconds Arrival time of the latest delivered probe.\n# TYPE tmf_last_success_timestamp_seconds gauge\n")
	for _, st := range statuses {
		if st.LastSuccess != nil {
			fmt.Fprintf(&b, "tmf_last_success_timestamp_seconds{flow=%q} %d\n", st.Flow, st.LastSuccess.Unix())
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
