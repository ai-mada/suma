package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	runAttemptsUsage          = "run attempts requires RUN and one --node NODE, with optional --limit 1..100 and --cursor CURSOR"
	runAttemptUsage           = "run attempt requires RUN, one --node NODE and one --attempt N"
	runToolEventsUsage        = "run tool-events requires RUN, one --node NODE and one --attempt N, with optional --limit 1..100 and --cursor CURSOR"
	runAttemptResponseInvalid = "invalid run attempt response or mismatched request scope"

	runAttemptsCursorLimit   = 2731 // Existing scheduling cursor's 2048 decoded bytes.
	runToolEventsCursorLimit = 4096
)

var runAttemptCursor = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validRunAttemptCursor(value string, maxLength int) bool {
	return len(value) <= maxLength && runAttemptCursor.MatchString(value)
}

func runAttemptReadCommand(ctx context.Context, c *client, action string, args []string, out io.Writer) error {
	usageText, cursorLimit := runAttemptsUsage, runAttemptsCursorLimit
	switch action {
	case "attempt":
		usageText = runAttemptUsage
	case "tool-events":
		usageText, cursorLimit = runToolEventsUsage, runToolEventsCursorLimit
	}
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 100) {
		return usage(usageText)
	}
	run := args[0]
	var node, attemptArg, limitArg, cursor string
	flags := commandFlags("run " + action)
	one := func(target *string, valid func(string) bool) func(string) error {
		return func(value string) error {
			if *target != "" || !valid(value) {
				return errors.New(usageText)
			}
			*target = value
			return nil
		}
	}
	flags.Func("node", "exact workflow node ID", one(&node, func(v string) bool { return compositionCLIIdentifier(v, 150) }))
	if action != "attempts" {
		flags.Func("attempt", "exact positive attempt number", one(&attemptArg, func(v string) bool {
			_, ok := runWatchDecimal(v, 1, maxRunWatchCursor)
			return ok
		}))
	}
	if action != "attempt" {
		flags.Func("limit", "page size 1..100", one(&limitArg, func(v string) bool {
			_, ok := runWatchDecimal(v, 1, 100)
			return ok
		}))
		flags.Func("cursor", "original server continuation", one(&cursor, func(v string) bool { return validRunAttemptCursor(v, cursorLimit) }))
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || node == "" || (action != "attempts" && attemptArg == "") {
		return usage(usageText)
	}

	path := "/api/v1/runs/" + url.PathEscape(run) + "/nodes/" + url.PathEscape(node) + "/attempts"
	var attempt int
	if attemptArg != "" {
		var err error
		attempt, err = strconv.Atoi(attemptArg)
		if err != nil {
			return usage(usageText) // Guard narrower native int widths.
		}
		path += "/" + attemptArg
	}
	if action == "tool-events" {
		path += "/tool-events"
	}
	limit := 50
	if action != "attempt" {
		if limitArg != "" {
			limit, _ = strconv.Atoi(limitArg)
		}
		q := url.Values{"limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		path += "?" + q.Encode()
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return err
	}
	var valid bool
	switch action {
	case "attempts":
		valid = validRunAttemptPage(raw, run, node, cursor, limit)
	case "attempt":
		var value dag.ExecutionAttempt
		valid = strictjson.Decode(raw, &value, 16<<20) == nil && validRunAttempt(value, run, node) &&
			value.Attempt == attempt && validRunAttemptResult(value)
	case "tool-events":
		valid = validRunToolEventsPage(raw, c.project, run, node, attempt, cursor, limit)
	}
	if !valid {
		return &failure{Code: "run_attempt_response_invalid", Message: runAttemptResponseInvalid}
	}
	// Emit the server's original facts rather than a projection, preserving
	// numeric lexemes, provider payloads and opaque pagination positions.
	return writeJSON(out, raw, c.token)
}

func validRunAttempt(item dag.ExecutionAttempt, run, node string) bool {
	// A child workflow attempt can legitimately have no worker or fence.
	return item.RunID == run && item.NodeID == node && item.Attempt > 0 && int64(item.Attempt) <= maxRunWatchCursor &&
		item.Fence >= 0 && item.Fence <= maxRunWatchCursor && item.State != ""
}

func validRunAttemptPage(raw []byte, run, node, cursor string, limit int) bool {
	var page dag.AttemptPage
	if strictjson.Decode(raw, &page, 16<<20) != nil || len(page.Items) > limit || page.Total < len(page.Items) {
		return false
	}
	if page.NextCursor != "" && (len(page.Items) != limit || page.NextCursor == cursor || !validRunAttemptCursor(page.NextCursor, runAttemptsCursorLimit)) {
		return false
	}
	for i, item := range page.Items {
		if !validRunAttempt(item, run, node) || item.ResultSource != "not_loaded" || item.Result != nil ||
			item.Reconciliation != nil || item.ChildRunID != "" || (i > 0 && page.Items[i-1].Attempt <= item.Attempt) {
			return false
		}
	}
	return true
}

func validRunAttemptResult(item dag.ExecutionAttempt) bool {
	if result := item.Result; result != nil && (result.RunID != item.RunID || result.NodeID != item.NodeID ||
		result.Attempt != item.Attempt || result.Fence != item.Fence || result.WorkerID != item.WorkerID) {
		return false
	}
	switch item.ResultSource {
	case "unreported":
		return item.Result == nil && item.Reconciliation == nil && item.ChildRunID == ""
	case "runner_result":
		return item.Result != nil && item.Reconciliation == nil && item.ChildRunID == ""
	case "manual_reconciliation":
		return item.Result == nil && item.Reconciliation != nil && item.Reconciliation.Outcome != "" && item.ChildRunID == ""
	case "child_workflow":
		return item.Result == nil && item.Reconciliation == nil && compositionCLIIdentifier(item.ChildRunID, 100)
	case "managed_observation":
		// A managed observation can coexist with a prior worker result.
		return true
	default:
		return false
	}
}

func validRunToolEventsPage(raw []byte, project, run, node string, attempt int, cursor string, limit int) bool {
	var page dag.AttemptToolEventPage
	if strictjson.Decode(raw, &page, (4<<20)+(32<<10)) != nil || page.SchemaVersion != 1 ||
		page.ProjectID != project || page.RunID != run || page.NodeID != node || page.Attempt != attempt ||
		page.Fence < 0 || page.Fence > maxRunWatchCursor || page.Watermark < 0 || page.Watermark > maxRunWatchCursor ||
		page.ScannedThrough < 0 || page.ScannedThrough > page.Watermark || len(page.Items) > limit ||
		!validRunAttemptCursor(page.PageCursor, runToolEventsCursorLimit) || (cursor != "" && page.PageCursor != cursor) {
		return false
	}
	if page.HasMore {
		if page.ScannedThrough == 0 || page.ScannedThrough == page.Watermark || page.NextCursor == page.PageCursor ||
			!validRunAttemptCursor(page.NextCursor, runToolEventsCursorLimit) {
			return false
		}
	} else if page.NextCursor != "" || page.ScannedThrough != page.Watermark {
		return false
	}
	// The durable raw payload and its display projection can include exact
	// numbers above float64 precision. Compare without normalizing their text.
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&page) != nil {
		return false
	}
	var previous int64
	for _, event := range page.Items {
		if event.Sequence <= previous || event.Sequence > page.ScannedThrough || event.RunID != run || event.NodeID != node ||
			event.Type != "tool_progress" || !validRunToolEventParent(event, page) {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, event.At); err != nil {
			return false
		}
		previous = event.Sequence
	}
	return true
}

func validRunToolEventParent(event dag.Event, page dag.AttemptToolEventPage) bool {
	if page.Fence == 0 || event.RawData == "" || len(event.RawData) > 2<<20 || !json.Valid([]byte(event.RawData)) {
		return false
	}
	d := json.NewDecoder(strings.NewReader(event.RawData))
	d.UseNumber()
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	original := make(map[string]any)
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return false
		}
		if _, duplicate := original[name]; duplicate {
			return false
		}
		switch strings.ToLower(name) {
		case "attempt", "fence", "worker_id":
			if name != strings.ToLower(name) {
				return false
			}
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || (name == "worker_id" && !tc.ValidJSONUnicode(value)) {
			return false
		}
		field := json.NewDecoder(bytes.NewReader(value))
		field.UseNumber()
		var projected any
		if field.Decode(&projected) != nil {
			return false
		}
		original[name] = projected
	}
	return original["attempt"] == json.Number(strconv.Itoa(page.Attempt)) &&
		original["fence"] == json.Number(strconv.FormatInt(page.Fence, 10)) && original["worker_id"] == page.WorkerID &&
		reflect.DeepEqual(original, event.Data)
}
