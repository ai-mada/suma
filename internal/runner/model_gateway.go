package runner

import (
	"context"
	"fmt"
	"path/filepath"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func openAttemptModelGateway(ctx context.Context, cancel context.CancelFunc, a Assignment, cfg ExecConfig, workspace *attemptWorkspace, profile tc.Profile, local *localModelExecution) (*mg.Gateway, error) {
	if workspace == nil || local == nil || workspace.Assignment.Attempt != a.Attempt || workspace.Assignment.Fence != a.Fence {
		return nil, fmt.Errorf("model gateway requires its original isolated attempt")
	}
	var persist func(mg.Snapshot) error
	if cfg.Journal != nil {
		persist = func(snapshot mg.Snapshot) error { return cfg.Journal.checkpointModelGateway(a, profile, snapshot) }
	}
	return mg.Open(mg.Config{Context: ctx, Abort: cancel, RequestID: digest([]byte(key(a))), Directory: filepath.Join(workspace.Directory, "model-gateway"), Binding: local.Binding, BaseURL: local.BaseURL, APIKey: local.APIKey, Persist: persist})
}

// SDK progress is diagnostic only. The host replaces all model identity and
// accounting facts with its own durable HTTP receipts before saving a result.
func applyModelGatewayEvidence(evidence *tc.Evidence, snapshot mg.Snapshot) bool {
	model := mg.Evidence(snapshot)
	evidence.ModelExecution = cloneModelEvidence(&model)
	usage, complete := mc.AggregateEvidenceUsage(snapshot.Binding, model)
	evidence.Usage = nil
	evidence.UsageState = tc.UsageUnreported
	if usage != nil {
		evidence.Usage = &tc.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, CachedInputTokens: usage.CachedInputTokens, CacheWriteInputTokens: usage.CacheWriteInputTokens, ReasoningOutputTokens: usage.ReasoningOutputTokens}
		evidence.UsageState = tc.UsagePartial
	}
	if complete {
		evidence.UsageState = tc.UsageComplete
	}
	if evidence.OctopExecution != nil {
		evidence.OctopExecution.BindingSHA256 = snapshot.Binding.BindingSHA256
	}
	return snapshot.FailureCode == "" && mc.EvidenceResponsesComplete(snapshot.Binding, model)
}
