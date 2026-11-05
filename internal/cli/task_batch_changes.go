package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/strictjson"
)

const (
	taskPriorityBatchPath   = "/api/v1/task-priority-changes"
	taskStatusBatchPath     = "/api/v1/task-planning-status-changes"
	taskBatchInputLimit     = 16 << 10
	taskBatchUsage          = "task-priority and task-status require preview or apply with --file JSON_FILE|-; use --help"
	taskBatchInputInvalid   = "invalid task batch; provide the exact project, 1..16 unique task IDs and integer versions, the required target, and an original operation key for apply"
	taskBatchPreviewInvalid = "task batch preview does not match the selected project, tasks or versions"
	taskBatchUnconfirmed    = "task batch receipt is unconfirmed; retry only with the same operation key and payload"
)

func taskBatchCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	command := args[0]
	if len(args) == 2 && (args[1] == "--help" || args[1] == "help") {
		_, err := io.WriteString(out, command+c.text(taskBatchHelpZH, taskBatchHelpEN))
		return err
	}
	if len(args) < 2 || args[1] != "preview" && args[1] != "apply" {
		return usage(taskBatchUsage)
	}
	apply := args[1] == "apply"
	flags := commandFlags(command + " " + args[1])
	file := flags.String("file", "", "original JSON request file; - reads standard input")
	if flags.Parse(args[2:]) != nil || flags.NArg() != 0 || *file == "" {
		return usage(taskBatchUsage)
	}
	if !taskBatchID(c.project) {
		return usage(taskBatchInputInvalid)
	}

	path, field := taskPriorityBatchPath, "priority"
	if command == "task-status" {
		path, field = taskStatusBatchPath, "status"
	}
	body, err := readTaskBatchInput(*file, input)
	if err != nil {
		return err
	}
	if !validTaskBatchInput(body, c.project, field, apply) {
		return usage(taskBatchInputInvalid)
	}
	body["project_id"] = c.project
	if !apply {
		path += "/preview"
	}
	// The original selection and operation key are retained. Never retry
	// automatically or infer an operation ID for a batch write.
	raw, err := c.request(ctx, http.MethodPost, path, body, false)
	if err != nil {
		var f *failure
		if apply && errors.As(err, &f) && (f.Code == "response" || f.Status >= 500) {
			return &failure{Code: "unconfirmed", Message: taskBatchUnconfirmed}
		}
		return err
	}
	if !validTaskBatchResponse(raw, body, field, apply) {
		if apply {
			return &failure{Code: "unconfirmed", Message: taskBatchUnconfirmed}
		}
		return &failure{Code: "response", Message: taskBatchPreviewInvalid}
	}
	return writeJSON(out, raw, c.token)
}

func readTaskBatchInput(file string, input io.Reader) (map[string]any, error) {
	if file != "-" {
		handle, err := os.Open(file)
		if err != nil {
			return nil, usage("cannot open JSON file")
		}
		defer handle.Close()
		input = handle
	}
	if input == nil {
		return nil, usage(taskBatchInputInvalid)
	}
	raw, err := io.ReadAll(io.LimitReader(input, taskBatchInputLimit+1))
	var syntax map[string]json.RawMessage
	if err != nil || strictjson.Decode(raw, &syntax, taskBatchInputLimit) != nil {
		return nil, usage(taskBatchInputInvalid)
	}
	return readObject("-", bytes.NewReader(raw))
}

func taskBatchID(value string) bool {
	if !handoffCLIIdentifier(value, false) || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 33 || r == 127 {
			return false
		}
	}
	return true
}

func taskBatchTarget(field string, value any) bool {
	if field == "priority" {
		switch value {
		case "urgent", "high", "medium", "low":
			return true
		}
	} else {
		switch value {
		case "draft", "todo", "in_progress", "blocked", "cancelled":
			return true
		}
	}
	return false
}

