package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/strictjson"
)

const (
	dispatchRecommendationListUsage              = "tasks recommendations requires TASK and accepts --limit 1..100 and --before original_cursor"
	dispatchRecommendationGetUsage               = "tasks recommendation requires exactly one ID"
	dispatchRecommendationHistoryResponseInvalid = "dispatch recommendation response must preserve the requested scope and contain valid records"
)

func dispatchRecommendationHistoryCommand(ctx context.Context, c *client, action string, args []string, out io.Writer) error {
	const collection = "/api/v1/dispatch-recommendations"
	path := collection
	var task, id, before string
	var limit int64
	switch action {
	case "recommendations":
		if len(args) == 0 || !compositionCLIIdentifier(args[0], 64) {
			return usage(dispatchRecommendationListUsage)
		}
		task = args[0]
		flags := commandFlags("tasks recommendations")
		limitValue := flags.String("limit", "20", "page size 1..100")
		cursor := flags.String("before", "", "original continuation cursor")
		if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
			return usage(dispatchRecommendationListUsage)
		}
		var ok bool
		limit, ok = runWatchDecimal(*limitValue, 1, 100)
		if !ok || !validRecommendationHistoryCursor(*cursor) {
			return usage(dispatchRecommendationListUsage)
		}
		before = *cursor
		query := url.Values{"task_id": {task}, "limit": {*limitValue}}
		if before != "" {
			query.Set("before", before)
		}
		path += "?" + query.Encode()
	case "recommendation":
		if len(args) != 1 || !compositionCLIIdentifier(args[0], 64) {
			return usage(dispatchRecommendationGetUsage)
		}
		id = args[0]
		path += "/" + url.PathEscape(id)
	default:
		return usage(dispatchRecommendationGetUsage)
	}
	if c.project == "" {
		return usage("--project is required for this command")
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	invalid := &failure{Code: "response", Message: dispatchRecommendationHistoryResponseInvalid}
	// Validate the JSON token stream without closing opaque workflow and
	// recommendation fields. Preserve the server's original view in output.
	if strictjson.Decode(raw, &fields, 16<<20) != nil {
		return invalid
	}
	if id != "" {
		if recordID, ok := recommendationHistoryCardID(fields, c.project, ""); !ok || recordID != id {
			return invalid
		}
	} else if !validRecommendationHistoryPage(fields, c.project, task, before, int(limit)) {
		return invalid
	}
	return writeJSON(out, raw, c.token)
}

func validRecommendationHistoryCursor(value string) bool {
	return len(value) <= 64 && utf8.ValidString(value) && !strings.ContainsAny(value, "/\\") &&
		!strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) })
}

func validRecommendationHistoryPage(fields map[string]json.RawMessage, project, task, before string, limit int) bool {
	var items []map[string]json.RawMessage
	var total *int
	var next *string
	if len(fields) != 3 || json.Unmarshal(fields["items"], &items) != nil || items == nil || len(items) > limit ||
		json.Unmarshal(fields["total"], &total) != nil || total == nil || *total < len(items) ||
		json.Unmarshal(fields["next_cursor"], &next) != nil || next == nil || !validRecommendationHistoryCursor(*next) {
		return false
	}
	seen := make(map[string]bool, len(items))
	var last string
	for _, item := range items {
		id, ok := recommendationHistoryCardID(item, project, task)
		if !ok || seen[id] || id == before {
			return false
		}
		seen[id] = true
		last = id
	}
	return *next == "" || len(items) == limit && *total > len(items) && *next == last
}

func recommendationHistoryCardID(fields map[string]json.RawMessage, project, task string) (string, bool) {
	var id, projectID string
	var version int
	if !recommendationHistoryCanonicalFields(fields, "id", "project_id", "version", "source") ||
		json.Unmarshal(fields["id"], &id) != nil || !compositionCLIIdentifier(id, 64) ||
		json.Unmarshal(fields["project_id"], &projectID) != nil || projectID != project ||
		json.Unmarshal(fields["version"], &version) != nil || version < 1 {
		return "", false
	}
	var source, context, record map[string]json.RawMessage
	var taskID string
	if json.Unmarshal(fields["source"], &source) != nil || !recommendationHistoryCanonicalFields(source, "context") ||
		json.Unmarshal(source["context"], &context) != nil || !recommendationHistoryCanonicalFields(context, "task") ||
		json.Unmarshal(context["task"], &record) != nil || !recommendationHistoryCanonicalFields(record, "id") ||
		json.Unmarshal(record["id"], &taskID) != nil || !compositionCLIIdentifier(taskID, 64) || task != "" && taskID != task {
		return "", false
	}
	return id, true
}

func recommendationHistoryCanonicalFields(fields map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
		for key := range fields {
			if key != name && strings.EqualFold(key, name) {
				return false
			}
		}
	}
	return true
}
