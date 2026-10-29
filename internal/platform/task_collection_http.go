package platform

import "net/http"

func (s *Server) taskCollectionSummaryHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
		return
	}
	if !myWorkHuman(actor) {
		writeError(w, forbidden())
		return
	}
	q, options, err := parseTaskWorkspaceQuery(r, project, true)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.Service.TaskCollectionSummary(r.Context(), actor, project, q, options)
	respond(w, http.StatusOK, out, err)
}
