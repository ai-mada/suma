package cli

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

const inboxCLIUsage = "inbox list accepts --filter inbox|unread|archived, --limit 1..50, and --cursor; inbox read|unread|archive|restore requires ID, --version 1..2147483646, and explicit --operation (8..128 UTF-8 bytes); all commands require --project"

func notificationInboxCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") || len(args) == 2 && (args[1] == "help" || args[1] == "--help") {
		_, err := io.WriteString(out, notificationInboxHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(inboxCLIUsage)
	}
	if args[0] == "list" {
		fs := commandFlags("inbox list")
		filter := fs.String("filter", "inbox", "inbox, unread, or archived")
		limit := fs.String("limit", "20", "page size 1..50")
		cursor := fs.String("cursor", "", "original next_cursor for the current inbox scope")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *filter != "inbox" && *filter != "unread" && *filter != "archived" || !notificationInboxCLIInteger(*limit, 50) || len(*cursor) > 4096 || !utf8.ValidString(*cursor) || strings.IndexFunc(*cursor, unicode.IsControl) >= 0 {
			return usage(inboxCLIUsage)
		}
		query := url.Values{"filter": {*filter}, "limit": {*limit}}
		if *cursor != "" {
			query.Set("cursor", *cursor)
		}
		return contextRead(ctx, c, "/api/v1/notification-inbox?"+query.Encode(), out)
	}
	switch args[0] {
	case "read", "unread", "archive", "restore":
	default:
		return usage(inboxCLIUsage)
	}
	if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) {
		return usage(inboxCLIUsage)
	}
	fs := commandFlags("inbox " + args[0])
	version := fs.String("version", "", "original expected notification version")
	operation := fs.String("operation", "", "explicit original operation key: 8..128 UTF-8 bytes")
	if fs.Parse(args[2:]) != nil || fs.NArg() != 0 || !notificationInboxCLIInteger(*version, 2147483646) || len(*operation) < 8 || len(*operation) > 128 || !utf8.ValidString(*operation) || strings.TrimSpace(*operation) == "" || strings.IndexFunc(*operation, unicode.IsControl) >= 0 {
		return usage(inboxCLIUsage)
	}
	if c.project == "" {
		return usage("--project is required for this command")
	}
	expected, _ := strconv.Atoi(*version)
	body := map[string]any{"project_id": c.project, "expected_version": expected, "action": args[0], "idempotency_key": *operation}
	// The caller supplies the original operation identity. Calling the generic
	// key generator here would change recovery after an unconfirmed response.
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/notification-inbox/"+url.PathEscape(args[1])+"/commands", body, false)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func notificationInboxCLIInteger(value string, maximum int) bool {
	parsed, err := strconv.Atoi(value)
	return err == nil && parsed >= 1 && parsed <= maximum && strconv.Itoa(parsed) == value
}

func notificationInboxHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return notificationInboxHelpEN
	}
	return notificationInboxHelpZH
}

const notificationInboxHelpEN = `
Personal notification inbox (put --project and other global flags before inbox):
  inbox list [--filter inbox|unread|archived] [--limit 1..50] [--cursor NEXT_CURSOR]
  inbox read|unread|archive|restore ID --version N --operation ORIGINAL_OPERATION

List defaults to inbox and 20 rows. Each call reads one current authorized page;
continue explicitly using its cursor with the same project, filter and page size.
The server derives the receiver from the current user credential. Runner and
agent identities cannot use this personal inbox. Listing leaves read state unchanged.
Commands affect only the caller's private read or archive state. Restoring retains
the read state, and notification state does not change task execution or acceptance.
--operation must be an explicit 8..128-byte UTF-8 identity without control characters.
Browser commands retain --expected-user and --expected-organization from the original
view. If a response is unconfirmed, reconcile by explicitly repeating the same action,
ID, version, operation, project and original caller. Conflicts require a new decision.
`

const notificationInboxHelpZH = `
个人收件箱（--project 等全局参数放在 inbox 前）：
  inbox list [--filter inbox|unread|archived] [--limit 1..50] [--cursor 下一页游标]
  inbox read|unread|archive|restore 通知ID --version 原版本 --operation 原操作标识

列表默认查看未归档通知，每页 20 条；每次仅读当前授权范围的一页。
继续读取须沿用同一项目、过滤与每页条数，并明确提供原下一页游标。
服务端从当前用户凭据推导接收人，Runner／Agent 身份不能使用个人收件箱。
读取列表保持已读状态；命令只管理本人的已读或归档状态，恢复保留原已读状态。
通知状态不改变任务执行或验收结果。--operation 必须明确提供 8–128 字节的 UTF-8 标识，且不含控制字符。
浏览器命令保留操作时原 --expected-user／--expected-organization。
回执未确认时，使用相同动作、通知ID、版本、操作标识、项目及原调用者明确恢复；版本冲突须重新决策。
`
