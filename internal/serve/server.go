package serve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// Server holds shared state for the HTTP serve command.
type Server struct {
	config      *Config
	smtpBase    *smtp.Config                     // nil when SMTP env vars are absent
	msgraphBase *msgraph.Config                  // nil when MSGRAPH env vars are absent
	graphClient *msgraphsdk.GraphServiceClient   // nil when msgraphBase is nil
	logger      *slog.Logger
}

// New creates a Server. smtpBase and msgraphBase may be nil when the
// corresponding credentials were not configured at startup.
func New(cfg *Config, smtpBase *smtp.Config, msgraphBase *msgraph.Config, graphClient *msgraphsdk.GraphServiceClient, logger *slog.Logger) *Server {
	return &Server{
		config:      cfg,
		smtpBase:    smtpBase,
		msgraphBase: msgraphBase,
		graphClient: graphClient,
		logger:      logger,
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled or the server errors.
func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort(s.config.Listen, strconv.Itoa(s.config.Port))
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 70 * time.Second, // handler timeout is 60s; extra 10s for response flush
		IdleTimeout:  60 * time.Second, // keep-alive connections closed after 60s of inactivity
	}

	s.logger.Info("HTTP server listening", "addr", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// handler builds the fully-wrapped HTTP handler: the route mux behind the
// API-key middleware. It is separated from Run so tests can exercise route
// registration (http.ServeMux panics on conflicting patterns) without binding a
// socket.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleSummary)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /smtp/sendmail", s.handleSMTPSendMail)
	mux.HandleFunc("POST /msgraph/sendmail", s.handleMsgraphSendMail)
	mux.HandleFunc("POST /ews/sendmail", s.handleEWSSendMail)

	// Mount the MCP server over Streamable HTTP alongside the REST API. The
	// methods are registered explicitly (POST send, GET SSE stream, DELETE
	// session end) so the patterns are more specific than "GET /" and do not
	// conflict with it. A fresh mcp.Server is built per session; the handler is
	// wrapped by apiKeyMiddleware like every other route, so /mcp requires
	// X-API-Key too (MCP must not be an auth bypass).
	if s.config.EnableMCP {
		mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return s.newMCPServer()
		}, nil)
		mux.Handle("POST /mcp", mcpHandler)
		mux.Handle("GET /mcp", mcpHandler)
		mux.Handle("DELETE /mcp", mcpHandler)
	}

	return s.apiKeyMiddleware(mux)
}

// apiKeyMiddleware enforces X-API-Key on all routes except /health and GET /.
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exempt := r.URL.Path == "/health" || (r.URL.Path == "/" && r.Method == http.MethodGet)
		provided := r.Header.Get("X-API-Key")
		// Fail closed: an empty configured key must never authenticate a request,
		// even one sent without an X-API-Key header.
		if !exempt && (s.config.APIKey == "" || provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(s.config.APIKey)) != 1) {
			writeJSON(w, http.StatusUnauthorized, apiResponse{Status: "error", Message: "missing or invalid X-API-Key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Get()})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	type endpointInfo struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Description string `json:"description"`
		Available   bool   `json:"available"`
	}
	type summaryResponse struct {
		Name      string         `json:"name"`
		Version   string         `json:"version"`
		Endpoints []endpointInfo `json:"endpoints"`
	}
	writeJSON(w, http.StatusOK, summaryResponse{
		Name:    "gomailtest serve",
		Version: version.Get(),
		Endpoints: []endpointInfo{
			{Method: "GET", Path: "/health", Description: "Health check (no API key required)", Available: true},
			{Method: "POST", Path: "/smtp/sendmail", Description: "Send email via SMTP (X-API-Key required)", Available: s.smtpBase != nil},
			{Method: "POST", Path: "/msgraph/sendmail", Description: "Send email via Microsoft Graph (X-API-Key required)", Available: s.msgraphBase != nil && s.graphClient != nil},
			{Method: "POST", Path: "/ews/sendmail", Description: "Send email via EWS — not yet implemented", Available: false},
			{Method: "POST", Path: "/mcp", Description: "MCP (Streamable HTTP) endpoint exposing the sendmail tools (X-API-Key required)", Available: s.config.EnableMCP},
		},
	})
}

// writeSendError maps a classified send error to its HTTP status code and the
// standard JSON envelope.
func writeSendError(w http.ResponseWriter, se *sendError) {
	var code int
	switch se.kind {
	case sendErrBadRequest:
		code = http.StatusBadRequest
	case sendErrUnavailable:
		code = http.StatusServiceUnavailable
	default:
		code = http.StatusInternalServerError
	}
	writeJSON(w, code, apiResponse{Status: "error", Message: se.msg})
}

// newServeCSVLogger creates a per-request CSV result logger for the serve mode,
// logging a warning and returning nil (rather than failing the request) if it
// cannot be initialised. Callers must close a non-nil logger.
func newServeCSVLogger(slogger *slog.Logger, action string) logger.Logger {
	l, err := logger.NewLogger(logger.LogFormatCSV, "servetool", action)
	if err != nil {
		slogger.Warn("Could not initialise CSV logger", "action", action, "error", err)
		return nil
	}
	return l
}

// apiResponse is the standard JSON envelope for all endpoint responses.
type apiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
