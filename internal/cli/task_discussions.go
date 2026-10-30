package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

const taskDiscussionsCLIUsage = "discussion requires list --task ID, people --task ID, message MESSAGE_ID --task ID, or post --file JSON|-; use discussion --help"
const taskDiscussionsCLIInput = "discussion post requires original project_id matching --project, task_id, integer expected_task_version, nonblank body, explicit idempotency_key, and optional unique mention_resource_ids or exact knowledge references; no author or role fields"

func taskDiscussionsCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") || len(args) == 2 && (args[1] == "help" || args[1] == "--help") {
		_, err := io.WriteString(out, taskDiscussionsHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(taskDiscussionsCLIUsage)
	}
	if args[0] == "post" {
		fs := commandFlags("discussion post")
		file := fs.String("file", "", "original UTF-8 JSON request file, or - for stdin")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *file == "" {
			return usage(taskDiscussionsCLIUsage)
		}
		body, err := readObject(*file, input)
		if err != nil {
			return err
		}
		if !validTaskDiscussionsCLIPost(body, c.project) {
			return usage(taskDiscussionsCLIInput)
		}
		// This endpoint has a closed, query-free POST contract. Keep the exact
		// caller payload and operation; generic scoped API adds a project query.
		raw, err := c.request(ctx, http.MethodPost, "/api/v1/task-conversations", body, false)
		if err != nil {
			return err
		}
		return writeJSON(out, raw, c.token)
	}
	if args[0] != "list" && args[0] != "people" && args[0] != "message" {
		return usage(taskDiscussionsCLIUsage)
	}
	options, path := args[1:], "/api/v1/task-conversations"
	if args[0] == "people" {
		path = "/api/v1/task-conversation-mention-targets"
	}
	if args[0] == "message" {
		if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) {
			return usage(taskDiscussionsCLIUsage)
		}
		path, options = "/api/v1/task-conversation-messages/"+url.PathEscape(args[1]), args[2:]
	}
	fs := commandFlags("discussion " + args[0])
	task := fs.String("task", "", "exact task in the current project")
	var limit, before, query, cursor string
	if args[0] != "message" {
		fs.StringVar(&limit, "limit", "20", "page size 1..50")
	}
	if args[0] == "list" {
		fs.StringVar(&before, "before", "", "original exclusive message sequence")
	}
	if args[0] == "people" {
		fs.StringVar(&query, "query", "", "literal current people directory query")
		fs.StringVar(&cursor, "cursor", "", "original directory next_cursor")
	}
	if fs.Parse(options) != nil || fs.NArg() != 0 || !compositionCLIIdentifier(*task, 64) {
		return usage(taskDiscussionsCLIUsage)
	}
	q := url.Values{"task_id": {*task}}
	if args[0] != "message" {
		if !taskDiscussionsCLIPositive(limit, 50) {
			return usage(taskDiscussionsCLIUsage)
		}
		q.Set("limit", limit)
	}
	if before != "" {
		if !taskDiscussionsCLIPositive(before, 9007199254740991) {
			return usage(taskDiscussionsCLIUsage)
		}
		q.Set("before", before)
	}
	if args[0] == "people" {
		if !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 || strings.IndexFunc(query, unicode.IsControl) >= 0 || len(cursor) > 4096 || !utf8.ValidString(cursor) || strings.IndexFunc(cursor, unicode.IsControl) >= 0 {
			return usage(taskDiscussionsCLIUsage)
		}
		if query != "" {
			q.Set("q", query)
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
	}
	return contextRead(ctx, c, path+"?"+q.Encode(), out)
}

func taskDiscussionsCLIPositive(value string, maximum int64) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && parsed <= maximum && strconv.FormatInt(parsed, 10) == value
}

