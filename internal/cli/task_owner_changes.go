package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

const taskOwnersUsage = "task-owners requires targets, preview, or apply; use task-owners --help"
const taskOwnersInvalid = "task owner request requires exact task and owner versions; apply also requires preview_sha256, explicit acknowledgment, and the original operation key"

func taskOwnersCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		text := taskOwnersHelpZH
		if c.locale == localization.EnUS {
			text = taskOwnersHelpEN
		}
		_, err := io.WriteString(out, text)
		return err
	}
	if len(args) == 0 {
		return usage(taskOwnersUsage)
	}
	if args[0] == "targets" {
		flags := commandFlags("task-owners targets")
		query := flags.String("query", "", "literal person name")
		limit := flags.Int("limit", 20, "page size: 1..100")
		cursor := flags.String("cursor", "", "original directory cursor")
		if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !utf8.ValidString(*query) || utf8.RuneCountInString(*query) > 200 || strings.ContainsRune(*query, 0) || *limit < 1 || *limit > 100 || len(*cursor) > 4096 {
			return usage(taskOwnersInvalid)
		}
		values := url.Values{"limit": {strconv.Itoa(*limit)}}
		if *query != "" {
			values.Set("q", *query)
		}
		if *cursor != "" {
			values.Set("cursor", *cursor)
		}
		return contextRead(ctx, c, "/api/v1/task-owner-targets?"+values.Encode(), out)
	}
	if args[0] != "preview" && args[0] != "apply" {
		return usage(taskOwnersUsage)
	}
	flags := commandFlags("task-owners " + args[0])
	file := flags.String("file", "", "exact JSON request file; - reads standard input")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *file == "" {
		return usage(taskOwnersInvalid)
	}
	body, err := readObject(*file, in)
	if err != nil {
		return err
	}
	if !validTaskOwnersBody(body, c.project, args[0] == "apply") {
		return usage(taskOwnersInvalid)
	}
	body["project_id"] = c.project
	path := "/api/v1/task-owner-changes"
	if args[0] == "preview" {
		path += "/preview"
	}
	// The supplied operation is never synthesized or replaced. POST scope is
	// carried only by this body; original-view headers are retained by request.
	raw, err := c.request(ctx, http.MethodPost, path, body, false)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func validTaskOwnersBody(body map[string]any, project string, apply bool) bool {
	if !handoffCLIIdentifier(project, false) {
		return false
	}
	allowed := map[string]bool{"project_id": true, "owner_id": true, "expected_owner_version": true, "tasks": true}
	if apply {
		allowed["preview_sha256"], allowed["acknowledge_invalidation"], allowed["idempotency_key"] = true, true, true
	}
	for key := range body {
		if !allowed[key] {
			return false
		}
	}
	if value, present := body["project_id"]; present && value != project {
		return false
	}
	owner, ok := body["owner_id"].(string)
	if !ok || !handoffCLIIdentifier(owner, false) || !taskOwnersVersion(body["expected_owner_version"]) {
		return false
	}
	tasks, ok := body["tasks"].([]any)
	if !ok || len(tasks) < 1 || len(tasks) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range tasks {
		task, ok := value.(map[string]any)
		if !ok || len(task) != 2 {
			return false
		}
		id, ok := task["task_id"].(string)
		if !ok || !handoffCLIIdentifier(id, false) || seen[id] || !taskOwnersVersion(task["expected_version"]) {
			return false
		}
		seen[id] = true
	}
	if apply {
		sha, ok := body["preview_sha256"].(string)
		if !ok || len(sha) != 64 || sha != strings.ToLower(sha) {
			return false
		}
		if _, err := hex.DecodeString(sha); err != nil {
			return false
		}
		key, ok := body["idempotency_key"].(string)
		if !ok || !handoffCLIOperation(key) || body["acknowledge_invalidation"] != true {
			return false
		}
	}
	return true
}

func taskOwnersVersion(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	n, err := number.Int64()
	return err == nil && n >= 1 && n <= math.MaxInt32
}

const taskOwnersHelpEN = `task-owners — explicit versioned owner changes
  targets [--query TEXT] [--limit 20] [--cursor CURSOR]
  preview --file request.json
  apply --file original-confirmed-request.json

Use --file - for standard input. Global --project is required.
Preview contains owner_id, expected_owner_version and 1..16 explicit tasks
with task_id/expected_version. Review its current preview_sha256 and impacts.
Apply adds that digest, acknowledge_invalidation:true and idempotency_key.
Keep the exact original body and key when an outcome is unknown. This command
does not dispatch tools or approve delivery; unchanged owners are not empty edits.
`

const taskOwnersHelpZH = `task-owners — 明确版本的任务负责人变更
  targets [--query 人员名称] [--limit 20] [--cursor 原游标]
  preview --file request.json
  apply --file original-confirmed-request.json

--file - 从标准输入读取；需要全局 --project。
预览正文包含 owner_id、expected_owner_version，以及1～16项明确的
task_id／expected_version。先审阅返回的 preview_sha256 和影响。
采用正文再明确提供该摘要、acknowledge_invalidation:true、idempotency_key。
结果未知时保留原正文和原操作编号核对；不会自动派发工具或批准交付验收。
负责人不变的条目不会空更新，但仍可能受本次依赖失效影响。
`
