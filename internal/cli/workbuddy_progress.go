package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const (
	workBuddyProgressListUsage   = "run workbuddy-list accepts --limit 1..20, --filter attention, --task TASK, and the original --before cursor"
	workBuddyProgressCursorLimit = 1024
	workBuddyProgressUsage       = "run workbuddy-status requires RUN and one --node NODE"
	workBuddyProgressInvalid     = "invalid WorkBuddy progress response or mismatched request scope"
	workBuddyProgressRejected    = "control rejected the WorkBuddy progress read"
)

func workBuddyProgressCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 100) {
		return usage(workBuddyProgressUsage)
	}
	run := args[0]
	var node string
	fs := commandFlags("run workbuddy-status")
	fs.Func("node", "exact WorkBuddy workflow node ID", func(value string) error {
		if node != "" || !compositionCLIIdentifier(value, 150) {
			return errors.New(workBuddyProgressUsage)
		}
		node = value
		return nil
	})
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || node == "" {
		return usage(workBuddyProgressUsage)
	}
	path := "/api/v1/runs/" + url.PathEscape(run) + "/nodes/" + url.PathEscape(node) + "/workbuddy-status"
	raw, err := c.request(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		if f, ok := err.(*failure); ok && f.remote {
			return workBuddyProgressHTTPFailure(f.Status)
		}
		return err
	}
	progress, err := wc.DecodeCloudNodeProgress(raw)
	if err != nil || progress.ProjectID != c.project || progress.RunID != run || progress.NodeID != node {
		return &failure{Code: "workbuddy_progress_invalid", Message: workBuddyProgressInvalid}
	}
	return writeJSON(out, progress, c.token)
}

func workBuddyProgressListCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	fs := commandFlags("run workbuddy-list")
	var limit int64
	var before, filter, task string
	fs.Func("limit", "page size from 1 to 20", func(value string) error {
		n, valid := runWatchDecimal(value, 1, 20)
		if limit != 0 || !valid {
			return errors.New(workBuddyProgressListUsage)
		}
		limit = n
		return nil
	})
	fs.Func("filter", "attention to show submissions needing attention", func(value string) error {
		if filter != "" || value != wc.CloudProgressAttentionFilter {
			return errors.New(workBuddyProgressListUsage)
		}
		filter = value
		return nil
	})
	fs.Func("task", "exact Suma task ID", func(value string) error {
		if task != "" || !wc.ValidCloudProgressTaskID(value) {
			return errors.New(workBuddyProgressListUsage)
		}
		task = value
		return nil
	})
	fs.Func("before", "original continuation from next_cursor", func(value string) error {
		if before != "" || !validRunAttemptCursor(value, workBuddyProgressCursorLimit) {
			return errors.New(workBuddyProgressListUsage)
		}
		before = value
		return nil
	})
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return usage(workBuddyProgressListUsage)
	}
	if limit == 0 {
		limit = 10
	}
	query := url.Values{"limit": {strconv.FormatInt(limit, 10)}}
	if filter != "" {
		query.Set("filter", filter)
	}
	if task != "" {
		query.Set("task_id", task)
	}
	if before != "" {
		query.Set("before", before)
	}
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/workbuddy-cloud-progress?"+query.Encode(), nil, true)
	if err != nil {
		if f, ok := err.(*failure); ok && f.remote {
			return workBuddyProgressHTTPFailure(f.Status)
		}
		return err
	}
	page, err := wc.DecodeCloudNodeProgressPage(raw)
	if err != nil || page.ProjectID != c.project || page.Filter != filter || page.TaskID != task || int64(len(page.Items)) > limit ||
		page.NextCursor != "" && (page.NextCursor == before || !validRunAttemptCursor(page.NextCursor, workBuddyProgressCursorLimit) ||
			filter == "" && task == "" && int64(len(page.Items)) != limit) {
		return &failure{Code: "workbuddy_progress_invalid", Message: workBuddyProgressInvalid}
	}
	return writeJSON(out, page, c.token)
}

func workBuddyProgressHTTPFailure(status int) *failure {
	code := "http_error"
	switch status {
	case http.StatusBadRequest:
		code = "invalid"
	case http.StatusUnauthorized:
		code = "unauthenticated"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusConflict:
		code = "conflict"
	case http.StatusServiceUnavailable:
		code = "service_unavailable"
	}
	return &failure{Code: code, Status: status, Message: workBuddyProgressRejected}
}
