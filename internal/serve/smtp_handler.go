package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// smtpSendRequest is the JSON body for POST /smtp/sendmail.
type smtpSendRequest struct {
	To       []string `json:"to"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	From     string   `json:"from,omitempty"` // optional override for SMTPFROM
	Subject  string   `json:"subject"`
	Body     string   `json:"body,omitempty"`
	Priority string   `json:"priority,omitempty"` // high, normal, low (default: normal)
}

func sanitizeEmailSubjectInput(subject string) string {
	subject = strings.ReplaceAll(subject, "\r", "")
	subject = strings.ReplaceAll(subject, "\n", "")
	return strings.TrimSpace(subject)
}

func sanitizeEmailBodyInput(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	var b strings.Builder
	b.Grow(len(body))
	for _, r := range body {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Server) handleSMTPSendMail(w http.ResponseWriter, r *http.Request) {
	if s.smtpBase == nil {
		writeJSON(w, http.StatusServiceUnavailable, apiResponse{Status: "error", Message: "SMTP not configured (set SMTPHOST and related env vars)"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req smtpSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid JSON: " + err.Error()})
		return
	}

	if len(req.To) == 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "to is required"})
		return
	}
	if req.Subject == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "subject is required"})
		return
	}
	if req.From == "" && s.smtpBase.From == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "from is required (set SMTPFROM or provide 'from' in request body)"})
		return
	}
	if req.From != "" {
		if err := validation.ValidateEmail(req.From); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid from address: " + err.Error()})
			return
		}
	}
	for _, addr := range req.To {
		if err := validation.ValidateEmail(addr); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid to address " + addr + ": " + err.Error()})
			return
		}
	}
	for _, addr := range req.Cc {
		if err := validation.ValidateEmail(addr); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid cc address " + addr + ": " + err.Error()})
			return
		}
	}
	for _, addr := range req.Bcc {
		if err := validation.ValidateEmail(addr); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid bcc address " + addr + ": " + err.Error()})
			return
		}
	}
	if req.Priority != "" {
		switch strings.ToLower(req.Priority) {
		case "high", "normal", "low":
		default:
			writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid priority: " + req.Priority + " (must be one of: high, normal, low)"})
			return
		}
	}

	// Clone base config and overlay request content
	cfg := *s.smtpBase
	cfg.To = req.To
	cfg.Cc = req.Cc
	cfg.Bcc = req.Bcc
	cfg.Subject = sanitizeEmailSubjectInput(req.Subject)
	cfg.Body = sanitizeEmailBodyInput(req.Body)
	cfg.Action = smtp.ActionSendMail
	if req.From != "" {
		cfg.From = req.From
	}
	if req.Priority != "" {
		cfg.Priority = strings.ToLower(req.Priority)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	csvLogger, err := logger.NewLogger(logger.LogFormatCSV, "servetool", "smtp-sendmail")
	if err != nil {
		s.logger.Warn("Could not initialise CSV logger for SMTP send", "error", err)
	} else {
		defer csvLogger.Close()
	}

	if err := smtp.SendMail(ctx, &cfg, csvLogger, s.logger); err != nil {
		s.logger.Error("SMTP sendmail failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, apiResponse{Status: "error", Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Status: "ok"})
}
