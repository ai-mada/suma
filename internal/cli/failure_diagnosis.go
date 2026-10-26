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

const diagnosisCLIUsage = "failure-diagnosis requires context, list, get, create, or command; use failure-diagnosis --help"
const diagnosisCLIReadInvalid = "invalid failure-diagnosis read options; provide --run, --node, exact positive --attempt and limit 1..100"
const diagnosisCLIWriteOptions = "failure-diagnosis writes require --file JSON_FILE or --file - and explicit --confirm"
const diagnosisCLIWriteInvalid = "failure-diagnosis requires typed JSON, exact integer versions, fixed source SHA256, explicit acknowledgement and a stable 8..128 character idempotency_key"
const diagnosisCLIBrowserRequired = "failure-diagnosis writes require an existing browser session token and original --expected-user; API-token writes are unsupported and view headers do not grant authority"

type diagnosisCLICreate struct {
	ProjectID            string      `json:"project_id"`
	IssueID              string      `json:"issue_id"`
	ExpectedIssueVersion json.Number `json:"expected_issue_version"`
	RunID                string      `json:"run_id"`
	NodeID               string      `json:"node_id"`
	Attempt              json.Number `json:"attempt"`
	ExpectedSourceSHA256 string      `json:"expected_source_sha256"`
	Goal                 string      `json:"goal"`
	Locale               string      `json:"locale"`
	ToolProfileID        string      `json:"tool_profile_id"`
	ExpectedToolVersion  json.Number `json:"expected_tool_version"`
	MaxSteps             json.Number `json:"max_steps,omitempty"`
	MaxOutputTokens      json.Number `json:"max_output_tokens,omitempty"`
	AcknowledgeModelUse  bool        `json:"acknowledge_model_use"`
	IdempotencyKey       string      `json:"idempotency_key"`
}
type diagnosisCLIRepair struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type diagnosisCLICommand struct {
	ProjectID        string              `json:"project_id"`
	ExpectedVersion  json.Number         `json:"expected_version"`
	Action           string              `json:"action"`
	Repair           *diagnosisCLIRepair `json:"repair,omitempty"`
	Reason           string              `json:"reason"`
	AcknowledgeDraft bool                `json:"acknowledge_draft"`
	IdempotencyKey   string              `json:"idempotency_key"`
}