func validTaskBatchInput(body map[string]any, project, field string, apply bool) bool {
	hasTarget := apply || field == "priority"
	for key := range body {
		switch key {
		case "project_id", "tasks":
		case field:
			if !hasTarget {
				return false
			}
		case "idempotency_key":
			if !apply {
				return false
			}
		default:
			return false
		}
	}
	if value, exists := body["project_id"]; exists && value != project {
		return false
	}
	if hasTarget && !taskBatchTarget(field, body[field]) {
		return false
	}
	if apply {
		key, ok := body["idempotency_key"].(string)
		if !ok || !handoffCLIOperation(key) {
			return false
		}
	}
	tasks, ok := body["tasks"].([]any)
	if !ok || len(tasks) < 1 || len(tasks) > 16 {
		return false
	}
	seen := make(map[string]bool, len(tasks))
	for _, raw := range tasks {
		task, ok := raw.(map[string]any)
		if !ok || len(task) != 2 || !taskOwnersVersion(task["expected_version"]) {
			return false
		}
		id, ok := task["task_id"].(string)
		if !ok || !taskBatchID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func validTaskBatchResponse(raw []byte, body map[string]any, field string, apply bool) bool {
	// Use the same duplicate and Unicode checks as the request and retain
	// numeric literals so versions are compared as exact integers.
	var syntax map[string]json.RawMessage
	if strictjson.Decode(raw, &syntax, 16<<20) != nil {
		return false
	}
	var response map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&response) != nil || response["project_id"] != body["project_id"] {
		return false
	}
	if apply && (len(response) != 3 || response["operation_id"] != body["idempotency_key"]) || !apply && len(response) != 2 {
		return false
	}
	list := "items"
	if apply {
		list = "changes"
	}
	items, ok := response[list].([]any)
	tasks := body["tasks"].([]any)
	if !ok || len(items) != len(tasks) {
		return false
	}
	for i, value := range items {
		item, ok := value.(map[string]any)
		task := tasks[i].(map[string]any)
		if !ok || item["task_id"] != task["task_id"] || !taskOwnersVersion(item["version"]) {
			return false
		}
		if apply {
			if !validTaskBatchChange(item, task["expected_version"].(json.Number), field, body[field]) {
				return false
			}
		} else if item["version"] != task["expected_version"] || !validTaskBatchPreview(item, field, body[field]) {
			return false
		}
	}
	return true
}

func validTaskBatchPreview(item map[string]any, field string, target any) bool {
	if field == "priority" {
		previous, previousOK := item["previous_priority"].(string)
		changed, changedOK := item["changed"].(bool)
		return len(item) == 5 && previousOK && changedOK && item["priority"] == target && changed == (previous != target)
	}
	status, ok := item["status"].(string)
	if !ok || len(item) != 4 {
		return false
	}
	switch item["eligibility"] {
	case "editable":
		return taskBatchTarget(field, status)
	case "workflow_status", "active_run", "review_required", "unverified_run":
		return true
	}
	return false
}

func validTaskBatchChange(item map[string]any, expected json.Number, field string, target any) bool {
	previous, previousOK := item["previous_"+field].(string)
	changed, changedOK := item["changed"].(bool)
	if len(item) != 6 || !previousOK || !changedOK || item["previous_version"] != expected ||
		item[field] != target || changed != (previous != target) || field == "status" && !taskBatchTarget(field, previous) {
		return false
	}
	version, _ := item["version"].(json.Number).Int64()
	before, _ := expected.Int64()
	if changed && before < math.MaxInt32 {
		return version == before+1
	}
	return !changed && version == before
}

const taskBatchHelpEN = ` — versioned task batch
  preview --file request.json
  apply --file original-confirmed-request.json

Use --file - to read standard input. Put global --project before the command.
Provide 1..16 distinct tasks with task_id and expected_version.
Priority preview also takes priority: urgent, high, medium or low.
Status preview takes only tasks and shows whether each task is eligible.
Status apply takes status: draft, todo, in_progress, blocked or cancelled.
Apply requires idempotency_key. Retain the same body and key for recovery.
Use a current project API credential, or a login session with original-view headers.
`

const taskBatchHelpZH = ` — 带版本校验的任务批量操作
  preview --file request.json
  apply --file original-confirmed-request.json

--file - 从标准输入读取。全局 --project 放在命令前。
tasks 包含 1～16 个不同的 task_id 和各自的 expected_version。
优先级预览还需提供 priority，可选 urgent、high、medium 或 low。
计划状态预览只需 tasks，返回各项是否可以调整。
提交计划状态需提供 status，可选 draft、todo、in_progress、blocked 或 cancelled。
提交必须提供 idempotency_key。恢复时保留原正文和操作编号。
使用当前项目 API 凭据，或附有原视图标识的登录会话。
`
