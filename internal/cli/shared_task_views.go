package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/ai-mada/suma/internal/localization"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const sharedTaskViewUsage = "task-views requires list, get ID, create --file JSON, update ID --file JSON, or archive ID --file JSON; --project is required"

// CLI wire shapes keep the small client independent of the control service.
// Closed decoding rejects duplicate/unknown fields before JSON reserialization.
type sharedTaskViewCLIPreferences struct {
	Q             string `json:"q"`
	Status        string `json:"status"`
	StatusUnset   bool   `json:"status_unset"`
	OwnerID       string `json:"owner_id"`
	Priority      string `json:"priority"`
	RequirementID string `json:"requirement_id"`
	Unassigned    bool   `json:"unassigned"`
	PriorityUnset bool   `json:"priority_unset"`
	Sort          string `json:"sort"`
	Layout        string `json:"layout"`
	GroupBy       string `json:"group_by"`
}
type sharedTaskViewCLIWrite struct {
	ProjectID       string                       `json:"project_id"`
	Name            string                       `json:"name"`
	Visibility      string                       `json:"visibility"`
	Preferences     sharedTaskViewCLIPreferences `json:"preferences"`
	ExpectedVersion int                          `json:"expected_version"`
	IdempotencyKey  string                       `json:"idempotency_key"`
}
type sharedTaskViewCLIArchive struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func sharedTaskViewsCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, sharedTaskViewsHelp(c.locale))
		return err
	}
	if len(args) == 0 || c.project == "" {
		return usage(sharedTaskViewUsage)
	}
	path := "/api/v1/task-views"
	if args[0] == "list" {
		fs := commandFlags("task-views list")
		limit := fs.String("limit", "20", "page size 1..100")
		cursor := fs.String("cursor", "", "original continuation")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
			return usage(sharedTaskViewUsage)
		}
		n, err := strconv.Atoi(*limit)
		if err != nil || n < 1 || n > 100 || strconv.Itoa(n) != *limit || len(*cursor) > 4096 || strings.IndexFunc(*cursor, unicode.IsControl) >= 0 {
			return usage(sharedTaskViewUsage)
		}
		q := url.Values{"limit": {*limit}}
		if *cursor != "" {
			q.Set("cursor", *cursor)
		}
		return contextRead(ctx, c, path+"?"+q.Encode(), out)
	}
	offset, method := 1, http.MethodPost
	switch args[0] {
	case "create":
	case "get", "update", "archive":
		if len(args) < 2 || !contextID(args[1]) || len(args[1]) > 64 {
			return usage(sharedTaskViewUsage)
		}
		path += "/" + url.PathEscape(args[1])
		offset = 2
		if args[0] == "get" {
			if len(args) != 2 {
				return usage(sharedTaskViewUsage)
			}
			return contextRead(ctx, c, path, out)
		}
		if args[0] == "update" {
			method = http.MethodPatch
		} else {
			path += "/archive"
		}
	default:
		return usage(sharedTaskViewUsage)
	}
	fs := commandFlags("task-views " + args[0])
	file := fs.String("file", "", "typed JSON file, or - for stdin")
	if fs.Parse(args[offset:]) != nil || fs.NArg() != 0 || *file == "" {
		return usage(sharedTaskViewUsage)
	}
	reader := in
	if *file != "-" {
		f, err := os.Open(*file)
		if err != nil {
			return usage("cannot open JSON file")
		}
		defer f.Close()
		reader = f
	}
	if reader == nil {
		return usage(sharedTaskViewUsage)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return usage(sharedTaskViewUsage)
	}
	var key string
	if args[0] == "archive" {
		var request sharedTaskViewCLIArchive
		if woc.Decode(raw, &request) != nil || request.ExpectedVersion < 1 {
			return usage(sharedTaskViewUsage)
		}
		key = request.IdempotencyKey
	} else {
		var request sharedTaskViewCLIWrite
		if woc.Decode(raw, &request) != nil || args[0] == "update" && request.ExpectedVersion < 1 || request.Name == "" || request.Visibility != "private" && request.Visibility != "project" {
			return usage(sharedTaskViewUsage)
		}
		key = request.IdempotencyKey
	}
	if len(key) < 8 || len(key) > 128 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return usage(sharedTaskViewUsage)
	}
	body, err := readObject("-", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if args[0] == "create" {
		if _, exists := body["expected_version"]; exists {
			return usage(sharedTaskViewUsage)
		}
	}
	// This typed endpoint accepts project scope only in the mutation body.
	// The generic scoped client also adds a query parameter, which this endpoint
	// correctly rejects; preserve the same explicit scope check locally instead.
	if value, exists := body["project_id"]; exists && value != c.project {
		return usage("JSON project_id differs from --project")
	}
	body["project_id"] = c.project
	// Deliberately never synthesize operation IDs or retry a write automatically.
	response, err := c.request(ctx, method, path, body, false)
	if err != nil {
		return err
	}
	return writeJSON(out, response, c.token)
}

func sharedTaskViewsHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return `
Server task views (global --project goes before task-views):
  task-views list [--limit 1..100] [--cursor ORIGINAL_CURSOR]
  task-views get ID
  task-views create --file request.json
  task-views update ID --file request.json
  task-views archive ID --file request.json
Use --file - for bounded stdin. Writes require the original idempotency_key;
update/archive require the observed expected_version. No automatic retry.
Private views follow your account; project views require current project access.
Changing scope means explicitly creating a new view. No task is dispatched.
`
	}
	return `
服务端任务视图（全局 --project 放在 task-views 前）：
  task-views list [--limit 1..100] [--cursor 原游标]
  task-views get ID
  task-views create --file request.json
  task-views update ID --file request.json
  task-views archive ID --file request.json
--file - 读取有界标准输入。写入须携带原 idempotency_key；更新和归档
须携带所见 expected_version，不自动重试。个人视图属于当前账号；
项目共享视图仍要求当前项目权限。切换范围须明确另存，不派发任务。
`
}
