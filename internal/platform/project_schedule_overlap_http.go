package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) projectScheduleOverlapHTTP(w http.ResponseWriter, r *http.Request, actor Actor, path string) bool {
	if path != "project-portfolio/schedule-overlaps" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if actor.kind != actorBrowser {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
		return true
	}
	if len(r.URL.RawQuery) > 128 {
		writeError(w, invalid(projectScheduleOverlapInvalid))
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["week"]) != 1 {
		writeError(w, invalid(projectScheduleOverlapInvalid))
		return true
	}
	out, err := s.Service.ProjectScheduleOverlaps(r.Context(), actor, query.Get("week"))
	respond(w, http.StatusOK, out, err)
	return true
}
