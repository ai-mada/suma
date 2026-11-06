package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
)

const dispatchRecommendationOptionsCLIUsage = "tasks recommendation-options requires TASK --expected-version N --workflow ID --workflow-version N --node ID; optional --max-steps, --max-output-tokens, --token-limit and --require-hard-budget (requires --token-limit)"
const dispatchRecommendationOptionsCLIResponseInvalid = "dispatch recommendation options response must preserve the requested task, workflow, node and constraints"

func dispatchRecommendationOptionsCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 64) {
		return usage(dispatchRecommendationOptionsCLIUsage)
	}
	fs := commandFlags("tasks recommendation-options")
	taskVersion := fs.String("expected-version", "", "current task version")
	workflow := fs.String("workflow", "", "fixed workflow ID")
	workflowVersion := fs.String("workflow-version", "", "fixed workflow version")
	node := fs.String("node", "", "development Agent node ID")
	maxSteps := fs.String("max-steps", "8", "maximum execution steps")
	maxOutputTokens := fs.String("max-output-tokens", "4096", "maximum output tokens")
	requireHardBudget := fs.Bool("require-hard-budget", false, "restrict to candidates with enforceable budgets")
	var budget *mc.Budget
	fs.Func("token-limit", "total token budget", func(raw string) error {
		n, valid := recommendationOptionsDecimal(raw, 0, mc.MaxSafeInteger)
		if !valid {
			return usage(dispatchRecommendationOptionsCLIUsage)
		}
		budget = &mc.Budget{TokenLimit: &n}
		return nil
	})
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return usage(dispatchRecommendationOptionsCLIUsage)
	}
	task, taskOK := recommendationOptionsDecimal(*taskVersion, 1, 2147483646)
	version, versionOK := recommendationOptionsDecimal(*workflowVersion, 1, 2147483646)
	steps, stepsOK := recommendationOptionsDecimal(*maxSteps, 1, mc.MaxSteps)
	output, outputOK := recommendationOptionsDecimal(*maxOutputTokens, 1, mc.MaxOutputTokens)
	if !taskOK || !versionOK || !stepsOK || !outputOK ||
		!compositionCLIIdentifier(*workflow, 100) || !compositionCLIIdentifier(*node, 150) ||
		(*requireHardBudget && budget == nil) {
		return usage(dispatchRecommendationOptionsCLIUsage)
	}
	options := platform.DispatchRecommendationOptionsRequest{
		ProjectID: c.project, TaskID: args[0], ExpectedTaskVersion: int(task),
		WorkflowID: *workflow, WorkflowVersion: int(version), AgentNodeID: *node,
		Knowledge: []platform.ProjectKnowledgeReference{},
		Constraints: platform.DispatchRecommendationConstraints{
			MaxSteps: int(steps), MaxOutputTokens: output, RequireHardBudget: *requireHardBudget, Budget: budget,
		},
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/dispatch-recommendations/options", map[string]any{
		"project_id": options.ProjectID, "task_id": options.TaskID, "expected_task_version": options.ExpectedTaskVersion,
		"workflow_id": options.WorkflowID, "workflow_version": options.WorkflowVersion, "agent_node_id": options.AgentNodeID,
		"knowledge": options.Knowledge, "constraints": options.Constraints,
	}, true)
	if err != nil {
		return err
	}
	if !validRecommendationOptionsResponse(raw, options, c.expectedOrganization) {
		return &failure{Code: "response", Message: dispatchRecommendationOptionsCLIResponseInvalid}
	}
	return writeJSON(out, raw, c.token)
}

func recommendationOptionsDecimal(raw string, minimum, maximum int64) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil && n >= minimum && n <= maximum && strconv.FormatInt(n, 10) == raw
}

func validRecommendationOptionsResponse(raw json.RawMessage, options platform.DispatchRecommendationOptionsRequest, organization string) bool {
	const maxResponseBytes = 16 << 20
	const maxCandidates = 24
	// The strict envelope rejects duplicate fields while leaving nullable
	// workflow arrays intact. The execution constraints are a closed object.
	var fields map[string]json.RawMessage
	var source platform.DispatchRecommendationSource
	var constraints platform.DispatchRecommendationConstraints
	if strictjson.Decode(raw, &fields, maxResponseBytes) != nil ||
		!recommendationHistoryCanonicalFields(fields, "sha256", "policy_sha256", "context", "workflow", "agent_node_id", "execution_route", "constraints", "candidates", "coverage") ||
		json.Unmarshal(raw, &source) != nil ||
		strictjson.Decode(fields["constraints"], &constraints, maxResponseBytes) != nil ||
		!reflect.DeepEqual(constraints, options.Constraints) {
		return false
	}
	if source.ExecutionRoute.ProjectID != options.ProjectID ||
		(organization != "" && source.ExecutionRoute.TenantID != organization) ||
		source.Context.Task.ID != options.TaskID || source.Context.Task.Version != options.ExpectedTaskVersion ||
		source.Workflow.ID != options.WorkflowID || source.Workflow.Version != options.WorkflowVersion ||
		source.AgentNodeID != options.AgentNodeID || len(source.Context.Knowledge) != 0 ||
		!mc.ValidDigest(source.SHA256) || !mc.ValidDigest(source.PolicySHA256) ||
		source.Candidates == nil || len(source.Candidates) > maxCandidates || source.Coverage.CandidateCount != len(source.Candidates) ||
		!recommendationOptionsCanonicalSource(fields) {
		return false
	}
	selected := 0
	for _, node := range source.Workflow.Nodes {
		if node.ID == options.AgentNodeID {
			if node.Type != "agent" {
				return false
			}
			selected++
		}
	}
	if selected != 1 {
		return false
	}
	candidates := make(map[string]bool, len(source.Candidates))
	for _, candidate := range source.Candidates {
		if !mc.ValidDigest(candidate.ID) || candidates[candidate.ID] {
			return false
		}
		candidates[candidate.ID] = true
	}
	return true
}

// Check the fields that justify returning the original response. Ordinary Go
// JSON decoding would let a later case variant override the canonical value.
func recommendationOptionsCanonicalSource(fields map[string]json.RawMessage) bool {
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		var value map[string]json.RawMessage
		if json.Unmarshal(raw, &value) != nil {
			return nil
		}
		return value
	}
	canonical := recommendationHistoryCanonicalFields
	taskContext := object(fields["context"])
	workflow := object(fields["workflow"])
	if !canonical(taskContext, "task") || !canonical(object(taskContext["task"]), "id", "version") ||
		!canonical(workflow, "id", "version", "nodes") ||
		!canonical(object(fields["execution_route"]), "project_id", "tenant_id") ||
		!canonical(object(fields["coverage"]), "candidate_count") {
		return false
	}
	for key := range taskContext {
		if key != "knowledge" && strings.EqualFold(key, "knowledge") {
			return false
		}
	}
	var nodes, candidates []map[string]json.RawMessage
	if json.Unmarshal(workflow["nodes"], &nodes) != nil || json.Unmarshal(fields["candidates"], &candidates) != nil {
		return false
	}
	for _, node := range nodes {
		if !canonical(node, "id", "type") {
			return false
		}
	}
	for _, candidate := range candidates {
		if !canonical(candidate, "id") {
			return false
		}
	}
	return true
}
