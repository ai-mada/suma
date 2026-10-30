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

	"github.com/ai-mada/suma/internal/localization"
)

func taskReviewRequestsCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") || len(args) == 2 && (args[1] == "help" || args[1] == "--help") {
		_, err := io.WriteString(out, taskReviewRequestsHelp(c.locale))
		return err
	}
	badUsage := func() error { return usage(taskReviewRequestsCLIText(c.locale, false)) }
	if len(args) == 0 || !compositionCLIIdentifier(c.project, 64) {
		return badUsage()
	}
	action, path, options := args[0], "/api/v1/task-review-requests", args[1:]
	switch action {
	case "get", "cancel":
		if len(options) == 0 || !compositionCLIIdentifier(options[0], 64) {
			return badUsage()
		}
		path += "/" + url.PathEscape(options[0])
		options = options[1:]
		if action == "cancel" {
			path += "/commands"
		}
	case "targets":
		path = "/api/v1/task-review-targets"
	case "list", "create":
	default:
		return badUsage()
	}
	fs := commandFlags("review-requests " + action)
	if action == "create" || action == "cancel" {
		file := fs.String("file", "", "original UTF-8 JSON file, or - for stdin")
		if fs.Parse(options) != nil || fs.NArg() != 0 || *file == "" {
			return badUsage()
		}
		body, err := readObject(*file, input)
		if err != nil {
			return err
		}
		if !validTaskReviewRequestsCLIWrite(body, c.project, action) {
			return usage(taskReviewRequestsCLIText(c.locale, true))
		}
		// The closed POST contract requires the caller's original JSON identity,
		// versions and operation. Generic scoped API adds a project query.
		raw, err := c.request(ctx, http.MethodPost, path, body, false)
		if err != nil {
			return err
		}
		return writeJSON(out, raw, c.token)
	}
	if action == "get" {
		if fs.Parse(options) != nil || fs.NArg() != 0 {
			return badUsage()
		}
		return contextRead(ctx, c, path, out)
	}
	task := fs.String("task", "", "exact task in the current project")
	limit := fs.String("limit", "20", "page size 1..50")
	cursor := fs.String("cursor", "", "original next_cursor; no automatic pagination")
	var query string
	if action == "targets" {
		fs.StringVar(&query, "query", "", "literal current reviewer directory query")
	}
	if fs.Parse(options) != nil || fs.NArg() != 0 || !compositionCLIIdentifier(*task, 64) || !taskDiscussionsCLIPositive(*limit, 50) || !reviewRequestsCLICursor(*cursor) || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 || strings.IndexFunc(query, unicode.IsControl) >= 0 {
		return badUsage()
	}
	q := url.Values{"task_id": {*task}, "limit": {*limit}}
	if *cursor != "" {
		q.Set("cursor", *cursor)
	}
	if query != "" {
		q.Set("q", query)
	}
	return contextRead(ctx, c, path+"?"+q.Encode(), out)
}

func reviewRequestsCLICursor(cursor string) bool {
	return len(cursor) <= 4096 && utf8.ValidString(cursor) && strings.IndexFunc(cursor, unicode.IsControl) < 0
}

func validTaskReviewRequestsCLIWrite(body map[string]any, project, action string) bool {
	if project == "" || body["project_id"] != project {
		return false
	}
	allowed := map[string]bool{"project_id": true, "idempotency_key": true}
	ids, versions, textKey := []string{}, []string{}, "reason"
	if action == "create" {
		ids, versions, textKey = []string{"task_id", "run_id", "reviewer_resource_id"}, []string{"expected_task_version", "expected_run_version"}, "note"
	} else {
		allowed["action"] = true
		versions = []string{"expected_version"}
		if body["action"] != "cancel" {
			return false
		}
	}
	allowed[textKey] = true
	for _, key := range ids {
		allowed[key] = true
		value, ok := body[key].(string)
		maximum := 64
		if key == "run_id" {
			maximum = 100
		}
		if !ok || !compositionCLIIdentifier(value, maximum) {
			return false
		}
	}
	for _, key := range versions {
		allowed[key] = true
		value, ok := body[key].(json.Number)
		if !ok || !taskDiscussionsCLIPositive(string(value), 2147483646) {
			return false
		}
	}
	for key := range body {
		if !allowed[key] {
			return false
		}
	}
	text, textOK := body[textKey].(string)
	key, keyOK := body["idempotency_key"].(string)
	return textOK && utf8.ValidString(text) && strings.TrimSpace(text) != "" && !strings.ContainsRune(text, 0) && utf8.RuneCountInString(text) <= 2000 && len(text) <= 8000 && keyOK && len(key) >= 8 && len(key) <= 128 && utf8.ValidString(key) && strings.TrimSpace(key) != "" && strings.IndexFunc(key, unicode.IsControl) < 0
}

