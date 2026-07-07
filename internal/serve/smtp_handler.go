package serve

import (
	"encoding/json"
	"net/http"
)

// smtpSendRequest is the JSON body for POST /smtp/sendmail and the argument
// schema for the smtp_sendmail MCP tool (the SDK infers the input schema from
// these json tags).
type smtpSendRequest struct {
	To       []string `json:"to"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	From     string   `json:"from,omitempty"` // optional override for SMTPFROM
	Subject  string   `json:"subject"`
	Body     string   `json:"body,omitempty"`
	Priority string   `json:"priority,omitempty"` // high, normal, low (default: normal)
}

func (s *Server) handleSMTPSendMail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req smtpSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid JSON: " + err.Error()})
		return
	}

	csvLogger := newServeCSVLogger(s.logger, "smtp-sendmail")
	if csvLogger != nil {
		defer func() { _ = csvLogger.Close() }()
	}

	if se := s.sendSMTP(r.Context(), req, csvLogger); se != nil {
		writeSendError(w, se)
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Status: "ok"})
}