func diagnosisCLIText(value string, maximum int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maximum && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func diagnosisCLIIdentifier(value string) bool {
	return handoffCLIIdentifier(value, false) && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
func diagnosisCLIFields(body map[string]any, fields string) bool {
	allowed := map[string]bool{}
	for _, name := range strings.Fields(fields) {
		allowed[name] = true
	}
	for name := range body {
		if !allowed[name] {
			return false
		}
	}
	return true
}
func diagnosisCLINumber(body map[string]any, name string, maximum int64, optional bool) bool {
	value, present := body[name]
	if !present {
		return optional
	}
	n, ok := value.(json.Number)
	if !ok {
		return false
	}
	integer, err := n.Int64()
	return err == nil && integer >= 1 && integer <= maximum
}
func diagnosisCLIRepairValid(body map[string]any, repair *diagnosisCLIRepair) bool {
	fields, ok := body["repair"].(map[string]any)
	if !ok || repair == nil || !diagnosisCLIFields(fields, "title description priority acceptance_criteria") || !diagnosisCLIText(repair.Title, 200) || !diagnosisCLIText(repair.Description, 16000) || repair.Priority != "low" && repair.Priority != "medium" && repair.Priority != "high" || len(repair.AcceptanceCriteria) < 1 || len(repair.AcceptanceCriteria) > 16 {
		return false
	}
	for _, criterion := range repair.AcceptanceCriteria {
		if !diagnosisCLIText(criterion, 1000) {
			return false
		}
	}
	return true
}

func diagnosisCLIObject(file string, input io.Reader, action string) (map[string]any, error) {
	reader := input
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return nil, usage("cannot open JSON file")
		}
		defer f.Close()
		reader = f
	}
	if reader == nil {
		return nil, usage(diagnosisCLIWriteInvalid)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || !utf8.Valid(raw) || !tc.ValidJSONUnicode(raw) {
		return nil, usage(diagnosisCLIWriteInvalid)
	}
	// As in team-plan, validate original JSON before map decoding: duplicates,
	// explicit nulls and numeric aliases must not become a different request.
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return usage(diagnosisCLIWriteInvalid)
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return usage(diagnosisCLIWriteInvalid)
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return usage(diagnosisCLIWriteInvalid)
				}
				seen[name] = true
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return usage(diagnosisCLIWriteInvalid)
		}
		_, err = d.Token()
		return err
	}
	if walk(0) != nil {
		return nil, usage(diagnosisCLIWriteInvalid)
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, usage(diagnosisCLIWriteInvalid)
	}
	body, err := readObject("-", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	valid := false
	if action == "create" {
		var q diagnosisCLICreate
		if decoder.Decode(&q) == nil {
			valid = diagnosisCLIFields(body, "project_id issue_id expected_issue_version run_id node_id attempt expected_source_sha256 goal locale tool_profile_id expected_tool_version max_steps max_output_tokens acknowledge_model_use idempotency_key") && diagnosisCLIIdentifier(q.IssueID) && diagnosisCLIIdentifier(q.RunID) && diagnosisCLIIdentifier(q.NodeID) && diagnosisCLIIdentifier(q.ToolProfileID) && tc.ValidDigest(q.ExpectedSourceSHA256) && diagnosisCLIText(q.Goal, 2000) && (q.Locale == "zh-CN" || q.Locale == "en-US") && q.AcknowledgeModelUse && handoffCLIOperation(q.IdempotencyKey)
			for _, name := range []string{"expected_issue_version", "attempt", "expected_tool_version"} {
				valid = valid && diagnosisCLINumber(body, name, 2147483646, false)
			}
			valid = valid && diagnosisCLINumber(body, "max_steps", 1, true) && diagnosisCLINumber(body, "max_output_tokens", 8192, true)
		}
	} else {
		var q diagnosisCLICommand
		if decoder.Decode(&q) == nil {
			valid = diagnosisCLIFields(body, "project_id expected_version action repair reason acknowledge_draft idempotency_key") && diagnosisCLINumber(body, "expected_version", 2147483646, false) && diagnosisCLIText(q.Reason, 2000) && handoffCLIOperation(q.IdempotencyKey)
			switch q.Action {
			case "revise":
				valid = valid && diagnosisCLIRepairValid(body, q.Repair)
			case "adopt":
				_, hasRepair := body["repair"]
				valid = valid && !hasRepair && q.AcknowledgeDraft
			case "dismiss":
				_, hasRepair := body["repair"]
				valid = valid && !hasRepair
			default:
				valid = false
			}
		}
	}
	if !valid {
		return nil, usage(diagnosisCLIWriteInvalid)
	}
	return body, nil
}

