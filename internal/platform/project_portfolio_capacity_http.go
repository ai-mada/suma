package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) projectPortfolioCapacityHTTP(w http.ResponseWriter, r *http.Request, actor Actor) bool {
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
		writeError(w, invalid(projectPortfolioCapacityInvalid))
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["week"]) != 1 {
		writeError(w, invalid(projectPortfolioCapacityInvalid))
		return true
	}
	out, err := s.Service.ProjectPortfolioCapacity(r.Context(), actor, query.Get("week"))
	respond(w, http.StatusOK, out, err)
	return true
}
