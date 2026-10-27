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
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const compositionCLIUsage = "candidate-compositions requires options, list, get, conflict, revision-context, create, repair, revise-repair, start, or command; task-code-baseline requires get or bind; use --help"
const compositionCLIReadInvalid = "invalid composition read options; provide a valid task or plan ID and limit 1..100"
const compositionCLIWriteOptions = "composition and code-baseline writes require --file JSON_FILE or --file - and explicit --confirm"
const compositionCLIWriteInvalid = "composition requires typed JSON, exact integer versions, ordered 2..16 distinct sources, explicit acknowledgement and a stable 8..128 character idempotency_key"

type compositionCLISource struct {
	TaskID              string      `json:"task_id"`
	ExpectedTaskVersion json.Number `json:"expected_task_version"`
	SourceNodeID        string      `json:"source_node_id"`
}
type compositionCLICreate struct {
	ProjectID           string                 `json:"project_id"`
	TaskID              string                 `json:"task_id"`
	ExpectedTaskVersion json.Number            `json:"expected_task_version"`
	WorkflowID          string                 `json:"workflow_id"`
	WorkflowVersion     json.Number            `json:"workflow_version"`
	ProducerNodeID      string                 `json:"producer_node_id"`
	Sources             []compositionCLISource `json:"sources"`
	IdempotencyKey      string                 `json:"idempotency_key"`
}
type compositionCLIStart struct {
	ProjectID       string      `json:"project_id"`
	ExpectedVersion json.Number `json:"expected_version"`
	Acknowledge     bool        `json:"acknowledge"`
	IdempotencyKey  string      `json:"idempotency_key"`
}
type compositionCLICommand struct {
	ProjectID       string      `json:"project_id"`
	ExpectedVersion json.Number `json:"expected_version"`
	Action          string      `json:"action"`
	Reason          string      `json:"reason"`
	IdempotencyKey  string      `json:"idempotency_key"`
}
type compositionCLIBind struct {
	ProjectID                  string      `json:"project_id"`
	ExpectedTaskVersion        json.Number `json:"expected_task_version"`
	ExpectedBindingVersion     json.Number `json:"expected_binding_version"`
	CompositionID              string      `json:"composition_id"`
	ExpectedCompositionVersion json.Number `json:"expected_composition_version"`
	Acknowledge                bool        `json:"acknowledge"`
	Reason                     string      `json:"reason"`
	IdempotencyKey             string      `json:"idempotency_key"`
}

