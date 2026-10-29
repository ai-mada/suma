package platform

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) overviewAttentionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid("无效分页游标"))
		return
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "project_id" && key != "kind" && key != "limit" && key != "cursor") {
			writeError(w, invalid("无效分页游标"))
			return
		}
	}
	limit := 6
	if values.Has("limit") {
		v := values.Get("limit")
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 50 || strconv.Itoa(limit) != v {
			writeError(w, invalid("无效分页游标"))
			return
		}
	}
	if values.Has("cursor") && values.Get("cursor") == "" {
		writeError(w, invalid("无效分页游标"))
		return
	}
	s.projectReadJSON(w, r, actor, project, func(ctx context.Context, auth *authorizedCommand) (any, error) {
		return s.Service.overviewAttentionTx(ctx, auth, project, values.Get("kind"), limit, values.Get("cursor"), now())
	})
}
