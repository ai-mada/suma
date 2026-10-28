package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

const workspaceTreeRequestInvalid = "Workspace tree request is invalid"

// Only bounded directory reads are accepted. ParseQuery also rejects malformed
// escaping instead of silently accepting a partial query via URL.Query().
func workspaceTreeQuery(r *http.Request, project string, tree bool) (directory string, limit int, before string, err error) {
	limit = 100
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || len(r.URL.RawQuery) > 8192 || q.Get("project_id") == "" || q.Get("project_id") != project {
		return "", 0, "", &APIError{400, "workspace_tree_invalid", workspaceTreeRequestInvalid, nil}
	}
	for key, values := range q {
		if len(values) != 1 || key != "project_id" && !(tree && (key == "directory" || key == "limit" || key == "before")) {
			return "", 0, "", &APIError{400, "workspace_tree_invalid", workspaceTreeRequestInvalid, nil}
		}
	}
	if value, present := q["limit"]; present {
		limit, e = strconv.Atoi(value[0])
		if e != nil || strconv.Itoa(limit) != value[0] || limit < 1 || limit > 200 {
			return "", 0, "", &APIError{400, "workspace_tree_invalid", workspaceTreeRequestInvalid, nil}
		}
	}
	directory, before = q.Get("directory"), q.Get("before")
	if len(directory) > 1024 || len(before) > 1024 || q.Has("before") && before == "" {
		return "", 0, "", &APIError{400, "workspace_tree_invalid", workspaceTreeRequestInvalid, nil}
	}
	return directory, limit, before, nil
}
