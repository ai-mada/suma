package cli

import (
	"context"
	"flag"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

const taskCollectionCLIUsage = "tasks list or tasks groups requires valid filters; use --q TEXT, --status STATUS or --status-unset, --owner ID or --unassigned, --priority TEXT or --priority-unset, --requirement ID; list accepts --sort, --limit 1..100, and --cursor ORIGINAL_CURSOR; groups accepts --group-by status|owner|priority, --group-limit 1..100, and --group-cursor ORIGINAL_GROUP_CURSOR"

// Collection reads share the control plane's project authorization and cursor.
// One invocation reads one page; no client sorting, filtering or cursor replay.
func taskCollectionCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") || len(args) == 2 && (args[1] == "--help" || args[1] == "help") {
		_, err := io.WriteString(out, taskCollectionHelp(c.locale))
		return err
	}
	if len(args) == 0 || args[0] != "list" && args[0] != "groups" {
		return usage(taskCollectionCLIUsage)
	}
	fs := commandFlags("tasks " + args[0])
	query := fs.String("q", "", "literal project-wide search, at most 200 characters")
	owner := fs.String("owner", "", "resource owner ID")
	unassigned := fs.Bool("unassigned", false, "tasks with no resource owner")
	priority := fs.String("priority", "", "current priority value")
	priorityUnset := fs.Bool("priority-unset", false, "tasks with no priority value")
	requirement := fs.String("requirement", "", "linked requirement ID")
	status := fs.String("status", "", "literal current status; omit for all states")
	statusUnset := fs.Bool("status-unset", false, "tasks with no status value")
	var limit, cursor, sort, groupBy, groupLimit, groupCursor string
	if args[0] == "list" {
		fs.StringVar(&limit, "limit", "20", "page size 1..100")
		fs.StringVar(&cursor, "cursor", "", "original next_cursor for the same filters")
		fs.StringVar(&sort, "sort", "updated_desc", "updated_desc, created_desc, title_asc, or priority_desc")
	} else {
		fs.StringVar(&groupBy, "group-by", "status", "status, owner, or priority")
		fs.StringVar(&groupLimit, "group-limit", "20", "group page size 1..100")
		fs.StringVar(&groupCursor, "group-cursor", "", "original next_group_cursor for the same group query")
	}
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !utf8.ValidString(*query) || utf8.RuneCountInString(*query) > 200 || strings.IndexFunc(*query, unicode.IsControl) >= 0 || !taskCollectionCLIReference(*owner) || !taskCollectionCLIReference(*requirement) || *unassigned && *owner != "" || *priorityUnset && *priority != "" || *statusUnset && *status != "" || !taskCollectionCLIValue(*priority) || !taskCollectionCLIValue(*status) {
		return usage(taskCollectionCLIUsage)
	}
	statusProvided, groupCursorProvided := false, false
	fs.Visit(func(option *flag.Flag) {
		if option.Name == "status" {
			statusProvided = true
		}
		if option.Name == "group-cursor" {
			groupCursorProvided = true
		}
	})
	if statusProvided && *status == "" {
		return usage(taskCollectionCLIUsage)
	}
	q := url.Values{}
	for key, value := range map[string]string{"q": *query, "status": *status, "owner_id": *owner, "priority": *priority, "requirement_id": *requirement} {
		if value != "" {
			q.Set(key, value)
		}
	}
	if *unassigned {
		q.Set("unassigned", "true")
	}
	if *priorityUnset {
		q.Set("priority_unset", "true")
	}
	if *statusUnset {
		q.Set("status_unset", "true")
	}
	path := "/api/v1/task-collection/summary"
	if args[0] == "list" {
		pageSize, err := strconv.Atoi(limit)
		if err != nil || pageSize < 1 || pageSize > 100 || strconv.Itoa(pageSize) != limit || len(cursor) > 4096 || !utf8.ValidString(cursor) || strings.IndexFunc(cursor, taskCollectionCLIControl) >= 0 || sort != "updated_desc" && sort != "created_desc" && sort != "title_asc" && sort != "priority_desc" {
			return usage(taskCollectionCLIUsage)
		}
		path = "/api/v1/task-workspace"
		q.Set("limit", limit)
		q.Set("sort", sort)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
	} else {
		pageSize, err := strconv.Atoi(groupLimit)
		if groupBy != "status" && groupBy != "owner" && groupBy != "priority" || err != nil || pageSize < 1 || pageSize > 100 || strconv.Itoa(pageSize) != groupLimit || groupCursorProvided && groupCursor == "" || len(groupCursor) > 4096 || !utf8.ValidString(groupCursor) || strings.IndexFunc(groupCursor, taskCollectionCLIControl) >= 0 {
			return usage(taskCollectionCLIUsage)
		}
		q.Set("group_by", groupBy)
		q.Set("group_limit", groupLimit)
		if groupCursor != "" {
			q.Set("group_cursor", groupCursor)
		}
	}
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return contextRead(ctx, c, path, out)
}

