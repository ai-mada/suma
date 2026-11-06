package platform

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// ToolProfileListQuery is the single current list contract for HTTP, CLI and
// domain readers. State and Query are normalized before binding a cursor.
type ToolProfileListQuery struct {
	State    string `json:"state,omitempty"`
	Query    string `json:"query,omitempty"`
	WorkerID string `json:"worker_id,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

func toolProfileListInvalid() error {
	return invalidDiagnostic("validation.tool_profile_filter", "工具列表筛选无效", nil)
}

func normalizeToolProfileListQuery(q ToolProfileListQuery) (ToolProfileListQuery, error) {
	if q.State == "" {
		q.State = "active"
	}
	switch q.State {
	case "active", "enabled", "paused", "revoked", "all":
	default:
		return q, toolProfileListInvalid()
	}
	if len(q.Query) > 120 || !utf8.ValidString(q.Query) || strings.IndexFunc(q.Query, unicode.IsControl) >= 0 || len(q.Cursor) > 1024 {
		return q, toolProfileListInvalid()
	}
	if q.WorkerID != "" && !toolcontract.ValidCommandID(q.WorkerID) {
		return q, toolProfileListInvalid()
	}
	q.Query = strings.ToLower(strings.TrimSpace(q.Query))
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > 200 {
		return q, invalid("limit 必须为 1..200")
	}
	return q, nil
}

func toolProfileListCursorScope(p Principal, q ToolProfileListQuery) string {
	return "tool_profiles:" + digest(stringify(struct {
		Actor, State, Query string
		WorkerID            string `json:",omitempty"`
	}{p.ID, q.State, q.Query, q.WorkerID}))
}

// HTTP parses only transport shape here. Domain normalization is deliberately
// after current actor/organization/project authorization in ListToolProfiles.
func toolProfileListHTTPQuery(r *http.Request) (ToolProfileListQuery, error) {
	var out ToolProfileListQuery
	if len(r.URL.RawQuery) > 4096 {
		return out, toolProfileListInvalid()
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return out, toolProfileListInvalid()
	}
	for key, values := range q {
		if len(values) != 1 {
			return out, toolProfileListInvalid()
		}
		switch key {
		case "project_id", "state", "query", "worker_id", "limit", "cursor":
		default:
			return out, toolProfileListInvalid()
		}
	}
	out.State, out.Query, out.Cursor = q.Get("state"), q.Get("query"), q.Get("cursor")
	if raw, exists := q["worker_id"]; exists {
		if raw[0] == "" {
			return out, toolProfileListInvalid()
		}
		out.WorkerID = raw[0]
	}
	if raw, exists := q["limit"]; exists {
		out.Limit, err = strconv.Atoi(raw[0])
		if err != nil || out.Limit < 1 || out.Limit > 200 {
			return out, toolProfileListInvalid()
		}
	}
	return out, nil
}

func toolProfileListWhere(tenant, project, state string) (string, []any) {
	where, args := "tenant_id=? AND project_id=?", []any{tenant, project}
	if state == "active" {
		return where + " AND state IN (?,?)", append(args, "enabled", "paused")
	}
	if state != "all" {
		return where + " AND state=?", append(args, state)
	}
	return where, args
}

func toolProfilePublicContains(p ToolProfile, query string) bool {
	for _, value := range []string{p.Name, p.Profile.CommandID, p.WorkerID, p.Profile.Adapter, p.Profile.Protocol} {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

// Search is bounded by the existing 1000-profile project contract. Filtering
// exact public fields in Go keeps Unicode contains identical across all stores,
// and prevents a raw JSON search from matching credentials or private metadata.
func (s *Service) listToolProfilesFilteredTx(ctx context.Context, auth *authorizedCommand, project string, q ToolProfileListQuery) (ToolProfileList, error) {
	out := ToolProfileList{Items: []ToolProfile{}}
	var err error
	q, err = normalizeToolProfileListQuery(q)
	if err != nil {
		return out, err
	}
	collection := toolProfileListCursorScope(auth.currentPrincipal, q)
	last, err := automationDecodeCursor(auth.currentPrincipal, project, collection, q.Cursor)
	if err != nil {
		return out, err
	}
	where, args := toolProfileListWhere(auth.currentPrincipal.TenantID, project, q.State)
	if q.WorkerID != "" {
		where += " AND worker_id=?"
		args = append(args, q.WorkerID)
	}
	if q.Query == "" && q.WorkerID == "" {
		if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_profiles WHERE `+where), args...).Scan(&out.Total); err != nil {
			return out, err
		}
		pageArgs := append(append([]any{}, args...), last, q.Limit+1)
		rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE `+where+` AND id>? ORDER BY id LIMIT ?`), pageArgs...)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			p, e := scanToolProfile(rows)
			if e != nil {
				rows.Close()
				return out, e
			}
			out.Items = append(out.Items, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	} else {
		rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE `+where+` ORDER BY id LIMIT 1001`), args...)
		if e != nil {
			return out, e
		}
		examined := 0
		for rows.Next() {
			examined++
			if examined > 1000 {
				rows.Close()
				return out, conflict("项目工具档案已达 1000 项限制")
			}
			p, e := scanToolProfile(rows)
			if e != nil {
				rows.Close()
				return out, e
			}
			// Keep exact identity across stores, including collations that fold case.
			if q.WorkerID != "" && p.WorkerID != q.WorkerID || q.Query != "" && !toolProfilePublicContains(p, q.Query) {
				continue
			}
			out.Total++
			if p.ID > last && len(out.Items) <= q.Limit {
				out.Items = append(out.Items, p)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, collection, out.Items[q.Limit-1].ID)
	}
	for i, p := range out.Items {
		out.Items[i], err = s.toolProfileObservationTx(ctx, auth.tx, auth.currentPrincipal, project, p)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
