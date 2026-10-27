package cli

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const dispatchOptionsCLIUsage = "tasks dispatch-options requires TASK --expected-version N --workflow ID --workflow-version N --node ID; optional --limit 1..200 and --before original_cursor"

// This read command deliberately cannot enable, prepare or start a tool. The
// server checks task/workflow versions and signs the recommendation page scope.
func taskDispatchOptionsCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 64) {
		return usage(dispatchOptionsCLIUsage)
	}
	fs := commandFlags("tasks dispatch-options")
	task := fs.String("expected-version", "", "current task version")
	flow := fs.String("workflow", "", "fixed workflow ID")
	version := fs.String("workflow-version", "", "fixed workflow version")
	node := fs.String("node", "", "development Agent node ID")
	limit := fs.String("limit", "50", "page size 1..200")
	before := fs.String("before", "", "original scoped next_cursor")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return usage(dispatchOptionsCLIUsage)
	}
	valid := func(raw string, max int) bool {
		n, err := strconv.Atoi(raw)
		return err == nil && n > 0 && n <= max && strconv.Itoa(n) == raw
	}
	if !valid(*task, 2147483646) || !valid(*version, 2147483646) || !valid(*limit, 200) || !compositionCLIIdentifier(*flow, 100) || !compositionCLIIdentifier(*node, 150) || len(*before) > 4096 || !utf8.ValidString(*before) || strings.IndexFunc(*before, unicode.IsControl) >= 0 {
		return usage(dispatchOptionsCLIUsage)
	}
	q := url.Values{"expected_task_version": {*task}, "workflow_id": {*flow}, "workflow_version": {*version}, "agent_node_id": {*node}, "limit": {*limit}}
	if *before != "" {
		q.Set("before", *before)
	}
	return contextRead(ctx, c, "/api/v1/tasks/"+url.PathEscape(args[0])+"/dispatch-options?"+q.Encode(), out)
}
