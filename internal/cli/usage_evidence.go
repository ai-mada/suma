package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	"github.com/ai-mada/suma/internal/toolcontract"
)

const (
	usageEvidenceUsage        = "usage requires attempts or evidence ATTEMPT. See usage --help"
	usageAttemptsUsage        = "usage attempts requires --project and accepts --month YYYY-MM, --state, --limit 1..100, and the original --before cursor"
	usageFactsUsage           = "usage evidence requires --project and an exact attempt ID and accepts --limit 1..100 and the original --before cursor"
	usageEvidenceBrowser      = "usage reads require an existing login session and the original --expected-user. The original --expected-organization is optional"
	usageEvidenceInvalid      = "invalid usage response or mismatched request scope"
	usageEvidenceRejected     = "control rejected the usage evidence read"
	usageEvidenceViewChanged  = "the current account or organization differs from the original request"
	usageEvidenceResponseSize = 16 << 20
)

func usageEvidenceCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	help := func(value string) bool { return value == "help" || value == "--help" || value == "-h" }
	if len(args) == 1 && help(args[0]) {
		return writeUsageEvidenceHelp(c, out)
	}
	if len(args) == 0 || args[0] != "attempts" && args[0] != "evidence" {
		return usage(usageEvidenceUsage)
	}
	action, args := args[0], args[1:]
	if len(args) == 1 && help(args[0]) {
		return writeUsageEvidenceHelp(c, out)
	}
	path, message, attempt := "/api/v1/usage/attempts", usageAttemptsUsage, ""
	if action == "evidence" {
		message = usageFactsUsage
		if len(args) == 0 || !usageEvidenceID(args[0]) || args[0] == "." || strings.Contains(args[0], "..") || strings.HasPrefix(args[0], "-") {
			return usage(message)
		}
		attempt, args = args[0], args[1:]
		path += "/" + url.PathEscape(attempt) + "/facts"
	}
	query := url.Values{}
	set := func(name string, valid func(string) bool) func(string) error {
		return func(value string) error {
			if query.Has(name) || !valid(value) {
				return usage(message)
			}
			query.Set(name, value)
			return nil
		}
	}
	flags := commandFlags("usage " + action)
	flags.Func("limit", "page size from 1 to 100", set("limit", func(value string) bool {
		_, ok := runWatchDecimal(value, 1, 100)
		return ok
	}))
	flags.Func("before", "original next_cursor", set("before", usageEvidenceID))
	if action == "attempts" {
		flags.Func("month", "accounting month in YYYY-MM", set("month", func(value string) bool {
			month, err := time.Parse("2006-01", value)
			return err == nil && month.Format("2006-01") == value
		}))
		flags.Func("state", "attempt accounting state", set("state", usageAttemptState))
	}
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		return writeUsageEvidenceHelp(c, out)
	} else if err != nil || flags.NArg() != 0 || !usageEvidenceID(c.project) {
		return usage(message)
	}
	if strings.TrimSpace(c.token) == "" || c.expectedUser == "" {
		return usage(usageEvidenceBrowser)
	}
	if !query.Has("limit") {
		query.Set("limit", "20")
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	raw, err := c.request(ctx, http.MethodGet, path+"?"+query.Encode(), nil, true)
	invalid := &failure{Code: "usage_evidence_invalid", Message: usageEvidenceInvalid}
	if err != nil {
		if f, ok := err.(*failure); ok {
			if f.remote {
				return usageEvidenceHTTPFailure(f)
			}
			if f.Code == "response" {
				return invalid
			}
		}
		return err
	}
	before := query.Get("before")
	if attempt == "" && !validUsageAttemptsPage(raw, c.project, before, limit) ||
		attempt != "" && !validUsageFactsPage(raw, c.project, attempt, before, limit) {
		return invalid
	}
	return writeJSON(out, raw, c.token)
}

