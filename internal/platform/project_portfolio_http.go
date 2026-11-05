package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) projectPortfolioHTTP(w http.ResponseWriter, r *http.Request, actor Actor, path string) bool {
	if path == "project-portfolio/schedule-overlaps" {
		return s.projectScheduleOverlapHTTP(w, r, actor, path)
	}
	if path != "project-portfolio" {
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
	if len(r.URL.RawQuery) > 2048 {
		writeError(w, invalid(projectPortfolioInvalid))
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, invalid(projectPortfolioInvalid))
		return true
	}
	for key, values := range query {
		if len(values) != 1 || key != "q" && key != "limit" && key != "after" || key == "after" && values[0] == "" {
			writeError(w, invalid(projectPortfolioInvalid))
			return true
		}
	}
	limit := 20
	if query.Has("limit") {
		raw := query.Get("limit")
		limit, err = strconv.Atoi(raw)
		if err != nil || strconv.Itoa(limit) != raw {
			writeError(w, invalid(projectPortfolioInvalid))
			return true
		}
	}
	out, err := s.Service.ProjectPortfolio(r.Context(), actor, query.Get("q"), limit, query.Get("after"))
	respond(w, http.StatusOK, out, err)
	return true
}
