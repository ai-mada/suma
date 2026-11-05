package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/ai-mada/suma/internal/strictjson"
)

const (
	taskDispatchPlansPath      = "/api/v1/task-dispatch-plans"
	taskDispatchPreviewLimit   = 4 << 20
	dispatchTestOptionCLIUsage = "tasks dispatch-test-option requires TASK --expected-version N --workflow ID --workflow-version N --node ID --tool ID --expected-tool-version N --test-node ID --test-tool ID --expected-test-tool-version N"
	dispatchPreviewCLIUsage    = "tasks dispatch-preview requires --file REQUEST_JSON or --file - with an explicit idempotency_key of 8..128 UTF-8 bytes"
	dispatchPreviewCLIJSON     = "dispatch preview requires one UTF-8 JSON object with unique keys and an explicit original idempotency_key"
	dispatchPlanCLIUsage       = "tasks dispatch-plan requires an exact PLAN ID"
	dispatchStartCLIUsage      = "tasks dispatch-start requires PLAN --expected-version N --operation KEY (8..128 UTF-8 bytes)"
)

// The typed command paths share the existing project scope, authorization
// headers and server diagnostics. Preview and Start are separate requests.
func taskDispatchCommand(ctx context.Context, c *client, action string, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && args[0] == "--help" {
		_, err := io.WriteString(out, c.text(taskDispatchHelpZH, taskDispatchHelpEN))
		return err
	}
	switch action {
	case "dispatch-test-option":
		return taskDispatchTestOptionCommand(ctx, c, args, out)
	case "dispatch-plan":
		if len(args) != 1 || !compositionCLIIdentifier(args[0], 64) {
			return usage(dispatchPlanCLIUsage)
		}
		return contextRead(ctx, c, taskDispatchPlansPath+"/"+url.PathEscape(args[0]), out)
	case "dispatch-preview":
		fs := commandFlags("tasks dispatch-preview")
		file := fs.String("file", "", "original preview request JSON, or - for standard input")
		if fs.Parse(args) != nil || fs.NArg() != 0 || *file == "" {
			return usage(dispatchPreviewCLIUsage)
		}
		body, err := readTaskDispatchPreview(*file, in)
		if err != nil {
			return err
		}
		return taskDispatchPost(ctx, c, taskDispatchPlansPath, body, out)
	case "dispatch-start":
		if len(args) == 0 || !compositionCLIIdentifier(args[0], 64) {
			return usage(dispatchStartCLIUsage)
		}
		fs := commandFlags("tasks dispatch-start")
		version := fs.String("expected-version", "", "the reviewed plan version")
		key := fs.String("operation", "", "the original start operation key")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !compositionCLIInteger(json.Number(*version), 1) || !handoffCLIOperation(*key) {
			return usage(dispatchStartCLIUsage)
		}
		body := map[string]any{"expected_version": json.Number(*version), "idempotency_key": *key}
		return taskDispatchPost(ctx, c, taskDispatchPlansPath+"/"+url.PathEscape(args[0])+"/start", body, out)
	default:
		return usage("unknown command or arguments; use --help")
	}
}

func taskDispatchTestOptionCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 64) {
		return usage(dispatchTestOptionCLIUsage)
	}
	fs := commandFlags("tasks dispatch-test-option")
	taskVersion := fs.String("expected-version", "", "current task version")
	flow := fs.String("workflow", "", "fixed workflow ID")
	flowVersion := fs.String("workflow-version", "", "fixed workflow version")
	node := fs.String("node", "", "development node ID")
	tool := fs.String("tool", "", "fixed development tool profile ID")
	toolVersion := fs.String("expected-tool-version", "", "development tool profile version")
	testNode := fs.String("test-node", "", "test node ID")
	testTool := fs.String("test-tool", "", "independent test tool profile ID")
	testToolVersion := fs.String("expected-test-tool-version", "", "test tool profile version")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 ||
		!compositionCLIIdentifier(*flow, 100) || !compositionCLIIdentifier(*node, 150) ||
		!compositionCLIIdentifier(*tool, 64) || !compositionCLIIdentifier(*testNode, 150) || !compositionCLIIdentifier(*testTool, 64) {
		return usage(dispatchTestOptionCLIUsage)
	}
	for _, version := range []string{*taskVersion, *flowVersion, *toolVersion, *testToolVersion} {
		if !compositionCLIInteger(json.Number(version), 1) {
			return usage(dispatchTestOptionCLIUsage)
		}
	}
	q := url.Values{
		"expected_task_version": {*taskVersion}, "workflow_id": {*flow}, "workflow_version": {*flowVersion}, "agent_node_id": {*node},
		"development_tool_profile_id": {*tool}, "development_tool_profile_version": {*toolVersion},
		"test_node_id": {*testNode}, "test_tool_profile_id": {*testTool}, "test_tool_profile_version": {*testToolVersion},
	}
	return contextRead(ctx, c, "/api/v1/tasks/"+url.PathEscape(args[0])+"/dispatch-test-option?"+q.Encode(), out)
}

func readTaskDispatchPreview(file string, input io.Reader) (map[string]any, error) {
	if file != "-" {
		handle, err := os.Open(file)
		if err != nil {
			return nil, usage("cannot open JSON file")
		}
		defer handle.Close()
		input = handle
	}
	if input == nil {
		return nil, usage("cannot read JSON input")
	}
	raw, err := io.ReadAll(io.LimitReader(input, taskDispatchPreviewLimit+1))
	var syntax map[string]json.RawMessage
	if err != nil || strictjson.Decode(raw, &syntax, taskDispatchPreviewLimit) != nil {
		return nil, usage(dispatchPreviewCLIJSON)
	}
	// Read the validated original bytes to retain exact numbers and fields.
	// The server remains responsible for the preview's domain contract.
	body, err := readObject("-", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	key, _ := body["idempotency_key"].(string)
	if !handoffCLIOperation(key) {
		return nil, usage(dispatchPreviewCLIUsage)
	}
	return body, nil
}

func taskDispatchPost(ctx context.Context, c *client, path string, body map[string]any, out io.Writer) error {
	raw, err := c.request(ctx, http.MethodPost, path, body, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

const taskDispatchHelpEN = `Task dispatch requires --project before the tasks command.

  tasks dispatch-test-option TASK --expected-version N --workflow ID --workflow-version N
    --node ID --tool ID --expected-tool-version N --test-node ID --test-tool ID --expected-test-tool-version N
  tasks dispatch-preview --file REQUEST_JSON|-
  tasks dispatch-plan PLAN
  tasks dispatch-start PLAN --expected-version N --operation KEY

Review the saved plan before Start. For an unconfirmed write, reuse the same
project, caller, request and operation key. Preview requires an explicit
idempotency_key of 8..128 UTF-8 bytes.
`

const taskDispatchHelpZH = `任务派发须在 tasks 命令前指定 --project。

  tasks dispatch-test-option TASK --expected-version N --workflow ID --workflow-version N
    --node ID --tool ID --expected-tool-version N --test-node ID --test-tool ID --expected-test-tool-version N
  tasks dispatch-preview --file REQUEST_JSON|-
  tasks dispatch-plan PLAN
  tasks dispatch-start PLAN --expected-version N --operation KEY

启动前先审阅已保存的计划。写入结果未确认时，核对原项目、调用者、请求正文
和操作标识，沿原请求复核。预览须明确提供 8–128 个 UTF-8 字节的 idempotency_key。
`
