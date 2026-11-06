package cli

import (
	"context"
	"encoding/base64"
	"io"
	"net/url"
	"regexp"
	"strings"
)

const testPlanCatalogUsage = "test-plan requires list with optional --state draft|published|retired, --task TASK, --limit 1..100, --before CURSOR, or show PLAN. Use test-plan --help."

var testPlanCatalogID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func testPlanCatalogCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") ||
		len(args) == 2 && (args[0] == "list" || args[0] == "show") && args[1] == "--help" {
		_, err := io.WriteString(out, c.text(testPlanCatalogHelpZH, testPlanCatalogHelpEN))
		return err
	}
	if len(args) == 0 {
		return usage(testPlanCatalogUsage)
	}

	const path = "/api/v1/test-plans"
	switch args[0] {
	case "show":
		if len(args) != 2 || !testPlanCatalogID.MatchString(args[1]) || strings.HasPrefix(args[1], "-") {
			return usage(testPlanCatalogUsage)
		}
		return contextRead(ctx, c, path+"/"+url.PathEscape(args[1]), out)
	case "list":
		flags := commandFlags("test-plan list")
		query := url.Values{}
		one := func(key string, valid func(string) bool) func(string) error {
			return func(value string) error {
				if query.Has(key) || !valid(value) {
					return usage(testPlanCatalogUsage)
				}
				query.Set(key, value)
				return nil
			}
		}
		flags.Func("state", "draft, published or retired", one("state", func(value string) bool {
			return value == "draft" || value == "published" || value == "retired"
		}))
		flags.Func("task", "exact task ID", one("task_id", func(value string) bool {
			return testPlanCatalogID.MatchString(value) && !strings.HasPrefix(value, "-")
		}))
		flags.Func("limit", "page size 1..100", one("limit", func(value string) bool {
			_, ok := runWatchDecimal(value, 1, 100)
			return ok
		}))
		flags.Func("before", "next_cursor from the previous page with the same filters", one("before", func(value string) bool {
			return value != "" && len(value) <= 1024
		}))
		if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
			return usage(testPlanCatalogUsage)
		}
		if before := query.Get("before"); before != "" && !validTestPlanBefore(before, query.Has("state")) {
			return usage(testPlanCatalogUsage)
		}
		if !query.Has("limit") {
			query.Set("limit", "20")
		}
		return contextRead(ctx, c, path+"?"+query.Encode(), out)
	default:
		return usage(testPlanCatalogUsage)
	}
}

// The state-filtered cursor is opaque to the CLI. Validate its transport form,
// then forward the original value so the server can verify its scope and anchor.
func validTestPlanBefore(before string, stateFiltered bool) bool {
	if !stateFiltered {
		return testPlanCatalogID.MatchString(before) && !strings.HasPrefix(before, "-")
	}
	// Raw base64 decoding ignores line breaks, which are not valid here.
	if strings.ContainsAny(before, "\r\n") {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(before)
	return err == nil
}

const testPlanCatalogHelpEN = `Read test plans in the current --project.
  test-plan list [--state draft|published|retired] [--task TASK] [--limit 1..100] [--before CURSOR]
  test-plan show PLAN

To continue a list, use its next_cursor with the original project and filters.
`

const testPlanCatalogHelpZH = `读取当前 --project 的测试计划。
  test-plan list [--state draft|published|retired] [--task TASK] [--limit 1..100] [--before CURSOR]
  test-plan show PLAN

续页时请使用服务端返回的 next_cursor，保留原项目和筛选条件。
`
