package runner

import (
	"encoding/json"
	"fmt"
	"reflect"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The checkpoint contains only host facts. Provider credentials, the local
// gateway token, endpoint paths, prompts and raw model responses never enter it.
type modelGatewayCheckpoint struct {
	Profile  tc.Profile  `json:"profile"`
	Snapshot mg.Snapshot `json:"snapshot"`
}

func validModelGatewayCheckpoint(a Assignment, inboxKey string, checkpoint modelGatewayCheckpoint) error {
	p, snapshot := checkpoint.Profile, checkpoint.Snapshot
	sha, ok := a.Config["tool_profile_sha256"].(string)
	binding, err := mc.DecodeBinding(a.Config["model_execution"])
	if key(a) != inboxKey || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || a.Type != "agent" || len(a.Command) != 1 || !ok || sha != p.ProfileSHA256 || p.CommandID != a.Command[0] || p.ModelExecutionProtocol != mc.Protocol || tc.ValidateProfile(p, true) != nil || err != nil {
		return fmt.Errorf("model gateway checkpoint differs from its durable assignment or profile")
	}
	if snapshot.Version != 1 || snapshot.RequestID != digest([]byte(key(a))) || snapshot.Binding.BindingSHA256 != binding.BindingSHA256 || mc.ValidateBinding(snapshot.Binding) != nil || snapshot.Records == nil || len(snapshot.Records) > binding.Parameters.MaxSteps || snapshot.FailureCode != "" && !tc.ValidCommandID(snapshot.FailureCode) {
		return fmt.Errorf("model gateway checkpoint has invalid binding or request identity")
	}
	for _, record := range snapshot.Records {
		if !mc.ValidDigest(record.ClientSHA256) || record.HTTPStatus != 0 && (record.HTTPStatus < 100 || record.HTTPStatus > 599) || record.State == "completed" && record.HTTPStatus != 200 || snapshot.Closed && record.State == "started" {
			return fmt.Errorf("model gateway checkpoint has invalid original request facts")
		}
	}
	// Evidence converts in-flight requests to unknown without declaring the
	// gateway closed. It also validates exact counters and sequential request IDs.
	if !mc.EvidenceMatchesBinding(binding, mg.Evidence(snapshot)) {
		return fmt.Errorf("model gateway checkpoint has invalid request evidence")
	}
	return nil
}

func sameModelGatewayAssignment(a, original Assignment) bool {
	return key(a) == key(original) && a.TenantID == original.TenantID && a.ProjectID == original.ProjectID && a.WorkerID == original.WorkerID && a.PoolGrantID == original.PoolGrantID && a.PoolGrantVersion == original.PoolGrantVersion && a.Type == original.Type && reflect.DeepEqual(a.Command, original.Command)
}

func cloneModelGatewayCheckpoint(value modelGatewayCheckpoint) (modelGatewayCheckpoint, error) {
	// Record has an explicit decoder: embedding RequestEvidence's decoder would
	// otherwise discard client_sha256 and http_status during this durable clone.
	raw, err := json.Marshal(value)
	if err != nil {
		return modelGatewayCheckpoint{}, err
	}
	var copy modelGatewayCheckpoint
	if err = json.Unmarshal(raw, &copy); err != nil {
		return modelGatewayCheckpoint{}, err
	}
	copy.Profile = tc.CloneProfile(copy.Profile)
	return copy, nil
}

func modelGatewayCheckpointAdvances(previous, next modelGatewayCheckpoint) bool {
	// Closure freezes the complete admission ledger. Repeating exactly the
	// original receipt is safe; closure cannot authorize another request.
	if previous.Snapshot.Closed && !reflect.DeepEqual(previous.Snapshot, next.Snapshot) {
		return false
	}
	if previous.Snapshot.FailureCode != "" && next.Snapshot.FailureCode == "" {
		return false
	}
	if !tc.EqualProfile(previous.Profile, next.Profile) || previous.Snapshot.RequestID != next.Snapshot.RequestID || previous.Snapshot.Binding.BindingSHA256 != next.Snapshot.Binding.BindingSHA256 || previous.Snapshot.Closed && !next.Snapshot.Closed || len(next.Snapshot.Records) < len(previous.Snapshot.Records) {
		return false
	}
	for i, old := range previous.Snapshot.Records {
		current := next.Snapshot.Records[i]
		if old.Step != current.Step || old.RequestSHA256 != current.RequestSHA256 || old.ClientSHA256 != current.ClientSHA256 || old.OutputAllowance != current.OutputAllowance {
			return false
		}
		if old.State != "started" && !reflect.DeepEqual(old, current) {
			return false
		}
	}
	return true
}

// checkpointModelGateway runs after the broker fsyncs its receipt, and before a
// chargeable HTTP request. A journal error restores the prior in-memory fact and
// closes admission; neither persistence layer can authorize blind replay.
func (j *Journal) checkpointModelGateway(a Assignment, profile tc.Profile, snapshot mg.Snapshot) error {
	checkpoint, err := cloneModelGatewayCheckpoint(modelGatewayCheckpoint{Profile: profile, Snapshot: snapshot})
	if err != nil {
		return err
	}
	if err = validModelGatewayCheckpoint(a, key(a), checkpoint); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, exists := j.entries[k]
	if !exists || e.Result != nil || e.ArtifactPending != nil || e.CandidatePending != nil || !sameModelGatewayAssignment(a, e.Assignment) {
		return fmt.Errorf("model gateway assignment is absent, final or has a different original scope")
	}
	if err = validModelGatewayCheckpoint(e.Assignment, k, checkpoint); err != nil {
		return err
	}
	if e.ModelGateway != nil && !modelGatewayCheckpointAdvances(*e.ModelGateway, checkpoint) {
		return fmt.Errorf("model gateway checkpoint cannot replace an original request")
	}
	previous := e
	e.ModelGateway = &checkpoint
	j.entries[k] = e
	if err = j.save(); err != nil {
		j.entries[k] = previous
	}
	return err
}

func recoveredModelGatewayEvidence(a Assignment, checkpoint modelGatewayCheckpoint) (*tc.Evidence, error) {
	if err := validModelGatewayCheckpoint(a, key(a), checkpoint); err != nil {
		return nil, err
	}
	model := mg.Evidence(checkpoint.Snapshot)
	usage, complete := mc.AggregateEvidenceUsage(checkpoint.Snapshot.Binding, model)
	evidence := &tc.Evidence{Profile: tc.CloneProfile(checkpoint.Profile), Terminal: "unknown", UsageState: tc.UsageUnreported, ModelExecution: cloneModelEvidence(&model)}
	if usage != nil {
		evidence.Usage = &tc.Usage{InputTokens: cloneUsageCount(usage.InputTokens), OutputTokens: cloneUsageCount(usage.OutputTokens), CachedInputTokens: cloneUsageCount(usage.CachedInputTokens), CacheWriteInputTokens: cloneUsageCount(usage.CacheWriteInputTokens), ReasoningOutputTokens: cloneUsageCount(usage.ReasoningOutputTokens)}
		evidence.UsageState = tc.UsagePartial
	}
	if complete {
		evidence.UsageState = tc.UsageComplete
	}
	if tc.SupportsOctopExecutionProfile(checkpoint.Profile) {
		// A restart is not an ACP session or stop receipt. Retain only the known
		// model binding; process_stopped and cancel_requested remain false.
		evidence.OctopExecution = &tc.OctopExecutionEvidence{BindingSHA256: model.BindingSHA256}
	}
	if err := tc.ValidateEvidence(*evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}
