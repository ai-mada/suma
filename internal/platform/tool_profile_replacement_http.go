package platform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ai-mada/suma/internal/strictjson"
)

func replacementHTTPQuery(r *http.Request, list bool) (out ToolProfileReplacementListQuery, err error) {
	if len(r.URL.RawQuery) > 4096 {
		return out, toolReplacementInvalid()
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return out, toolReplacementInvalid()
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && (!list || key != "direction" && key != "limit" && key != "cursor") {
			return out, toolReplacementInvalid()
		}
	}
	if !list {
		return out, nil
	}
	out.Direction, out.Cursor = values.Get("direction"), values.Get("cursor")
	if raw, ok := values["direction"]; ok && raw[0] == "" {
		return out, toolReplacementInvalid()
	}
	if raw, ok := values["limit"]; ok {
		out.Limit, e = strconv.Atoi(raw[0])
		if e != nil || out.Limit < 1 || out.Limit > 100 || strconv.Itoa(out.Limit) != raw[0] {
			return out, toolReplacementInvalid()
		}
	}
	return out, nil
}
func (s *Server) toolProfileReplacementHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	list := len(parts) == 3 && parts[0] == "tool-profiles" && parts[2] == "replacements"
	detail := len(parts) == 2 && parts[0] == "tool-profile-replacements"
	if !list && !detail {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	if list && r.Method == http.MethodPost {
		out, e := s.Service.replaceToolProfile(r.Context(), actor, project, parts[1], func() (req ToolProfileReplacementRequest, err error) {
			raw, e := json.Marshal(m)
			if e != nil || strictjson.Decode(raw, &req, 2<<20) != nil {
				return req, toolReplacementInvalid()
			}
			if req.Reuse != nil {
				if _, present := m["name"]; present {
					return req, toolReplacementInvalid()
				}
			}
			return req, nil
		})
		respond(w, http.StatusCreated, out, e)
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	if detail {
		out, e := s.Service.getToolProfileReplacement(r.Context(), actor, project, parts[1], func() error { _, e := replacementHTTPQuery(r, false); return e })
		respond(w, http.StatusOK, out, e)
		return true
	}
	out, e := s.Service.listToolProfileReplacements(r.Context(), actor, project, parts[1], func() (ToolProfileReplacementListQuery, error) { return replacementHTTPQuery(r, true) })
	respond(w, http.StatusOK, out, e)
	return true
}