func writeUsageEvidenceHelp(c *client, out io.Writer) error {
	_, err := io.WriteString(out, c.text(`用量与证据（只读）
  usage attempts [--month YYYY-MM] [--state STATE] [--limit 1..100] [--before ID]
  usage evidence ATTEMPT [--limit 1..100] [--before ID]

将 --project PROJECT --expected-user USER [--expected-organization ORG] 放在命令前。
使用已有浏览器登录会话的令牌（建议通过 SUMA_TOKEN）。每页默认 20 条，输出一份 JSON。
省略 --month 时使用服务端当前账期。续页时保留原范围，并将 next_cursor 原值传给 --before。
状态：reserved、settled、partial、unreported、reconciling、confirmed、unpriced。
`, `Usage and evidence (read only)
  usage attempts [--month YYYY-MM] [--state STATE] [--limit 1..100] [--before ID]
  usage evidence ATTEMPT [--limit 1..100] [--before ID]

Put --project PROJECT --expected-user USER [--expected-organization ORG] before the command.
Use an existing browser login session token (prefer SUMA_TOKEN). Each page defaults to 20 and prints one JSON value.
Omit --month to use the server's current accounting month. Keep the original scope and pass next_cursor as --before.
States are reserved, settled, partial, unreported, reconciling, confirmed, and unpriced.
`))
	return err
}

// These identifiers follow the usage endpoint's existing path and cursor contract.
func usageEvidenceID(value string) bool {
	return value != "" && len(value) <= 64 && utf8.ValidString(value) &&
		!strings.ContainsFunc(value, func(r rune) bool { return r < 33 || r == 127 || r == '/' || r == '\\' })
}

func usageAttemptState(state string) bool {
	switch state {
	case "reserved", "settled", "partial", "unreported", "reconciling", "confirmed", "unpriced":
		return true
	}
	return false
}

func validUsageAttemptsPage(raw []byte, project, before string, limit int) bool {
	var page struct {
		Items      []map[string]json.RawMessage `json:"items"`
		Total      int64                        `json:"total"`
		NextCursor string                       `json:"next_cursor"`
	}
	if strictjson.Decode(raw, &page, usageEvidenceResponseSize) != nil || len(page.Items) > limit ||
		!usageSafeCounter(page.Total) {
		return false
	}
	seen := make(map[string]bool, len(page.Items))
	var last string
	// The control owns accounting-month filtering and ordering. A concurrent
	// ledger update can change its total independently of the returned page.
	for _, fields := range page.Items {
		item, ok := decodeUsageAttempt(fields)
		if !ok || item.ProjectID != project || item.ID == before || seen[item.ID] {
			return false
		}
		seen[item.ID] = true
		last = item.ID
	}
	return validUsageContinuation(page.NextCursor, last, len(page.Items), limit)
}

