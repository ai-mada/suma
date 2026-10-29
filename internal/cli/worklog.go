package cli

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

const worklogCLIUsage = "worklogs requires list, get, create, correct, void, or history; use worklogs --help"
const worklogCLIListInvalid = "worklog list requires --from-date and --to-date (YYYY-MM-DD, at most 31 days), valid filters, and --limit 1..50"
const worklogCLIReadInvalid = "worklog get/history requires a valid worklog ID; history accepts --limit 1..50 and --cursor"
const worklogCLICreateInvalid = "worklog create requires --task, positive --task-version, --member, --date YYYY-MM-DD, --minutes 1..1440, and --operation (8..128 UTF-8 bytes); --note allows up to 200 characters"
const worklogCLICorrectInvalid = "worklog correct requires ID, positive --version and --task-version, --task, --date YYYY-MM-DD, --minutes 1..1440, --reason (1..200 characters), and --operation (8..128 UTF-8 bytes); --note allows up to 200 characters"
const worklogCLIVoidInvalid = "worklog void requires ID, positive --version, --reason (1..200 characters), and --operation (8..128 UTF-8 bytes)"

func worklogCLIText(value string, required bool) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= 200 && (!required || strings.TrimSpace(value) != "")
}

func worklogCLIOperation(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && len(value) >= 8 && len(value) <= 128
}

func worklogCLIDate(value string) (time.Time, bool) {
	date, err := time.Parse("2006-01-02", value)
	return date, err == nil && date.Year() >= 1 && date.Format("2006-01-02") == value
}

func worklogsCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, worklogsHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(worklogCLIUsage)
	}
	const path = "/api/v1/worklogs"
	switch args[0] {
	case "list":
		fs := commandFlags("worklogs list")
		task := fs.String("task", "", "optional task ID")
		member := fs.String("member", "", "optional human project resource ID")
		from := fs.String("from-date", "", "first work date: YYYY-MM-DD")
		to := fs.String("to-date", "", "last work date: YYYY-MM-DD")
		status := fs.String("status", "all", "active, voided, or all")
		limit := fs.Int("limit", 20, "page size: 1..50")
		cursor := fs.String("cursor", "", "original next_cursor")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *limit < 1 || *limit > 50 || *task != "" && !compositionCLIIdentifier(*task, 64) || *member != "" && !compositionCLIIdentifier(*member, 64) || *status != "active" && *status != "voided" && *status != "all" {
			return usage(worklogCLIListInvalid)
		}
		start, startOK := worklogCLIDate(*from)
		end, endOK := worklogCLIDate(*to)
		if !startOK || !endOK || end.Before(start) || end.Sub(start) > 30*24*time.Hour {
			return usage(worklogCLIListInvalid)
		}
		query := url.Values{"from_date": {*from}, "to_date": {*to}, "status": {*status}, "limit": {strconv.Itoa(*limit)}}
		if *task != "" {
			query.Set("task_id", *task)
		}
		if *member != "" {
			query.Set("member_id", *member)
		}
		if *cursor != "" {
			query.Set("cursor", *cursor)
		}
		return contextRead(ctx, c, path+"?"+query.Encode(), out)
	case "get", "history":
		if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) {
			return usage(worklogCLIReadInvalid)
		}
		resource := path + "/" + url.PathEscape(args[1])
		if args[0] == "get" {
			if len(args) != 2 {
				return usage(worklogCLIReadInvalid)
			}
			return contextRead(ctx, c, resource, out)
		}
		fs := commandFlags("worklogs history")
		limit := fs.Int("limit", 20, "page size: 1..50")
		cursor := fs.String("cursor", "", "original next_cursor")
		if fs.Parse(args[2:]) != nil || fs.NArg() != 0 || *limit < 1 || *limit > 50 {
			return usage(worklogCLIReadInvalid)
		}
		query := url.Values{"limit": {strconv.Itoa(*limit)}}
		if *cursor != "" {
			query.Set("cursor", *cursor)
		}
		return contextRead(ctx, c, resource+"/revisions?"+query.Encode(), out)
	case "create", "correct", "void":
		return worklogCLIWrite(ctx, c, args, out)
	default:
		return usage(worklogCLIUsage)
	}
}

