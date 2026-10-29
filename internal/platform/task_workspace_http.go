package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) taskWorkspaceHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &APIError{http.StatusMethodNotAllowed, "method_not_allowed", "不支持此请求方法", nil})
		return
	}
	if !myWorkHuman(actor) {
		writeError(w, forbidden())
		return
	}
	q, _, err := parseTaskWorkspaceQuery(r, project, false)
	if err != nil {
		writeError(w, err)
		return
	}
	page, err := s.Service.ListTaskWorkspace(r.Context(), actor, project, q)
	respond(w, http.StatusOK, page, err)
}

// Both routes accept the same literal base filters. Summary has separate group
// paging and no item paging/sort; the item query remains /task-workspace.
func parseTaskWorkspaceQuery(r *http.Request, project string, summary bool) (TaskWorkspaceQuery, TaskCollectionGroupOptions, error) {
	q := TaskWorkspaceQuery{}
	if len(r.URL.RawQuery) > 8192 {
		return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
	}
	for key, entries := range values {
		if len(entries) != 1 {
			return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
		}
		switch key {
		case "project_id":
			if entries[0] == "" {
				return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
			}
		case "q", "status", "status_unset", "owner_id", "priority", "requirement_id", "unassigned", "priority_unset":
		case "sort", "limit", "cursor":
			if summary || entries[0] == "" {
				return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
			}
		case "group_by", "group_limit", "group_cursor":
			if !summary || entries[0] == "" {
				return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
			}
		default:
			return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
		}
	}
	if !handoffID(project, false) || values.Get("project_id") != project {
		return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
	}
	q = TaskWorkspaceQuery{Q: values.Get("q"), Status: values.Get("status"), OwnerID: values.Get("owner_id"), Priority: values.Get("priority"), RequirementID: values.Get("requirement_id"), Sort: values.Get("sort"), Cursor: values.Get("cursor"), Limit: 100}
	for name, target := range map[string]*bool{"unassigned": &q.Unassigned, "priority_unset": &q.PriorityUnset, "status_unset": &q.StatusUnset} {
		if values.Has(name) {
			switch values.Get(name) {
			case "true":
				*target = true
			case "false":
			default:
				return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
			}
		}
	}
	if values.Has("limit") {
		q.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || q.Limit < 1 || q.Limit > 100 || strconv.Itoa(q.Limit) != values.Get("limit") {
			return q, TaskCollectionGroupOptions{}, invalid(taskWorkspaceInvalid)
		}
	}
	q, err = normalizeTaskWorkspaceQuery(q)
	if err != nil {
		return q, TaskCollectionGroupOptions{}, err
	}
	options := TaskCollectionGroupOptions{}
	if summary {
		options.GroupBy, options.Cursor = values.Get("group_by"), values.Get("group_cursor")
		if values.Has("group_limit") {
			value := values.Get("group_limit")
			options.Limit, err = strconv.Atoi(value)
			if err != nil || options.Limit < 1 || options.Limit > 100 || strconv.Itoa(options.Limit) != value {
				return q, options, invalid(taskWorkspaceInvalid)
			}
		}
		options, err = normalizeTaskCollectionGroupOptions(options)
		if err != nil {
			return q, options, err
		}
		q.Sort, q.Limit = "", 0
	}
	return q, options, nil
}