func failureDiagnosisCommand(ctx context.Context, c *client, args []string, input io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, failureDiagnosisHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage(diagnosisCLIUsage)
	}
	const base = "/api/v1/failure-diagnosis"
	action := args[0]
	if action == "context" || action == "list" {
		fs := commandFlags("failure-diagnosis " + action)
		run := fs.String("run", "", "original failed run ID")
		node := fs.String("node", "", "exact failed node ID")
		attempt := fs.String("attempt", "", "exact positive attempt")
		limit, before := "20", ""
		if action == "list" {
			fs.StringVar(&limit, "limit", "20", "page size 1..100")
			fs.StringVar(&before, "before", "", "original next_cursor")
		}
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !diagnosisCLIIdentifier(*run) || !diagnosisCLIIdentifier(*node) {
			return usage(diagnosisCLIReadInvalid)
		}
		n, err := strconv.ParseInt(*attempt, 10, 32)
		if err != nil || n < 1 || n > 2147483646 || strconv.FormatInt(n, 10) != *attempt {
			return usage(diagnosisCLIReadInvalid)
		}
		query := url.Values{"run_id": {*run}, "node_id": {*node}, "attempt": {*attempt}}
		path := base + "/context"
		if action == "list" {
			size, err := strconv.Atoi(limit)
			if err != nil || size < 1 || size > 100 || strconv.Itoa(size) != limit || len(before) > 4096 || !utf8.ValidString(before) || strings.IndexFunc(before, unicode.IsControl) >= 0 {
				return usage(diagnosisCLIReadInvalid)
			}
			path = base + "/analyses"
			query.Set("limit", limit)
			if before != "" {
				query.Set("before", before)
			}
		}
		return contextRead(ctx, c, path+"?"+query.Encode(), out)
	}
	if action == "get" {
		if len(args) != 2 || !diagnosisCLIIdentifier(args[1]) {
			return usage(diagnosisCLIReadInvalid)
		}
		return contextRead(ctx, c, base+"/analyses/"+url.PathEscape(args[1]), out)
	}
	offset, path := 1, base+"/analyses"
	if action == "command" {
		if len(args) < 2 || !diagnosisCLIIdentifier(args[1]) {
			return usage(diagnosisCLIReadInvalid)
		}
		offset, path = 2, path+"/"+url.PathEscape(args[1])+"/commands"
	} else if action != "create" {
		return usage(diagnosisCLIUsage)
	}
	fs := commandFlags("failure-diagnosis " + action)
	file := fs.String("file", "", "typed UTF-8 request JSON, or - for stdin")
	confirm := fs.Bool("confirm", false, "confirm the explicit governed write")
	if fs.Parse(args[offset:]) != nil || fs.NArg() != 0 || *file == "" || !*confirm {
		return usage(diagnosisCLIWriteOptions)
	}
	// Presence is a local usability gate, never proof of credential kind. The
	// server independently requires a current browser session and project rights.
	if c.token == "" || c.expectedUser == "" {
		return usage(diagnosisCLIBrowserRequired)
	}
	body, err := diagnosisCLIObject(*file, input, action)
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

func failureDiagnosisHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return diagnosisCLIHelpEN
	}
	return diagnosisCLIHelpZH
}

const diagnosisCLIHelpEN = `
Failure diagnosis (global flags precede the command):
  failure-diagnosis context --run ID --node ID --attempt N
  failure-diagnosis list --run ID --node ID --attempt N [--limit 1..100] [--before NEXT_CURSOR]
  failure-diagnosis get ID
  failure-diagnosis create --file request.json --confirm
  failure-diagnosis command ID --file request.json --confirm

Writes need an existing browser session token and its original --expected-user
(or SUMA_TOKEN/SUMA_EXPECTED_USER). API-token writes are unsupported; expected-user
headers do not grant authority. Server current browser/project rights remain binding.
create requires fixed source SHA, exact versions and acknowledge_model_use=true.
command actions: revise, adopt, dismiss; only revise accepts repair; adopt requires
acknowledge_draft=true. Keep the original idempotency_key and exact payload on recovery.
UTF-8 --file - reads bounded stdin. No automatic model call, adoption, retry or
identity refresh follows a read. Adoption creates only a requirement draft;
it does not repair/retry the original run, accept a task or close an issue.
`
const diagnosisCLIHelpZH = `
失败分诊（全局参数放在命令前）：
  failure-diagnosis context --run ID --node ID --attempt N
  failure-diagnosis list --run ID --node ID --attempt N [--limit 1..100] [--before NEXT_CURSOR]
  failure-diagnosis get ID
  failure-diagnosis create --file request.json --confirm
  failure-diagnosis command ID --file request.json --confirm

写入需要既有浏览器会话凭据及原 --expected-user（或 SUMA_TOKEN/SUMA_EXPECTED_USER）。
不支持 API 凭据写入；expected-user 请求头不授予权限，服务端重核真实浏览器和当前项目权限。
create 必须提供固定来源 SHA、精确版本和 acknowledge_model_use=true。
command 只支持 revise、adopt、dismiss；仅 revise 携带 repair；adopt 必须
acknowledge_draft=true。原操作恢复须保留原 idempotency_key 与精确正文。
--file - 从标准输入读取有界 UTF-8 JSON。读取后不自动调用模型、采用、重试或刷新身份。
采用只创建需求草稿，不修复／重跑原运行、不验收任务、不关闭问题。
`