func taskCollectionCLIControl(value rune) bool { return value < 32 || value == 127 }

func taskCollectionCLIValue(value string) bool {
	return value == "" || utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200 && strings.TrimSpace(value) != "" && strings.IndexFunc(value, taskCollectionCLIControl) < 0
}

func taskCollectionCLIReference(value string) bool {
	return len(value) <= 64 && utf8.ValidString(value) && strings.IndexFunc(value, func(character rune) bool {
		return character < 33 || character == 127 || character == '/' || character == '\\'
	}) < 0
}

func taskCollectionHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return taskCollectionHelpEN
	}
	return taskCollectionHelpZH
}

const taskCollectionHelpEN = `
Project task collection (put --project and other global flags before tasks):
  tasks groups [--q TEXT] [--status STATUS | --status-unset] [--owner RESOURCE_ID | --unassigned]
               [--priority VALUE | --priority-unset] [--requirement ID]
               [--group-by status|owner|priority]
               [--group-limit 1..100] [--group-cursor ORIGINAL_NEXT_GROUP_CURSOR]
  tasks list   [--q TEXT] [--status STATUS | --status-unset] [--owner RESOURCE_ID | --unassigned]
               [--priority VALUE | --priority-unset] [--requirement ID]
               [--sort updated_desc|created_desc|title_asc|priority_desc]
               [--limit 1..100] [--cursor ORIGINAL_NEXT_CURSOR]

The server searches all tasks in the currently authorized project by ID, title
and description, including archived tasks, and returns real group totals.
Search spaces, Unicode and wildcard characters remain literal.
List defaults to 20 rows and reads one page. Reuse its cursor only with the same
project, filters, sort and page size. Groups rejects --sort, --limit and --cursor;
use its separate group pagination flags.
Groups reads one page of headers, default 20 and at most 100. Continue explicitly
with next_group_cursor and the original group query and group page size.
Summary total counts all matched tasks; groups_total counts all matching groups.
Omit --status to include all states; a provided value, including all, is literal.
Use --status-unset for a genuine empty status group; omitted status includes it.
Use --unassigned or --priority-unset for empty owner or priority group keys;
owner IDs such as __unassigned__ remain literal IDs. Groups defaults to status.
Reads preserve current server diagnostics and never start or accept a task.
`

const taskCollectionHelpZH = `
项目任务集合（--project 等全局参数放在 tasks 前）：
  tasks groups [--q 查询] [--status 当前状态 | --status-unset] [--owner 资源ID | --unassigned]
               [--priority 优先级 | --priority-unset] [--requirement 需求ID]
               [--group-by status|owner|priority]
               [--group-limit 1..100] [--group-cursor 原下一组游标]
  tasks list   [--q 查询] [--status 当前状态 | --status-unset] [--owner 资源ID | --unassigned]
               [--priority 优先级 | --priority-unset] [--requirement 需求ID]
               [--sort updated_desc|created_desc|title_asc|priority_desc]
               [--limit 1..100] [--cursor 原下一页游标]

服务端按标识、标题和描述搜索当前有权访问项目的全部任务，包含归档状态，返回真实分组总数。
查询保留空格、Unicode 和通配符字面值；列表默认 20 条，每次仅读一页。
游标必须沿用原项目、筛选、排序及每页条数；groups 不接受 --sort、--limit、--cursor，使用独立分组分页参数。
groups 每次仅读一页分组，默认 20、最多 100 组；继续读取须使用 next_group_cursor，
保留原分组查询与每组页条数。total 是全部匹配任务数，groups_total 是全部匹配分组数。
不提供 --status 时包含全部状态；显式值均为原状态字面值，包括 all。
--status-unset 筛选真实空状态分组；省略状态筛选也包含该分组。
空负责人／优先级分组使用 --unassigned／--priority-unset；__unassigned__ 等负责人标识仍为原标识。
groups 默认按状态分组，也可按负责人或优先级分组。
读取保留服务端当前诊断，不启动任务或改变验收状态。
`
