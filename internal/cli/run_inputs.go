package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/strictjson"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

const (
	runInputsUsage           = "run inputs requires RUN and one each of --node NODE, --attempt N and --fence F, with optional --workbench-origin ORIGIN"
	runInputsResponseInvalid = "invalid tool input response or mismatched request scope"
)

// The terminal prints delivery facts only. The original question, answer, and
// identity evidence remain in the separately authorized workbench view.
type runInputSummary struct {
	ID             string                   `json:"id"`
	Protocol       string                   `json:"protocol"`
	State          string                   `json:"state"`
	CreatedAt      string                   `json:"created_at"`
	ExpiresAt      string                   `json:"expires_at"`
	AllowedActions []string                 `json:"allowed_actions"`
	AnswerSaved    bool                     `json:"answer_saved"`
	Delivery       *runInputDeliverySummary `json:"delivery,omitempty"`
}

type runInputDeliverySummary struct {
	AnswerObserved  bool   `json:"answer_observed"`
	WriteState      string `json:"write_state"`
	RequestResolved bool   `json:"request_resolved"`
	ExecutionClosed bool   `json:"execution_closed"`
	FailureReported bool   `json:"failure_reported"`
}

type runInputsSummary struct {
	ProjectID    string            `json:"project_id"`
	RunID        string            `json:"run_id"`
	NodeID       string            `json:"node_id"`
	Attempt      int               `json:"attempt"`
	Fence        int64             `json:"fence"`
	Protocol     string            `json:"protocol"`
	WorkbenchURL string            `json:"workbench_url"`
	Items        []runInputSummary `json:"items"`
}

func parseWorkbenchOrigin(value string) (*url.URL, error) {
	origin, err := url.Parse(value)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Hostname() == "" ||
		origin.User != nil || origin.ForceQuery || origin.RawQuery != "" || strings.Contains(value, "#") ||
		origin.RawPath != "" || (origin.Path != "" && origin.Path != "/") {
		return nil, usage(toolInventoryOriginInvalid)
	}
	return origin, nil
}

func runInputsCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 100) || !compositionCLIIdentifier(c.project, 64) {
		return usage(runInputsUsage)
	}
	run := args[0]
	var node, attemptArg, fenceArg, workbenchOrigin string
	flags := commandFlags("run inputs")
	setOnce := func(target *string, valid func(string) bool) func(string) error {
		return func(value string) error {
			if *target != "" || !valid(value) {
				return errors.New(runInputsUsage)
			}
			*target = value
			return nil
		}
	}
	positive := func(value string) bool {
		_, ok := runWatchDecimal(value, 1, maxRunWatchCursor)
		return ok
	}
	flags.Func("node", "exact workflow node ID", setOnce(&node, func(value string) bool { return compositionCLIIdentifier(value, 150) }))
	flags.Func("attempt", "exact positive attempt number", setOnce(&attemptArg, positive))
	flags.Func("fence", "exact positive attempt fence", setOnce(&fenceArg, positive))
	flags.Func("workbench-origin", "explicit browser origin for the run link", setOnce(&workbenchOrigin, func(value string) bool { return value != "" }))
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || node == "" || attemptArg == "" || fenceArg == "" {
		return usage(runInputsUsage)
	}
	attempt, err := strconv.Atoi(attemptArg)
	if err != nil {
		return usage(runInputsUsage)
	}
	fence, _ := strconv.ParseInt(fenceArg, 10, 64) // Validated at flag parsing.
	if workbenchOrigin == "" {
		workbenchOrigin = c.base
	}
	origin, err := parseWorkbenchOrigin(workbenchOrigin)
	if err != nil {
		return err
	}
	origin.Path = "/"
	origin.RawQuery = url.Values{
		"project": {c.project}, "view": {"development"}, "sub": {"runs"},
		"run": {run}, "node": {node}, "attempt": {attemptArg},
	}.Encode()
	workbenchURL := origin.String()
	if c.token != "" && (strings.Contains(workbenchURL, c.token) || strings.Contains(workbenchOrigin, c.token) ||
		strings.Contains(c.project, c.token) || strings.Contains(run, c.token) || strings.Contains(node, c.token)) {
		return &failure{Code: "response", Message: toolInventoryLinkSensitive}
	}

	query := url.Values{"attempt": {attemptArg}, "fence": {fenceArg}}
	path := "/api/v1/runs/" + url.PathEscape(run) + "/nodes/" + url.PathEscape(node) + "/tool-inputs?" + query.Encode()
	raw, err := c.request(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return err
	}
	summary := runInputsSummary{
		ProjectID: c.project, RunID: run, NodeID: node, Attempt: attempt, Fence: fence, WorkbenchURL: workbenchURL,
	}
	if !summarizeRunInputs(raw, &summary) {
		return &failure{Code: "run_inputs_response_invalid", Message: runInputsResponseInvalid}
	}
	return writeJSON(out, summary, c.token)
}

func summarizeRunInputs(raw []byte, summary *runInputsSummary) bool {
	var page ic.ToolInputList
	if strictjson.Decode(raw, &page, 16<<20) != nil || len(page.Items) > 32 {
		return false
	}
	switch page.Protocol {
	case "", cc.UserInputProtocol, cu.InputProtocol, cl.InputProtocol:
	default:
		return false
	}
	summary.Protocol = page.Protocol
	summary.Items = make([]runInputSummary, 0, len(page.Items))
	seen := make(map[string]bool, len(page.Items))
	for _, item := range page.Items {
		if ic.ValidateInput(item) != nil || item.ProjectID != summary.ProjectID || item.RunID != summary.RunID ||
			item.NodeID != summary.NodeID || item.Attempt != summary.Attempt || item.Fence != summary.Fence || seen[item.ID] {
			return false
		}
		seen[item.ID] = true
		if len(item.AllowedActions) != 0 && inputRequestCapability(item.Request) != page.Protocol {
			return false
		}
		entry := runInputSummary{
			ID: item.ID, Protocol: item.Request.Protocol, State: item.State, CreatedAt: item.CreatedAt,
			ExpiresAt: item.ExpiresAt, AllowedActions: item.AllowedActions, AnswerSaved: item.Answer != nil,
		}
		if observed := item.Observation; observed != nil {
			writeState := "not_started"
			if observed.ReplyWritten {
				writeState = "written"
			} else if observed.ReplyStarted {
				writeState = "started"
			}
			entry.Delivery = &runInputDeliverySummary{
				AnswerObserved: observed.AnswerObserved, WriteState: writeState,
				RequestResolved: observed.RequestResolved, ExecutionClosed: observed.ExecutionClosed,
				FailureReported: observed.FailureCode != "",
			}
		}
		summary.Items = append(summary.Items, entry)
	}
	return true
}

func inputRequestCapability(request ic.ToolInputRequest) string {
	switch {
	case request.Codex != nil:
		return cc.UserInputProtocol
	case request.Cursor != nil:
		return cu.InputProtocol
	case request.Claude != nil:
		return cl.InputProtocol
	default:
		return ""
	}
}
