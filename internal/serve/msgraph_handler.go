package serve

import (
	"encoding/json"
	"net/http"
)

// msgraphSendRequest is the JSON body for POST /msgraph/sendmail and the
// argument schema for the msgraph_sendmail MCP tool.
//
// Attachments are intentionally omitted: accepting raw filesystem paths from
// remote callers would allow any authenticated client to read arbitrary
// server-side files.
type msgraphSendRequest struct {
	// To uses omitempty so the inferred MCP input schema does not mark it
	// required: Graph accepts a cc/bcc-only send (the "at least one recipient"
	// rule is enforced in sendMsgraph). This does not affect HTTP decoding.
	To       []string `json:"to,omitempty"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body,omitempty"`
	BodyHTML string   `json:"bodyHTML,omitempty"`
	Priority string   `json:"priority,omitempty"` // high, normal, low (default: normal)
}

func (s *Server) handleMsgraphSendMail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req msgraphSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Status: "error", Message: "invalid JSON: " + err.Error()})
		return
	}

	csvLogger := newServeCSVLogger(s.logger, "msgraph-sendmail")
	if csvLogger != nil {
		defer func() { _ = csvLogger.Close() }()
	}

	if se := s.sendMsgraph(r.Context(), req, csvLogger); se != nil {
		writeSendError(w, se)
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Status: "ok"})
}
