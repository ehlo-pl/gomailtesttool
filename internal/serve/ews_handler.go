package serve

import "net/http"

func (s *Server) handleEWSSendMail(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, apiResponse{Status: "error", Message: "EWS sendmail is not yet implemented"})
}