func worklogCLIWrite(ctx context.Context, c *client, args []string, out io.Writer) error {
	action, path, options := args[0], "/api/v1/worklogs", args[1:]
	invalid := worklogCLICreateInvalid
	if action != "create" {
		invalid = worklogCLICorrectInvalid
		if action == "void" {
			invalid = worklogCLIVoidInvalid
		}
		if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) {
			return usage(invalid)
		}
		path += "/" + url.PathEscape(args[1])
		if action == "correct" {
			path += "/corrections"
		} else {
			path += "/void"
		}
		options = args[2:]
	}
	fs := commandFlags("worklogs " + action)
	operation := fs.String("operation", "", "original operation key: 8..128 UTF-8 bytes")
	ack := fs.Bool("on-behalf-ack", false, "explicitly acknowledge recording for another human")
	var version, taskVersion, minutes int
	var task, member, date, note, reason string
	if action != "create" {
		fs.IntVar(&version, "version", 0, "current expected worklog version")
		fs.StringVar(&reason, "reason", "", "reason: 1..200 characters")
	}
	if action != "void" {
		fs.StringVar(&task, "task", "", "task ID")
		fs.IntVar(&taskVersion, "task-version", 0, "current expected task version")
		fs.StringVar(&date, "date", "", "work date: YYYY-MM-DD")
		fs.IntVar(&minutes, "minutes", 0, "integer human work minutes: 1..1440")
		fs.StringVar(&note, "note", "", "optional note: up to 200 characters")
	}
	if action == "create" {
		fs.StringVar(&member, "member", "", "human project resource ID")
	}
	if fs.Parse(options) != nil || fs.NArg() != 0 || !worklogCLIOperation(*operation) || action != "create" && (version < 1 || !worklogCLIText(reason, true)) {
		return usage(invalid)
	}
	body := map[string]any{"idempotency_key": *operation, "on_behalf_ack": *ack}
	if action != "create" {
		body["expected_version"], body["reason"] = version, reason
	}
	if action != "void" {
		_, dateOK := worklogCLIDate(date)
		if !compositionCLIIdentifier(task, 64) || taskVersion < 1 || !dateOK || minutes < 1 || minutes > 1440 || !worklogCLIText(note, false) || action == "create" && !compositionCLIIdentifier(member, 64) {
			return usage(invalid)
		}
		body["task_id"], body["expected_task_version"], body["work_date"], body["minutes"], body["note"] = task, taskVersion, date, minutes, note
	}
	if action == "create" {
		body["member_id"] = member
	}
	if c.project == "" {
		return usage("--project is required for this command")
	}
	body["project_id"] = c.project
	raw, err := c.request(ctx, http.MethodPost, path, body, false)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func worklogsHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return worklogsHelpEN
	}
	return worklogsHelpZH
}

const worklogsHelpEN = `
Manual human worklogs (--project and other global flags go before the command):
  worklogs list --from-date YYYY-MM-DD --to-date YYYY-MM-DD [--task ID]
                [--member HUMAN_RESOURCE_ID] [--status active|voided|all]
                [--limit 1..50] [--cursor NEXT_CURSOR]
  worklogs get ID
  worklogs history ID [--limit 1..50] [--cursor NEXT_CURSOR]
  worklogs create --task ID --task-version TASK_VERSION --member HUMAN_RESOURCE_ID
                  --date YYYY-MM-DD --minutes N [--note TEXT] --operation KEY
                  [--on-behalf-ack]
  worklogs correct ID --version WORKLOG_VERSION --task ID --task-version TASK_VERSION
                   --date YYYY-MM-DD --minutes N [--note TEXT] --reason TEXT
                   --operation KEY [--on-behalf-ack]
  worklogs void ID --version WORKLOG_VERSION --reason TEXT --operation KEY [--on-behalf-ack]

Reads return the original API JSON; list requires an inclusive date range of at most
31 days. The default page size is 20; pass the original next_cursor unchanged.
Minutes are integers from 1 to 1440. Notes are optional, up to 200 characters;
correction/void reasons require 1..200 characters. Operation keys use 8..128 UTF-8 bytes.
Create/correct require the current task version; correct/void also require the entry
version. Corrections retain the original human and append a revision; void retains history.
The server checks current access and human-resource eligibility. Recording for another
human requires current management access and explicit --on-behalf-ack; project API
credentials can record only for their own human resource. Unknown results require the
same path, payload and operation key. The CLI makes one request without automatic retry.
These commands explicitly record human work; Agent runs, model usage and machine time
are never converted into human worklogs automatically.
`

const worklogsHelpZH = `
人工工时（--project 等全局参数放在命令前）：
  worklogs list --from-date YYYY-MM-DD --to-date YYYY-MM-DD [--task ID]
                [--member HUMAN_RESOURCE_ID] [--status active|voided|all]
                [--limit 1..50] [--cursor NEXT_CURSOR]
  worklogs get ID
  worklogs history ID [--limit 1..50] [--cursor NEXT_CURSOR]
  worklogs create --task ID --task-version TASK_VERSION --member HUMAN_RESOURCE_ID
                  --date YYYY-MM-DD --minutes N [--note TEXT] --operation KEY
                  [--on-behalf-ack]
  worklogs correct ID --version WORKLOG_VERSION --task ID --task-version TASK_VERSION
                   --date YYYY-MM-DD --minutes N [--note TEXT] --reason TEXT
                   --operation KEY [--on-behalf-ack]
  worklogs void ID --version WORKLOG_VERSION --reason TEXT --operation KEY [--on-behalf-ack]

读取保留服务端原始 JSON；列表必须明确起止日期，包含首尾日期且最多 31 天。
默认每页 20 条，使用服务端原 next_cursor 翻页。分钟数为 1–1440 的整数。
备注可选且最多 200 字；更正／作废理由为 1–200 字；操作标识为 8–128 个 UTF-8 字节。
新建／更正须明确当前任务版本，更正／作废还须明确工时版本。
更正保留原人工资源并新增修订；作废保留历史，不删除记录。
服务端复核当前权限与人工资源资格。代他人登记须有当前管理权限并显式提供
--on-behalf-ack；项目 API 凭证只能为本人关联的人工资源登记。
结果未知时保留相同路径、正文和操作标识；命令仅发起一次请求，不自动重试。
这些命令用于明确登记人工工时；Agent 运行、模型用量和机器耗时不会自动折算为人工工时。
`
