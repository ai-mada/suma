package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func attemptHistoryQuery(r *http.Request, attemptRaw string) (string, int, int, string, error) {
	bad := func() (string, int, int, string, error) {
		return "", 0, 0, "", invalid("工具历史请求参数无效")
	}
	if len(r.URL.RawQuery) > 8192 {
		return bad()
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return bad()
	}
	for key, values := range q {
		if key != "project_id" && key != "limit" && key != "cursor" || len(values) != 1 || values[0] == "" {
			return bad()
		}
	}
	project := q.Get("project_id")
	if project == "" || len(project) > 512 {
		return bad()
	}
	canonical := func(raw string, max int64) (int64, bool) {
		if raw == "" || len(raw) > 16 || raw[0] == '0' {
			return 0, false
		}
		for _, c := range raw {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		return n, err == nil && n > 0 && n <= max
	}
	attempt, ok := canonical(attemptRaw, maxStreamSequence)
	if !ok {
		return bad()
	}
	limit := int64(50)
	if raw := q.Get("limit"); raw != "" {
		limit, ok = canonical(raw, 100)
		if !ok {
			return bad()
		}
	}
	cursor := q.Get("cursor")
	if len(cursor) > 4096 {
		return bad()
	}
	return project, int(attempt), int(limit), cursor, nil
}

func (s *Server) attemptToolHistoryHTTP(w http.ResponseWriter, r *http.Request, actor Actor, parts []string, project string) {
	queryProject, attempt, limit, cursor, err := attemptHistoryQuery(r, parts[5])
	if err != nil {
		writeError(w, err)
		return
	}
	if queryProject != project {
		writeError(w, forbidden())
		return
	}
	ctx, tx, cleanup, err := s.Service.beginAuthorityTx(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	defer cleanup()
	defer tx.Rollback()
	auth, err := s.authorizePersistentRunReadTx(ctx, tx, actor, project, parts[1])
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.Engine.AttemptToolEventsPageTx(auth.ctx, tx, auth.currentPrincipal.TenantID, project, parts[1], parts[3], attempt, limit, cursor)
	if err == nil {
		err = tx.Commit()
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, out, err)
}