func compositionCLIIdentifier(value string, maximum int) bool {
	return len(value) <= maximum && contextID(value) && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
func compositionCLIInteger(value json.Number, minimum int64) bool {
	n, err := value.Int64()
	return err == nil && n >= minimum && n <= 2147483646 && strconv.FormatInt(n, 10) == string(value)
}
func compositionCLINumber(body map[string]any, name string, minimum int64) bool {
	value, ok := body[name].(json.Number)
	return ok && compositionCLIInteger(value, minimum)
}

// Keep original numeric lexemes and operation keys. The bounded token walk is
// the same strict ingress policy used by team-plan and failure-diagnosis;
// duplicate keys, null, case aliases and unknown fields never reach the server.
func compositionCLIObject(file string, input io.Reader, action string) (map[string]any, error) {
	reader := input
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return nil, usage("cannot open JSON file")
		}
		defer f.Close()
		reader = f
	}
	invalid := func() (map[string]any, error) {
		if action == "revise-repair" {
			return nil, usage(compositionCLIRevisionInvalid)
		}
		if action == "repair" {
			return nil, usage(compositionCLIRepairInvalid)
		}
		return nil, usage(compositionCLIWriteInvalid)
	}
	if reader == nil {
		return invalid()
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || !utf8.Valid(raw) || !tc.ValidJSONUnicode(raw) {
		return invalid()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) bool
	walk = func(depth int) bool {
		if depth > 32 {
			return false
		}
		token, err := d.Token()
		if err != nil || token == nil {
			return false
		}
		delimiter, container := token.(json.Delim)
		if !container {
			return true
		}
		if delimiter == '{' {
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !walk(depth + 1) {
					return false
				}
			}
		} else if delimiter == '[' {
			for d.More() {
				if !walk(depth + 1) {
					return false
				}
			}
		} else {
			return false
		}
		_, err = d.Token()
		return err == nil
	}
	if !walk(0) {
		return invalid()
	}
	if _, err = d.Token(); err != io.EOF {
		return invalid()
	}
	body, err := readObject("-", bytes.NewReader(raw))
	if err != nil {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	valid := false
	switch action {
	case "create":
		var q compositionCLICreate
		if decoder.Decode(&q) == nil {
			valid = diagnosisCLIFields(body, "project_id task_id expected_task_version workflow_id workflow_version producer_node_id sources idempotency_key") && compositionCLIIdentifier(q.TaskID, 64) && compositionCLINumber(body, "expected_task_version", 1) && compositionCLIIdentifier(q.WorkflowID, 100) && compositionCLINumber(body, "workflow_version", 1) && compositionCLIIdentifier(q.ProducerNodeID, 150) && len(q.Sources) >= 2 && len(q.Sources) <= 16 && handoffCLIOperation(q.IdempotencyKey)
			entries, _ := body["sources"].([]any)
			if len(entries) != len(q.Sources) {
				return invalid()
			}
			seen := map[string]bool{}
			for i, source := range q.Sources {
				entry, ok := entries[i].(map[string]any)
				valid = valid && ok && diagnosisCLIFields(entry, "task_id expected_task_version source_node_id") && compositionCLIIdentifier(source.TaskID, 64) && compositionCLINumber(entry, "expected_task_version", 1) && compositionCLIIdentifier(source.SourceNodeID, 150) && !seen[source.TaskID]
				seen[source.TaskID] = true
			}
		}
	case "repair":
		valid = compositionCLIRepairObject(decoder, body)
	case "revise-repair":
		valid = compositionCLIRevisionObject(decoder, body)
	case "start":
		var q compositionCLIStart
		if decoder.Decode(&q) == nil {
			valid = diagnosisCLIFields(body, "project_id expected_version acknowledge idempotency_key") && compositionCLINumber(body, "expected_version", 1) && q.Acknowledge && handoffCLIOperation(q.IdempotencyKey)
		}
	case "command":
		var q compositionCLICommand
		if decoder.Decode(&q) == nil {
			valid = diagnosisCLIFields(body, "project_id expected_version action reason idempotency_key") && compositionCLINumber(body, "expected_version", 1) && (q.Action == "dismiss" || q.Action == "refresh") && diagnosisCLIText(q.Reason, 2000) && handoffCLIOperation(q.IdempotencyKey)
		}
	case "bind":
		var q compositionCLIBind
		if decoder.Decode(&q) == nil {
			valid = diagnosisCLIFields(body, "project_id expected_task_version expected_binding_version composition_id expected_composition_version acknowledge reason idempotency_key") && compositionCLINumber(body, "expected_task_version", 1) && compositionCLINumber(body, "expected_binding_version", 0) && compositionCLIIdentifier(q.CompositionID, 64) && compositionCLINumber(body, "expected_composition_version", 1) && q.Acknowledge && diagnosisCLIText(q.Reason, 2000) && handoffCLIOperation(q.IdempotencyKey)
		}
	}
	if !valid {
		return invalid()
	}
	return body, nil
}

func candidateCompositionCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, candidateCompositionHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(compositionCLIUsage)
	}
	const base = "/api/v1/candidate-compositions"
	action := args[0]
	if action == "options" || action == "list" {
		fs := commandFlags("candidate-compositions " + action)
		task := fs.String("task", "", "integration task ID")
		limit, before := "20", ""
		if action == "list" {
			fs.StringVar(&limit, "limit", "20", "page size 1..100")
			fs.StringVar(&before, "before", "", "original next_cursor")
		}
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *task != "" && !compositionCLIIdentifier(*task, 64) || action == "options" && *task == "" {
			return usage(compositionCLIReadInvalid)
		}
		path := base
		query := url.Values{}
		if *task != "" {
			query.Set("task_id", *task)
		}
		if action == "options" {
			path += "/options"
		} else {
			size, err := strconv.Atoi(limit)
			if err != nil || size < 1 || size > 100 || strconv.Itoa(size) != limit || len(before) > 4096 || !utf8.ValidString(before) || strings.IndexFunc(before, unicode.IsControl) >= 0 {
				return usage(compositionCLIReadInvalid)
			}
			query.Set("limit", limit)
			if before != "" {
				query.Set("before", before)
			}
		}
		return contextRead(ctx, c, path+"?"+query.Encode(), out)
	}
	if action == "get" || action == "conflict" || action == "revision-context" {
		if len(args) != 2 || !compositionCLIIdentifier(args[1], 64) {
			return usage(compositionCLIReadInvalid)
		}
		path := base + "/" + url.PathEscape(args[1])
		if action == "conflict" {
			path += "/conflict"
		} else if action == "revision-context" {
			path += "/repair-revision-context"
		}
		return contextRead(ctx, c, path, out)
	}
	offset, path := 1, base
	if action == "start" || action == "command" || action == "repair" || action == "revise-repair" {
		if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) {
			return usage(compositionCLIReadInvalid)
		}
		offset = 2
		path += "/" + url.PathEscape(args[1])
		if action == "start" {
			path += "/start"
		} else if action == "repair" {
			path += "/repairs"
		} else if action == "revise-repair" {
			path += "/repair-revisions"
		} else {
			path += "/commands"
		}
	} else if action != "create" {
		return usage(compositionCLIUsage)
	}
	return compositionCLIWrite(ctx, c, action, path, args[offset:], input, out)
}
func taskCodeBaselineCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, candidateCompositionHelp(c.locale))
		return err
	}
	if len(args) < 2 || !compositionCLIIdentifier(args[1], 64) {
		return usage(compositionCLIReadInvalid)
	}
	path := "/api/v1/tasks/" + url.PathEscape(args[1]) + "/code-baseline"
	if args[0] == "get" {
		if len(args) != 2 {
			return usage(compositionCLIReadInvalid)
		}
		return contextRead(ctx, c, path, out)
	}
	if args[0] != "bind" {
		return usage(compositionCLIUsage)
	}
	return compositionCLIWrite(ctx, c, "bind", path, args[2:], input, out)
}
func compositionCLIWrite(ctx context.Context, c *client, action, path string, args []string, input io.Reader, out io.Writer) error {
	fs := commandFlags("candidate composition " + action)
	file := fs.String("file", "", "typed UTF-8 request JSON, or - for stdin")
	confirm := fs.Bool("confirm", false, "confirm this explicit governed write")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *file == "" || !*confirm {
		return usage(compositionCLIWriteOptions)
	}
	body, err := compositionCLIObject(*file, input, action)
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
func candidateCompositionHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return compositionCLIHelpEN
	}
	return compositionCLIHelpZH
}