func validTaskDiscussionsCLIPost(body map[string]any, project string) bool {
	if project == "" || body["project_id"] != project {
		return false
	}
	for key := range body {
		switch key {
		case "project_id", "task_id", "expected_task_version", "body", "idempotency_key", "mention_resource_ids", "knowledge":
		default:
			return false
		}
	}
	task, taskOK := body["task_id"].(string)
	version, versionOK := body["expected_task_version"].(json.Number)
	text, textOK := body["body"].(string)
	key, keyOK := body["idempotency_key"].(string)
	if !taskOK || !compositionCLIIdentifier(task, 64) || !versionOK || !taskDiscussionsCLIPositive(string(version), 2147483646) || !textOK || !utf8.ValidString(text) || strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) || utf8.RuneCountInString(text) > 8000 || len(text) > 32000 || !keyOK || len(key) < 8 || len(key) > 128 || !utf8.ValidString(key) || strings.TrimSpace(key) == "" || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return false
	}
	if value, exists := body["mention_resource_ids"]; exists {
		ids, ok := value.([]any)
		if !ok || len(ids) > 20 {
			return false
		}
		seen := map[string]bool{}
		for _, value := range ids {
			id, ok := value.(string)
			if !ok || !compositionCLIIdentifier(id, 64) || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	if value, exists := body["knowledge"]; exists {
		refs, ok := value.([]any)
		if !ok || len(refs) > 16 {
			return false
		}
		seen := map[string]bool{}
		for _, raw := range refs {
			ref, ok := raw.(map[string]any)
			if !ok || len(ref) != 3 {
				return false
			}
			id, idOK := ref["id"].(string)
			revision, revisionOK := ref["revision"].(json.Number)
			sha, shaOK := ref["sha256"].(string)
			if !idOK || !compositionCLIIdentifier(id, 64) || seen[id] || !revisionOK || !taskDiscussionsCLIPositive(string(revision), 2147483647) || !shaOK || !compositionCLIHash(sha) {
				return false
			}
			seen[id] = true
		}
	}
	return true
}

func taskDiscussionsHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return taskDiscussionsHelpEN
	}
	return taskDiscussionsHelpZH
}

const taskDiscussionsHelpEN = `
Task discussions (put --project and other global flags before discussion):
  discussion list --task ID [--limit 1..50] [--before ORIGINAL_SEQUENCE]
  discussion people --task ID [--query TEXT] [--limit 1..50] [--cursor ORIGINAL_CURSOR]
  discussion message MESSAGE_ID --task ID
  discussion post --file request.json|-

Reads are one bounded page or one exact message. Posting sends the original JSON
without URL query parameters or a generated operation ID. The file must contain
project_id matching --project, task_id, expected_task_version, body and idempotency_key.
Optional mention_resource_ids retains the exact selected resource-ID order;
optional knowledge contains at most16 exact id/revision/sha256 pins in the original order.
the server rechecks current canonical human recipients. Plain @names remain text.
Posting does not run a model, start development or accept a task. If its response
is unconfirmed, explicitly repeat the same file and original caller to recover
the receipt. Keep --expected-user and --expected-organization for browser commands.
`

const taskDiscussionsHelpZH = `
任务讨论（--project 等全局参数放在 discussion 前）：
  discussion list --task 任务ID [--limit 1..50] [--before 原消息序号]
  discussion people --task 任务ID [--query 查询] [--limit 1..50] [--cursor 原目录游标]
  discussion message 消息ID --task 任务ID
  discussion post --file 请求.json|-

读取每次返回一个有界页或准确消息。发送原JSON时不添加URL查询，不生成新操作ID。
文件须包含与 --project 一致的 project_id，以及 task_id、expected_task_version、body、idempotency_key。
可选 mention_resource_ids 保留所选资源ID的原顺序，由服务端重核当前规范人员；@名字仍是普通正文。
可选 knowledge 包含最多16个准确 id/revision/sha256 引用，保留原顺序，由服务端核验发布资格。
发帖不启动模型、开发执行或任务验收。回执未确认时，使用相同原文件与原调用者明确恢复，
浏览器命令沿用操作时的 --expected-user／--expected-organization。--file - 从标准输入读取。
`
