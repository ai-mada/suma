package cli

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func contextID(id string) bool {
	return id != "" && id != "." && id != ".." && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "/\\?#%\r\n\t")
}

// This is an explicit query registry, not a suffix heuristic. Preview does not
// create tasks or reserve resources. Unknown POST routes remain mutations.
// Original-view headers are independently retained for all non-GET requests.
func requestIsReadOnly(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return true
	}
	if method != http.MethodPost {
		return false
	}
	u, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(u.Path, "/api/v1/") {
		return false
	}
	switch u.Path {
	case "/api/v1/task-owner-changes/preview", "/api/v1/tool-profiles/matches", taskPriorityBatchPath + "/preview", taskStatusBatchPath + "/preview":
		return true
	case "/api/v1/dispatch-recommendations/options":
		return true
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/api/v1/"), "/")
	return len(parts) == 4 && parts[0] == "requirements" && contextID(parts[1]) && parts[2] == "design" && parts[3] == "preview"
}

func knowledgeCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, projectContextHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(knowledgeActionsUsage)
	}
	path := "/api/v1/project-knowledge"
	switch args[0] {
	case "import-skill":
		return knowledgeSkillImportCommand(c, args[1:], out)
	case "search":
		return knowledgeSearchCommand(ctx, c, args[1:], out)
	case "list":
		fs := commandFlags("knowledge list")
		kind := fs.String("kind", "", "memory, spec, or skill")
		state := fs.String("state", "", "draft, published, or retired")
		query := fs.String("query", "", "search text")
		limit := fs.Int("limit", 20, "page size: 1..100")
		cursor := fs.String("cursor", "", "opaque next_cursor")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *limit < 1 || *limit > 100 ||
			(*kind != "" && *kind != "memory" && *kind != "spec" && *kind != "skill") ||
			(*state != "" && *state != "draft" && *state != "published" && *state != "retired") {
			return usage("invalid knowledge filters or pagination; limit must be 1..100")
		}
		q := url.Values{"limit": {strconv.Itoa(*limit)}}
		for key, value := range map[string]string{"kind": *kind, "state": *state, "q": *query, "cursor": *cursor} {
			if value != "" {
				q.Set(key, value)
			}
		}
		return contextRead(ctx, c, path+"?"+q.Encode(), out)
	case "create", "extract":
		if args[0] == "extract" {
			path += "/extractions"
		}
		return contextFile(ctx, c, http.MethodPost, path, args[1:], in, out)
	case "show", "history", "revisions", "edit", "command", "commands":
		if len(args) < 2 || !contextID(args[1]) {
			return usage("project context command requires a valid resource ID")
		}
		path += "/" + url.PathEscape(args[1])
		switch args[0] {
		case "show":
			if len(args) != 2 {
				return usage("project context show accepts only the resource ID")
			}
			return contextRead(ctx, c, path, out)
		case "history", "revisions":
			return contextHistory(ctx, c, path+"/revisions", args[2:], out)
		case "edit":
			return contextFile(ctx, c, http.MethodPatch, path, args[2:], in, out)
		default:
			return contextFile(ctx, c, http.MethodPost, path+"/commands", args[2:], in, out)
		}
	}
	return usage("unknown project context command; use knowledge --help or requirement design --help")
}

func requirementDesignCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, projectContextHelp(c.locale))
		return err
	}
	if len(args) < 2 || !contextID(args[1]) {
		return usage("requirement design requires an action and requirement ID; use requirement design --help")
	}
	path := "/api/v1/requirements/" + url.PathEscape(args[1]) + "/design"
	switch args[0] {
	case "show":
		if len(args) != 2 {
			return usage("project context show accepts only the resource ID")
		}
		return contextRead(ctx, c, path, out)
	case "history", "revisions":
		return contextHistory(ctx, c, path+"/revisions", args[2:], out)
	case "adoptions":
		return contextHistory(ctx, c, path+"/adoptions", args[2:], out)
	case "save":
		return contextFile(ctx, c, http.MethodPut, path, args[2:], in, out)
	case "command", "commands":
		return contextFile(ctx, c, http.MethodPost, path+"/commands", args[2:], in, out)
	case "preview", "adopt":
		return contextFile(ctx, c, http.MethodPost, path+"/"+args[0], args[2:], in, out)
	}
	return usage("unknown project context command; use knowledge --help or requirement design --help")
}

func contextRead(ctx context.Context, c *client, path string, out io.Writer) error {
	raw, err := c.request(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func contextHistory(ctx context.Context, c *client, path string, args []string, out io.Writer) error {
	fs := commandFlags("project context history")
	limit := fs.Int("limit", 20, "page size: 1..100")
	cursor := fs.String("cursor", "", "opaque next_cursor")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *limit < 1 || *limit > 100 {
		return usage("invalid knowledge filters or pagination; limit must be 1..100")
	}
	q := url.Values{"limit": {strconv.Itoa(*limit)}}
	if *cursor != "" {
		q.Set("cursor", *cursor)
	}
	return contextRead(ctx, c, path+"?"+q.Encode(), out)
}

func contextFile(ctx context.Context, c *client, method, path string, args []string, in io.Reader, out io.Writer) error {
	fs := commandFlags("project context request")
	file := fs.String("file", "", "UTF-8 JSON file, or - for bounded stdin")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *file == "" {
		return usage("project context request requires --file JSON_FILE or --file -")
	}
	body, err := readObject(*file, in)
	if err != nil {
		return err
	}
	if !requestIsReadOnly(method, path) {
		if err = c.operation(method, path, body); err != nil {
			return err
		}
	}
	raw, err := c.request(ctx, method, path, body, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}