const compositionCLIHelpEN = `
Governed candidate composition and task code baselines (global flags first):
  candidate-compositions options --task TASK_ID
  candidate-compositions list [--task TASK_ID] [--limit 1..100] [--before NEXT_CURSOR]
  candidate-compositions get PLAN_ID
  candidate-compositions conflict PLAN_ID
  candidate-compositions repair PLAN_ID --file repair.json --confirm
  candidate-compositions revision-context PLAN_ID
  candidate-compositions revise-repair PLAN_ID --file revision.json --confirm
  tasks run-history TASK_ID [--limit 1..100] [--before NEXT_CURSOR]
  candidate-compositions create --file request.json --confirm
  candidate-compositions start|command PLAN_ID --file request.json --confirm
  task-code-baseline get TASK_ID
  task-code-baseline bind TASK_ID --file request.json --confirm

conflict reads the original failure and current eligibility separately. repair explicitly
creates one unstarted repair task and a new plan; it does not execute development.
Use the new repair plan ID for its separately acknowledged start and exact review.
Revision creates a new plan for the same task and never starts it. Certified failed
startup replacement retains the original run; history separates bound and observed facts.
create stores a fixed preview and starts no run. start requires acknowledge=true;
command supports dismiss before start and explicit refresh of an unstarted repair. bind requires acknowledge=true and
explicit task/binding/composition versions; the first expected_binding_version is 0.
Use ordered 2..16 distinct source tasks, exact JSON integers, and the original
8..128 character idempotency_key. UTF-8 --file - reads at most 4 MiB from stdin.
Current scoped credentials and browser original-view headers are reused; neither
--confirm nor view headers grant authority. No automatic retry, identity refresh,
model call, task acceptance, target integration or downstream dispatch occurs.
`
const compositionCLIHelpZH = `
受管候选合成与任务代码基线（全局参数放在命令前）：
  candidate-compositions options --task TASK_ID
  candidate-compositions list [--task TASK_ID] [--limit 1..100] [--before NEXT_CURSOR]
  candidate-compositions get PLAN_ID
  candidate-compositions conflict PLAN_ID
  candidate-compositions repair PLAN_ID --file repair.json --confirm
  candidate-compositions revision-context PLAN_ID
  candidate-compositions revise-repair PLAN_ID --file revision.json --confirm
  tasks run-history TASK_ID [--limit 1..100] [--before NEXT_CURSOR]
  candidate-compositions create --file request.json --confirm
  candidate-compositions start|command PLAN_ID --file request.json --confirm
  task-code-baseline get TASK_ID
  task-code-baseline bind TASK_ID --file request.json --confirm

conflict 分别读取原冲突事实与当前资格；repair 明确创建一个未开始修复任务和新计划，
不启动开发。随后使用新修复计划编号单独确认 start，并独立测试和验收。
revision-context 核对当前修订资格；revise-repair 为同一任务创建新计划，不自动启动。
启动证明支持的失败替换仍保留原运行；run-history 区分正式绑定与后续观察事实。
create 保存固定预览，不启动运行；start 须 acknowledge=true；command 只支持启动前
dismiss；未开始修复计划还可明确 refresh 固定预览。bind 须 acknowledge=true 和明确的任务／绑定／合成版本，首次 expected_binding_version=0。
来源按明确顺序提供 2–16 个不同任务，版本须为精确 JSON 整数，保留原 8–128 字符
idempotency_key。--file - 从标准输入读取最多 4 MiB 的 UTF-8 JSON。
沿用既有当前范围凭据和浏览器原视图；--confirm 与视图请求头均不授予权限。
不自动重试、刷新身份、调用模型、验收任务、集成目标分支或派发下游开发。
`
