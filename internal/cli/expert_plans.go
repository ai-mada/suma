package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const expertPlanUsage = "team-plan requires list, get, revisions, create, start, or command; use team-plan --help"
const expertPlanReadInvalid = "invalid team-plan read options; provide a valid ID, revision and limit 1..100"
const expertPlanWriteOptions = "team-plan writes require --file JSON_FILE or --file - and explicit --confirm"
const expertPlanWriteInvalid = "team-plan requires exact positive integer versions, typed items, explicit acknowledgement and a stable 8..128 character idempotency_key"

type expertPlanCLIItem struct {
	ModelExecutionSelection *modelcontract.Selection `json:"model_execution_selection,omitempty"`
	TaskID                  string                   `json:"task_id"`
	ExpectedTaskVersion     json.Number              `json:"expected_task_version"`
	RoleKey                 string                   `json:"role_key"`
}
type expertPlanCLICreate struct {
	ProjectID                    string              `json:"project_id"`
	TeamID                       string              `json:"team_id"`
	ExpectedConfigurationVersion json.Number         `json:"expected_configuration_version"`
	Items                        []expertPlanCLIItem `json:"items"`
	IdempotencyKey               string              `json:"idempotency_key"`
}
type expertPlanCLIStart struct {
	ProjectID           string      `json:"project_id"`
	ExpectedVersion     json.Number `json:"expected_version"`
	AcknowledgeDispatch bool        `json:"acknowledge_dispatch"`
	IdempotencyKey      string      `json:"idempotency_key"`
}
type expertPlanCLICommand struct {
	ProjectID                    string              `json:"project_id"`
	ExpectedVersion              json.Number         `json:"expected_version"`
	Action                       string              `json:"action"`
	Reason                       string              `json:"reason"`
	Acknowledge                  bool                `json:"acknowledge"`
	ExpectedConfigurationVersion json.Number         `json:"expected_configuration_version,omitempty"`
	Items                        []expertPlanCLIItem `json:"items,omitempty"`
	IdempotencyKey               string              `json:"idempotency_key"`
}

func expertPlanInteger(n json.Number) bool {
	v, err := n.Int64()
	return err == nil && v > 0 && v <= 2147483646
}
func expertPlanItems(items []expertPlanCLIItem) bool {
	if len(items) < 1 || len(items) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !handoffCLIIdentifier(item.TaskID, false) || !expertPlanInteger(item.ExpectedTaskVersion) || item.RoleKey == "" || len(item.RoleKey) > 40 || seen[item.TaskID] {
			return false
		}
		for i, r := range item.RoleKey {
			if !(r >= 'a' && r <= 'z' || i > 0 && (r >= '0' && r <= '9' || r == '_' || r == '-')) {
				return false
			}
		}
		if selection := item.ModelExecutionSelection; selection != nil {
			if modelcontract.ValidateSelection(*selection) != nil || selection.Parameters.MaxSteps > 16 || selection.Parameters.MaxOutputTokens > 32768 {
				return false
			}
		}
		seen[item.TaskID] = true
	}
	return true
}

func expertPlanObject(file string, in io.Reader, action string) (map[string]any, error) {
	reader := in
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return nil, usage("cannot open JSON file")
		}
		defer f.Close()
		reader = f
	}
	if reader == nil {
		return nil, usage(expertPlanWriteInvalid)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || !utf8.Valid(raw) || !tc.ValidJSONUnicode(raw) {
		return nil, usage(expertPlanWriteInvalid)
	}
	// Preserve numeric lexemes and reject duplicates before map decoding. This
	// prevents a CLI round trip from turning an ambiguous file into a new request.
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return usage(expertPlanWriteInvalid)
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delimiter == '{' {
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return usage(expertPlanWriteInvalid)
				}
				seen[name] = true
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
		} else if delimiter == '[' {
			for d.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
		} else {
			return usage(expertPlanWriteInvalid)
		}
		_, err = d.Token()
		return err
	}
	if walk(0) != nil {
		return nil, usage(expertPlanWriteInvalid)
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, usage(expertPlanWriteInvalid)
	}
	body, err := readObject("-", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	valid := false
	switch action {
	case "create":
		var q expertPlanCLICreate
		if decoder.Decode(&q) == nil {
			valid = handoffCLIIdentifier(q.TeamID, false) && expertPlanInteger(q.ExpectedConfigurationVersion) && expertPlanItems(q.Items) && handoffCLIOperation(q.IdempotencyKey)
		}
	case "start":
		var q expertPlanCLIStart
		if decoder.Decode(&q) == nil {
			valid = expertPlanInteger(q.ExpectedVersion) && q.AcknowledgeDispatch && handoffCLIOperation(q.IdempotencyKey)
		}
	case "command":
		var q expertPlanCLICommand
		if decoder.Decode(&q) == nil {
			valid = expertPlanInteger(q.ExpectedVersion) && q.Acknowledge && handoffCLIText(q.Reason) && handoffCLIOperation(q.IdempotencyKey)
			switch q.Action {
			case "revise":
				valid = valid && expertPlanInteger(q.ExpectedConfigurationVersion) && expertPlanItems(q.Items)
			case "pause", "resume", "stop":
				_, hasItems := body["items"]
				_, hasConfiguration := body["expected_configuration_version"]
				valid = valid && !hasItems && !hasConfiguration
			default:
				valid = false
			}
		}
	}
	if !valid {
		return nil, usage(expertPlanWriteInvalid)
	}
	return body, nil
}

func expertPlansCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, expertPlansHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(expertPlanUsage)
	}
	const base = "/api/v1/expert-team-plans"
	action := args[0]
	if action == "list" || action == "revisions" {
		offset, path := 1, base
		if action == "revisions" {
			if len(args) < 2 || !handoffCLIIdentifier(args[1], false) {
				return usage(expertPlanReadInvalid)
			}
			offset, path = 2, base+"/"+url.PathEscape(args[1])+"/revisions"
			if len(args) == 3 && args[2] != "" && args[2][0] != '-' {
				revision, err := strconv.ParseInt(args[2], 10, 32)
				if err != nil || revision < 1 || revision > 2147483646 || strconv.FormatInt(revision, 10) != args[2] {
					return usage(expertPlanReadInvalid)
				}
				return contextRead(ctx, c, path+"/"+args[2], out)
			}
		}
		fs := commandFlags("team-plan " + action)
		limit := fs.Int("limit", 20, "page size 1..100")
		before := fs.String("before", "", "next_cursor from the previous page")
		if fs.Parse(args[offset:]) != nil || fs.NArg() != 0 || *limit < 1 || *limit > 100 || !handoffCLIIdentifier(*before, true) {
			return usage(expertPlanReadInvalid)
		}
		query := url.Values{"limit": {strconv.Itoa(*limit)}}
		if *before != "" {
			query.Set("before", *before)
		}
		return contextRead(ctx, c, path+"?"+query.Encode(), out)
	}
	if action == "get" {
		if len(args) != 2 || !handoffCLIIdentifier(args[1], false) {
			return usage(expertPlanReadInvalid)
		}
		return contextRead(ctx, c, base+"/"+url.PathEscape(args[1]), out)
	}
	offset, path := 1, base
	switch action {
	case "create":
	case "start", "command":
		if len(args) < 2 || !handoffCLIIdentifier(args[1], false) {
			return usage(expertPlanReadInvalid)
		}
		offset, path = 2, base+"/"+url.PathEscape(args[1])
		if action == "start" {
			path += "/start"
		} else {
			path += "/commands"
		}
	default:
		return usage(expertPlanUsage)
	}
	fs := commandFlags("team-plan " + action)
	file := fs.String("file", "", "UTF-8 typed request JSON file, or - for stdin")
	confirm := fs.Bool("confirm", false, "confirm this explicit governed write")
	if fs.Parse(args[offset:]) != nil || fs.NArg() != 0 || *file == "" || !*confirm {
		return usage(expertPlanWriteOptions)
	}
	body, err := expertPlanObject(*file, in, action)
	if err != nil {
		return err
	}
	if err = c.operation(http.MethodPost, path, body); err != nil {
		return err
	}
	raw, err := c.request(ctx, http.MethodPost, path, body, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func expertPlansHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return expertPlansHelpEN
	}
	return expertPlansHelpZH
}

const expertPlansHelpEN = `
Expert team delivery plans (global flags precede the command):
  team-plan list [--limit 1..100] [--before NEXT_CURSOR]
  team-plan get ID
  team-plan revisions ID [REVISION | --limit 1..100 --before NEXT_CURSOR]
  team-plan create --file request.json --confirm
  team-plan start|command ID --file request.json --confirm

Typed UTF-8 JSON files support --file - (bounded stdin). Writes require the original
idempotency_key and exact integer versions; no automatic retry or identity refresh.
start requires acknowledge_dispatch=true. command actions: pause, resume, stop, revise;
reason and acknowledge=true are required. Server current authority and CAS remain binding.
Run success never accepts a task. Stop does not stop active processes; no autoaccept.
`
const expertPlansHelpZH = `
专家组交付计划（全局参数放在命令前）：
  team-plan list [--limit 1..100] [--before NEXT_CURSOR]
  team-plan get ID
  team-plan revisions ID [REVISION | --limit 1..100 --before NEXT_CURSOR]
  team-plan create --file request.json --confirm
  team-plan start|command ID --file request.json --confirm

读取有界 UTF-8 类型化 JSON 文件；--file - 从标准输入读取。写入必须保留原
idempotency_key 和精确整数版本，不自动重试或刷新身份。start 须明确
acknowledge_dispatch=true；command 支持 pause、resume、stop、revise，并提供
reason 和 acknowledge=true。当前权限和 CAS 由服务端重核，--confirm 不授予权限。
运行成功不等于任务验收；停止计划不声称停止活动进程，不提供自动验收。
`
