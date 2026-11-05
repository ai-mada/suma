package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const (
	workBuddyProgressUsage    = "run workbuddy-status requires RUN and one --node NODE"
	workBuddyProgressInvalid  = "invalid WorkBuddy progress response or mismatched request scope"
	workBuddyProgressRejected = "control rejected the WorkBuddy progress read"
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