func decodeUsageAttempt(fields map[string]json.RawMessage) (item platform.UsageAttempt, valid bool) {
	// These four counters are required on the wire but explicitly allow null.
	// Adapt only the validation copy to strictjson's non-null contract. Output
	// uses the original validated response and preserves unknown amounts as null.
	for _, name := range []string{"reported_tokens", "estimated_cost_micros", "reserved_cost_micros", "held_cost_micros"} {
		if string(fields[name]) == "null" {
			fields[name] = json.RawMessage("0")
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil || strictjson.Decode(raw, &item, usageEvidenceResponseSize) != nil {
		return item, false
	}
	if !usageEvidenceID(item.ID) || item.RunID == "" || item.NodeID == "" || item.WorkerID == "" || item.ToolProfileID == "" ||
		!validUsageEvidenceStamp(item.CreatedAt) || !validUsageEvidenceStamp(item.UpdatedAt) || !toolcontract.ValidDigest(item.ProfileSHA256) ||
		item.SourceResultSHA256 != "" && !toolcontract.ValidDigest(item.SourceResultSHA256) ||
		!usageAttemptState(item.State) || !validUsageAttemptCoverage(item.UsageState, item.Usage) || !validUsageEndpoint(item.EndpointOrigin) ||
		item.Currency != "" && item.Currency != "CNY" && item.Currency != "USD" ||
		len(item.AllowedActions) > 1 || len(item.AllowedActions) == 1 && item.AllowedActions[0] != "confirm" {
		return item, false
	}
	switch item.UsageSource {
	case "none", "tool_evidence", "manual_confirmation", "host_startup_proof", "host_model_gateway":
	default:
		return item, false
	}
	if item.Version < 1 || item.Attempt < 1 || item.Fence < 1 ||
		!usageSafeCounter(int64(item.Version), int64(item.Attempt), item.Fence, item.ReservedTokens, item.HeldTokens,
			*item.ReportedTokens, *item.EstimatedCostMicros, *item.ReservedCostMicros, *item.HeldCostMicros) {
		return item, false
	}
	if rate := item.Rate; rate != nil && (!usageEvidenceID(rate.ID) || rate.ProjectID != item.ProjectID || rate.ToolProfileID != item.ToolProfileID ||
		rate.ProfileSHA256 != item.ProfileSHA256 || rate.Currency != item.Currency || rate.Model != item.Model || rate.EndpointOrigin != item.EndpointOrigin ||
		rate.Version < 1 || !usageSafeCounter(int64(rate.Version), int64(rate.ModelDeploymentRevision), rate.InputRateMicrosPerMillion,
		rate.OutputRateMicrosPerMillion, rate.InputTokenReservation, rate.OutputTokenReservation)) {
		return item, false
	}
	return item, (item.CodexExecution == nil || codexcontract.ValidateBinding(*item.CodexExecution) == nil) &&
		(item.ModelExecution == nil || modelcontract.ValidateBinding(*item.ModelExecution) == nil)
}

func validUsageAttemptCoverage(state string, usage *toolcontract.Usage) bool {
	switch state {
	case toolcontract.UsageComplete:
		return usage != nil && usage.InputTokens != nil && usage.OutputTokens != nil
	case toolcontract.UsagePartial:
		return usage != nil
	case toolcontract.UsageUnreported:
		// The attempt projection may retain counters from an earlier receipt.
		return true
	}
	return false
}

func validUsageEndpoint(value string) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && value == u.Scheme+"://"+u.Host &&
		(u.Scheme == "https" || u.Scheme == "http")
}

func usageSafeCounter(values ...int64) bool {
	for _, value := range values {
		if value < 0 || value > toolcontract.MaxSafeUsage {
			return false
		}
	}
	return true
}

func validUsageFactsPage(raw []byte, project, attempt, before string, limit int) bool {
	var page platform.UsageAttemptFactPage
	if strictjson.Decode(raw, &page, usageEvidenceResponseSize) != nil || page.ProjectID != project || page.AttemptID != attempt || len(page.Items) > limit {
		return false
	}
	seen := make(map[string]bool, len(page.Items))
	var last string
	for _, item := range page.Items {
		if !validUsageEvidenceStamp(item.CreatedAt) || !usageEvidenceID(item.ID) || !toolcontract.ValidDigest(item.IdentitySHA256) ||
			item.ID == before || seen[item.ID] || !validUsageFact(item) {
			return false
		}
		seen[item.ID] = true
		last = item.ID
	}
	return validUsageContinuation(page.NextCursor, last, len(page.Items), limit)
}

func validUsageFact(item platform.UsageAttemptFact) bool {
	switch item.Kind {
	case "result":
		return validUsageAttemptCoverage(item.UsageState, item.Usage) && (item.UsageState != toolcontract.UsageUnreported || item.Usage == nil)
	case "confirmation":
		return !item.OriginalResult && item.UsageState == toolcontract.UsageComplete && validUsageAttemptCoverage(item.UsageState, item.Usage)
	case "resolution":
		return !item.OriginalResult && item.UsageState == "" && item.Usage == nil
	}
	return false
}

func validUsageEvidenceStamp(value string) bool {
	stamp, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && len(value) >= 20 && len(value) <= 30 && strings.HasSuffix(value, "Z") &&
		value[:19] == stamp.Format("2006-01-02T15:04:05") && (len(value) == 20 || value[19] == '.')
}

func validUsageContinuation(next, last string, count, limit int) bool {
	return next == "" || count == limit && next == last
}

func usageEvidenceHTTPFailure(remote *failure) *failure {
	// Keep only recognized status codes. Never print arbitrary remote bodies.
	response := &failure{Code: "http_error", Status: remote.Status, Message: usageEvidenceRejected}
	switch remote.Status {
	case http.StatusBadRequest:
		response.Code = "invalid"
	case http.StatusUnauthorized:
		response.Code = "unauthenticated"
	case http.StatusForbidden:
		response.Code = "forbidden"
		if remote.Code == "view_context_changed" {
			response.Code, response.Message = "view_context_changed", usageEvidenceViewChanged
		}
	case http.StatusNotFound:
		response.Code = "not_found"
	case http.StatusConflict:
		response.Code = "conflict"
	case http.StatusPreconditionRequired:
		response.Code = "view_context_required"
	case http.StatusServiceUnavailable:
		response.Code = "service_unavailable"
	}
	return response
}
