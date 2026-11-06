package cli

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const toolListUsage = "tools list requires --project and accepts --state active|enabled|paused|revoked|all, --worker with an exact worker ID, --query up to 120 UTF-8 bytes, --limit 1..200 and the original --cursor. See tools list --help"
const toolListResponseInvalid = "tool list response must preserve the requested project, worker filter, state, complete profiles and page boundaries"

// toolListCommand is the original browser-session API read. It forwards the
// opaque cursor and filters so the server reads the exact requested page.
func toolListCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("tools list")
	state := flags.String("state", "active", "active, enabled, paused, revoked or all")
	query := flags.String("query", "", "public tool fields; at most 120 UTF-8 bytes")
	limit := flags.Int("limit", 50, "page size from 1 to 200")
	cursor := flags.String("cursor", "", "opaque cursor returned by the same filtered list")
	var worker string
	flags.Func("worker", "exact case-sensitive worker ID", func(value string) error {
		if !tc.ValidCommandID(value) {
			return usage(toolListUsage)
		}
		worker = value
		return nil
	})
	if flags.Parse(args) != nil || flags.NArg() != 0 || !toolDoctorID(c.project) || !toolListState(*state) || len(*query) > 120 || !utf8.ValidString(*query) || strings.ContainsFunc(*query, unicode.IsControl) || *limit < 1 || *limit > 200 || !toolListCursor(*cursor) {
		return usage(toolListUsage)
	}
	values := url.Values{"state": {*state}, "limit": {strconv.Itoa(*limit)}}
	if *query != "" {
		values.Set("query", *query)
	}
	if worker != "" {
		values.Set("worker_id", worker)
	}
	if *cursor != "" {
		values.Set("cursor", *cursor)
	}
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/tool-profiles?"+values.Encode(), nil, true)
	if err != nil {
		return err
	}
	var list platform.ToolProfileList
	if strictjson.Decode(raw, &list, 16<<20) != nil || list.Items == nil || len(list.Items) > *limit || list.Total < len(list.Items) || !toolListCursor(list.NextCursor) || len(list.Items) == 0 && list.NextCursor != "" {
		return &failure{Code: "response", Message: toolListResponseInvalid}
	}
	seen := map[string]bool{}
	for _, item := range list.Items {
		if !toolDoctorID(item.ID) || seen[item.ID] || item.ProjectID != c.project || !toolDoctorID(item.WorkerID) ||
			(worker != "" && item.WorkerID != worker) || !toolListItemState(*state, item.State) ||
			item.Version < 1 || strings.TrimSpace(item.Name) == "" || tc.ValidateProfile(item.Profile, true) != nil {
			return &failure{Code: "response", Message: toolListResponseInvalid}
		}
		seen[item.ID] = true
	}
	// Preserve source counters, metadata, diagnostics and numeric lexemes. The
	// common writer only applies the existing credential redaction/formatting.
	return writeJSON(out, raw, c.token)
}

func toolListState(state string) bool {
	switch state {
	case "active", "enabled", "paused", "revoked", "all":
		return true
	}
	return false
}
func toolListItemState(filter, state string) bool {
	if state != "enabled" && state != "paused" && state != "revoked" {
		return false
	}
	return filter == "all" || filter == state || filter == "active" && state != "revoked"
}
func toolListCursor(cursor string) bool {
	return len(cursor) <= 1024 && utf8.ValidString(cursor) && !strings.ContainsFunc(cursor, unicode.IsControl)
}
