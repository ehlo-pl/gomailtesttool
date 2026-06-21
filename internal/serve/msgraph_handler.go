package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
)

// msgraphSendRequest is the JSON body for POST /msgraph/sendmail.
// Attachments are intentionally omitted: accepting raw filesystem paths from HTTP
// clients would allow any API-key holder to read arbitrary server-side files.
type msgraphSendRequest struct {
	To       []string `json:"to"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body,omitempty"`
	BodyHTML string   `json:"bodyHTML,omitempty"`
	Priority string   `json:"priority,omitempty"` // high, normal, low (default: normal)
}

func (s *Server) handleMsgraphSendMail(w http.ResponseWriter, r *http.Request) {
	if s.msgraphBase == nil {
		writeJSON(w, http.StatusServiceUnavailable, apiResponse{Status: "error", Message: "Microsoft Graph not configured (set MSGRAPHTENANTID, MSGRAPHCLIENTID and auth env vars)"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req msgraphSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid JSON: " + err.Error()})
		return
	}

	if len(req.To) == 0 && len(req.Cc) == 0 && len(req.Bcc) == 0 {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "at least one recipient is required (to, cc, or bcc)"})
		return
	}
	if req.Subject == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "subject is required"})
		return
	}
	for _, list := range [][]string{req.To, req.Cc, req.Bcc} {
		for _, addr := range list {
			if err := validation.ValidateEmail(addr); err != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid recipient address " + addr + ": " + err.Error()})
				return
			}
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

	// Validation passed — now require a live client.
	if s.graphClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, apiResponse{Status: "error", Message: "Microsoft Graph client not initialised (check auth credentials)"})
		return
	}

	// Clone base config for runtime settings (VerboseMode, retries, mailbox, etc.)
	// Recipients are passed directly to SendEmail — no need to set them on the config.
	cfg := *s.msgraphBase
	cfg.Action = msgraph.ActionSendMail
	if req.Priority != "" {
		cfg.Priority = strings.ToLower(req.Priority)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	csvLogger, err := logger.NewLogger(logger.LogFormatCSV, "servetool", "msgraph-sendmail")
	if err != nil {
		s.logger.Warn("Could not initialise CSV logger for Graph send", "error", err)
	} else {
		defer csvLogger.Close()
	}

	if err := msgraph.SendEmail(ctx, s.graphClient, cfg.Mailbox, req.To, req.Cc, req.Bcc, req.Subject, req.Body, req.BodyHTML, nil, &cfg, csvLogger); err != nil {
		s.logger.Error("Graph sendmail failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, apiResponse{Status: "error", Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Status: "ok"})
}