func taskReviewRequestsCLIText(locale localization.Locale, input bool) string {
	if locale == localization.EnUS {
		if input {
			return "review-requests requires original JSON with project_id matching --project, original integer versions, nonblank note/reason, explicit idempotency_key and only the documented fields; cancel action must be cancel"
		}
		return "review-requests requires list --task ID, targets --task ID, get ID, create --file JSON|- or cancel ID --file JSON|-; use review-requests --help"
	}
	if input {
		return "审核责任请求须使用原JSON：project_id与--project一致，保留原整数版本、非空note/reason及明确idempotency_key；只接受合同字段，取消的action必须为cancel"
	}
	return "审核责任请求需要 list --task ID、targets --task ID、get ID、create --file JSON|- 或 cancel ID --file JSON|-；参见 review-requests --help"
}

func taskReviewRequestsHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return taskReviewRequestsHelpEN
	}
	return taskReviewRequestsHelpZH
}

const taskReviewRequestsHelpEN = `
Review responsibility requests (put --project and global flags before review-requests):
  review-requests list --task ID [--limit 1..50] [--cursor ORIGINAL_CURSOR]
  review-requests targets --task ID [--query TEXT] [--limit 1..50] [--cursor ORIGINAL_CURSOR]
  review-requests get REQUEST_ID
  review-requests create --file request.json|-
  review-requests cancel REQUEST_ID --file command.json|-

Reads return one bounded page or one exact request; they never mark inbox items
read. Create JSON requires project_id matching --project, task_id,
expected_task_version, run_id, expected_run_version, reviewer_resource_id, note
and the original explicit idempotency_key. Cancel JSON requires project_id,
expected_version, action:"cancel", reason and the original idempotency_key.
No URL query or operation is added to either POST. --file - reads stdin.
On an unconfirmed response, explicitly repeat the same file, path and original
caller; retain --expected-user/--expected-organization for browser commands.
A request assigns review responsibility without granting authority or starting
a run. Formal acceptance or rejection still uses the existing task Review gate.
`

const taskReviewRequestsHelpZH = `
审核责任请求（--project 等全局参数放在 review-requests 前）：
  review-requests list --task 任务ID [--limit 1..50] [--cursor 原游标]
  review-requests targets --task 任务ID [--query 查询] [--limit 1..50] [--cursor 原游标]
  review-requests get 请求ID
  review-requests create --file 请求.json|-
  review-requests cancel 请求ID --file 命令.json|-

读取只返回一个有界页或准确请求，不自动翻页或标记收件已读。
创建原JSON须包含与--project一致的project_id，以及task_id、expected_task_version、
run_id、expected_run_version、reviewer_resource_id、note和原idempotency_key。
取消原JSON须包含project_id、expected_version、action:"cancel"、reason和原idempotency_key。
两种POST均不添加URL查询或生成操作ID；--file - 从标准输入读取。
回执未确认时，明确重发相同原文件、路径和调用者，浏览器命令保留原
--expected-user／--expected-organization。请求只指定责任，不授予审核权或启动运行；
正式接受／退回仍走原任务Review质量门禁，个人收件状态仍用inbox命令。
`
