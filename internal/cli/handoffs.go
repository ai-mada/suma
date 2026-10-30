package cli

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

func handoffCLIIdentifier(value string, optional bool) bool {
	return value == "" && optional || len(value) <= 64 && contextID(value)
}

func handoffCLIText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 4000 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func handoffCLIOperation(value string) bool {
	return len(value) >= 8 && len(value) <= 128 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func handoffsCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, handoffsHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage("handoffs requires list, get, create, accept, return, or withdraw; use handoffs --help")
	}
	const path = "/api/v1/task-handoffs"
	if args[0] == "get" {
		if len(args) < 2 || !handoffCLIIdentifier(args[1], false) {
			return usage("handoff get requires ID and --task")
		}
		fs := commandFlags("handoffs get")
		task := fs.String("task", "", "exact task ID in the current project")
		if fs.Parse(args[2:]) != nil || fs.NArg() != 0 || !handoffCLIIdentifier(*task, false) {
			return usage("handoff get requires ID and --task")
		}
		query := url.Values{"task_id": {*task}}
		return contextRead(ctx, c, path+"/"+url.PathEscape(args[1])+"?"+query.Encode(), out)
	}
	if args[0] == "list" {
		fs := commandFlags("handoffs list")
		task := fs.String("task", "", "task ID")
		limit := fs.Int("limit", 20, "page size: 1..100")
		before := fs.String("before", "", "next_cursor from this task's previous page")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !handoffCLIIdentifier(*task, false) || !handoffCLIIdentifier(*before, true) || *limit < 1 || *limit > 100 {
			return usage("invalid handoff list options; provide --task and limit 1..100")
		}
		query := url.Values{"task_id": {*task}, "limit": {strconv.Itoa(*limit)}}
		if *before != "" {
			query.Set("before", *before)
		}
		return contextRead(ctx, c, path+"?"+query.Encode(), out)
	}
	if args[0] == "create" {
		fs := commandFlags("handoffs create")
		task := fs.String("task", "", "task ID")
		version := fs.Int("version", 0, "expected task version")
		resource := fs.String("to-resource", "", "actual receiving human project resource ID")
		team := fs.String("to-team", "", "optional active team ID")
		note := fs.String("note", "", "handoff note: 1..4000 UTF-8 bytes")
		operation := fs.String("operation", "", "original operation key: 8..128 characters")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !handoffCLIIdentifier(*task, false) || !handoffCLIIdentifier(*resource, false) || !handoffCLIIdentifier(*team, true) || *version < 1 || !handoffCLIText(*note) || !handoffCLIOperation(*operation) {
			return usage("handoff create requires --task, positive --version, --to-resource, --note, and --operation (8..128 characters)")
		}
		body := map[string]any{"task_id": *task, "expected_task_version": *version, "to_resource_id": *resource, "note": *note, "idempotency_key": *operation}
		if *team != "" {
			body["to_team_id"] = *team
		}
		return handoffCLIWrite(ctx, c, path, body, out)
	}
	if args[0] == "accept" || args[0] == "return" || args[0] == "withdraw" {
		if len(args) < 2 || !handoffCLIIdentifier(args[1], false) {
			return usage("handoff decision requires ID, positive --version, --reason, and --operation (8..128 characters)")
		}
		fs := commandFlags("handoffs " + args[0])
		version := fs.Int("version", 0, "expected handoff version")
		reason := fs.String("reason", "", "decision reason: 1..4000 UTF-8 bytes")
		operation := fs.String("operation", "", "original operation key: 8..128 characters")
		ack := fs.Bool("acknowledge-invalidation", false, "confirm old execution and acceptance evidence will be invalidated")
		if fs.Parse(args[2:]) != nil || fs.NArg() != 0 || *version < 1 || !handoffCLIText(*reason) || !handoffCLIOperation(*operation) {
			return usage("handoff decision requires ID, positive --version, --reason, and --operation (8..128 characters)")
		}
		if args[0] == "accept" && !*ack {
			return usage("handoff accept requires explicit --acknowledge-invalidation")
		}
		body := map[string]any{"action": args[0], "expected_version": *version, "reason": *reason, "idempotency_key": *operation, "acknowledge_invalidation": *ack}
		return handoffCLIWrite(ctx, c, path+"/"+url.PathEscape(args[1])+"/commands", body, out)
	}
	return usage("handoffs requires list, get, create, accept, return, or withdraw; use handoffs --help")
}

func handoffCLIWrite(ctx context.Context, c *client, path string, body map[string]any, out io.Writer) error {
	if err := c.operation(http.MethodPost, path, body); err != nil {
		return err
	}
	raw, err := c.request(ctx, http.MethodPost, path, body, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func handoffsHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return handoffsHelpEN
	}
	return handoffsHelpZH
}

const handoffsHelpEN = `
Task handoffs (global flags, including --project, go before the command):
  handoffs list --task ID [--limit 1..100] [--before NEXT_CURSOR]
  handoffs get HANDOFF_ID --task TASK_ID
  handoffs create --task ID --version TASK_VERSION --to-resource HUMAN_RESOURCE_ID
                  [--to-team TEAM_ID] --note TEXT --operation KEY
  handoffs accept ID --version HANDOFF_VERSION --reason TEXT --operation KEY
                  --acknowledge-invalidation
  handoffs return|withdraw ID --version HANDOFF_VERSION --reason TEXT --operation KEY

List and get are read-only. Get opens the exact handoff in its original task.
Create and decisions use explicit version and operation keys.
Accept transfers responsibility and invalidates previous execution/acceptance evidence;
it does not run or accept the task. Only the designated account may accept/return;
only the sender may withdraw. The server rechecks current access and fixed basis.
On an unknown result, retain the same caller, path, body and operation key.
No automatic retry, account switching, permission grant or task execution occurs.
`

const handoffsHelpZH = `
任务交接（--project 等全局参数放在命令前）：
  handoffs list --task ID [--limit 1..100] [--before NEXT_CURSOR]
  handoffs get HANDOFF_ID --task TASK_ID
  handoffs create --task ID --version TASK_VERSION --to-resource HUMAN_RESOURCE_ID
                  [--to-team TEAM_ID] --note TEXT --operation KEY
  handoffs accept ID --version HANDOFF_VERSION --reason TEXT --operation KEY
                  --acknowledge-invalidation
  handoffs return|withdraw ID --version HANDOFF_VERSION --reason TEXT --operation KEY

list 和 get 只读，get 在原任务范围打开准确交接。发起和决定必须提供明确版本与操作标识。
接收会转移负责人并使旧执行／验收依据失效，不会运行或验收任务。
仅指定账号可接收／退回，仅发起人可撤回；服务端重核当前权限及固定依据。
结果未知时保留原调用者、路径、正文和操作标识；不自动重试、切换账号、授权或执行。
`
