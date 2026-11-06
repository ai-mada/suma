package runner

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type entry struct {
	Assignment        Assignment                         `json:"assignment"`
	Result            *Result                            `json:"result,omitempty"`
	Acked             bool                               `json:"acked"`
	ResultRejection   ResultRejection                    `json:"result_rejection,omitempty"`
	Progress          []Progress                         `json:"progress,omitempty"`
	ProgressTruncated bool                               `json:"progress_truncated,omitempty"`
	ArtifactPending   *artifactTransfer                  `json:"artifact_pending,omitempty"`
	CandidatePending  *candidateTransfer                 `json:"candidate_pending,omitempty"`
	ModelGateway      *modelGatewayCheckpoint            `json:"model_gateway,omitempty"`
	WorkspaceTrace    *workspaceTraceCheckpoint          `json:"workspace_trace,omitempty"`
	Terminal          *terminalCheckpoint                `json:"terminal,omitempty"`
	Cloud             *cloudExecutionCheckpoint          `json:"workbuddy_cloud,omitempty"`
	ToolPermissions   *toolPermissionExecutionCheckpoint `json:"tool_permissions,omitempty"`
	ToolInputs        *toolInputExecutionCheckpoint      `json:"tool_inputs,omitempty"`
	Steering          *steeringExecutionCheckpoint       `json:"steering,omitempty"`
}
type Journal struct {
	mu                    sync.Mutex
	path                  string
	entries               map[string]entry
	unlock                func() error
	inventory             *InventoryReport
	claim                 *ClaimCheckpoint
	workspaceObservations map[string]workspaceObservationCheckpoint
	workspaceWrites       workspaceWriteJournal
}

func key(a Assignment) string {
	if a.PoolID != "" {
		b, _ := json.Marshal([]any{a.TenantID, a.ProjectID, a.PoolID, a.PoolRunnerID, a.PoolSlotID, a.PoolCredentialID, a.PoolCredentialVersion, a.DispatchID, a.RunID, a.NodeID, a.Attempt, a.Fence})
		return "pool/" + digest(b)
	}
	return fmt.Sprintf("%s/%s/%d/%d", a.RunID, a.NodeID, a.Attempt, a.Fence)
}
func operationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func OpenJournal(dir string) (*Journal, error) {
	return openJournal(dir, false)
}

// OpenMaintenanceJournal checks original business checkpoints under the same
// execution lock before normal restart handling can create an unknown result.
func OpenMaintenanceJournal(dir string) (*Journal, error) { return openJournal(dir, true) }
func openJournal(dir string, maintenanceOnly bool) (*Journal, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	unlock, err := lockJournal(filepath.Join(dir, "journal.lock"))
	if err != nil {
		return nil, fmt.Errorf("journal already owned or unavailable: %w", err)
	}
	j := &Journal{path: filepath.Join(dir, "journal.json"), entries: map[string]entry{}, unlock: unlock}
	ok := false
	defer func() {
		if !ok {
			_ = j.Close()
		}
	}()
	b, err := os.ReadFile(j.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&j.entries); err != nil || j.entries == nil {
			return nil, fmt.Errorf("journal corrupt; refusing claims: %w", err)
		}
		if decoder.Decode(new(any)) != io.EOF {
			return nil, fmt.Errorf("journal contains trailing data")
		}
		for _, e := range j.entries {
			if !e.validResultDisposition() {
				return nil, fmt.Errorf("journal contains invalid result disposition")
			}
			for _, p := range e.Progress {
				if !validProgressDisposition(p.Acked, p.Rejection, p.HostOperation != nil) {
					return nil, fmt.Errorf("journal contains invalid progress disposition")
				}
			}
		}
	}
	if err = j.loadInventory(); err != nil {
		return nil, err
	}
	if _, err = j.toolInspectionCheckpoint(); err != nil {
		return nil, err
	}
	if _, err = j.installationCheckpoint(); err != nil {
		return nil, err
	}
	if err = j.loadClaim(); err != nil {
		return nil, err
	}
	if err = j.loadWorkspaceObservations(); err != nil {
		return nil, err
	}
	if err = j.loadWorkspaceWrites(); err != nil {
		return nil, err
	}
	if maintenanceOnly {
		if err = j.requireNoPendingBusiness(); err != nil {
			return nil, err
		}
		ok = true
		return j, nil
	}
	for k, e := range j.entries {
		if err = validSteeringCheckpoint(e.Assignment, e.Steering); err != nil {
			return nil, err
		}
		if e.Steering != nil && !e.Steering.NotTaskExecution && !e.Steering.CloseRequested {
			e.Steering.CloseRequested = true
			e.Steering.CloseOperation = operationID()
			j.entries[k] = e
		}
		if err = validToolInputCheckpoint(e.Assignment, e.ToolInputs); err != nil {
			return nil, err
		}
		if err = validNativeRequestCheckpoints(e); err != nil {
			return nil, err
		}
		if err = validToolPermissionCheckpoint(e.Assignment, e.ToolPermissions); err != nil {
			return nil, err
		}
		if e.Cloud != nil {
			if err = validCloudExecution(e.Assignment, *e.Cloud); err != nil || cloudMixed(e) || k != key(e.Assignment) {
				return nil, fmt.Errorf("invalid cloud checkpoint or mixed execution channel")
			}
		}
		if e.Assignment.WorkBuddyCloud != nil && (!cloudNode(e.Assignment) || e.Assignment.WorkBuddyCloud.Validate() != nil || cloudMixed(e)) {
			return nil, fmt.Errorf("invalid protected cloud inbox")
		}
		if e.Assignment.WorkBuddyCloud != nil && e.Result != nil {
			if !validCloudResult(e, *e.Result) {
				return nil, fmt.Errorf("cloud outbox differs from original checkpoint")
			}
			e.Result.Assignment = cloneCloudAssignment(e.Assignment)
			j.entries[k] = e
		}
		if e.Terminal != nil {
			if err = j.validateTerminalCheckpoint(e.Assignment, *e.Terminal); err != nil {
				return nil, err
			}
			if err = j.importTerminalCheckpoint(&e); err != nil {
				return nil, err
			}
			j.entries[k] = e
		}
		if e.WorkspaceTrace != nil {
			if err = j.importWorkspaceTrace(&e); err != nil {
				return nil, err
			}
			j.entries[k] = e
		}
		if e.ModelGateway != nil {
			if err := validModelGatewayCheckpoint(e.Assignment, k, *e.ModelGateway); err != nil {
				return nil, err
			}
		}
		if e.CandidatePending != nil {
			p := *e.CandidatePending
			p.Result.Assignment = p.Assignment
			if e.Result != nil || e.ArtifactPending != nil || key(p.Assignment) != k || candidateRef(p.Assignment) != candidateRef(e.Assignment) || validCandidateCheckpoint(p) != nil {
				return nil, fmt.Errorf("candidate checkpoint differs from durable inbox or original result")
			}
		}
		if e.Assignment.PoolID != "" {
			if err := validPoolAssignment(e.Assignment); err != nil || k != key(e.Assignment) {
				return nil, fmt.Errorf("pool inbox has invalid frozen scope")
			}
			if e.Result != nil {
				if e.Result.RunID != e.Assignment.RunID || e.Result.NodeID != e.Assignment.NodeID || e.Result.Attempt != e.Assignment.Attempt || e.Result.Fence != e.Assignment.Fence || e.Result.WorkerID != e.Assignment.WorkerID {
					return nil, fmt.Errorf("pool outbox differs from its durable inbox")
				}
				// Result's public serializer intentionally omits the full execution
				// envelope. Restore only from this validated immutable inbox.
				e.Result.Assignment = e.Assignment
				j.entries[k] = e
			}
		}
		if e.Result == nil && e.ArtifactPending == nil && e.CandidatePending == nil && e.Assignment.ReleaseExecution == nil && e.Assignment.RepositoryIntegration == nil && e.Assignment.WorkBuddyCloud == nil {
			e.Result = &Result{Assignment: e.Assignment, ExitCode: -1, OutcomeUnknown: true, Error: "runner restart: execution outcome unknown; reconcile required", OperationID: operationID()}
			if e.ModelGateway != nil {
				e.Result.ToolEvidence, err = recoveredModelGatewayEvidence(e.Assignment, *e.ModelGateway)
				if err != nil {
					return nil, err
				}
			}
			j.entries[k] = e
		}
	}
	if err = j.save(); err != nil {
		return nil, err
	}
	ok = true
	return j, nil
}

func (j *Journal) pendingReleases() []Assignment {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	for k, e := range j.entries {
		if e.Result == nil && (e.Assignment.ReleaseExecution != nil || e.Assignment.RepositoryIntegration != nil) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	assignments := make([]Assignment, 0, len(keys))
	for _, k := range keys {
		assignments = append(assignments, j.entries[k].Assignment)
	}
	return assignments
}
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.unlock != nil {
		err := j.unlock()
		j.unlock = nil
		return err
	}
	return nil
}
func (j *Journal) save() error {
	b, err := json.MarshalIndent(j.entries, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(j.path), "journal-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return replaceFile(name, j.path)
}
func (j *Journal) Accept(a Assignment) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	if _, ok := j.entries[k]; ok {
		return fmt.Errorf("attempt already recorded")
	}
	j.entries[k] = entry{Assignment: cloneModelAssignment(a)}
	return j.save()
}
func (j *Journal) Finish(r Result) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(r.Assignment)
	e, ok := j.entries[k]
	if !ok {
		return fmt.Errorf("assignment absent")
	}
	if e.Result != nil {
		return fmt.Errorf("final result already recorded")
	}
	if r.OperationID == "" {
		r.OperationID = operationID()
	}
	previous := e
	if e.Assignment.WorkBuddyCloud != nil {
		if !validCloudResult(e, r) {
			return fmt.Errorf("cloud result requires its original waiting proof")
		}
		r.Assignment = e.Assignment
		proof := *r.WorkBuddyCloudWait
		r.WorkBuddyCloudWait = &proof
	}
	if r.StartupFailure != nil {
		proof := *r.StartupFailure
		proof.Profile = tc.CloneProfile(proof.Profile)
		r.StartupFailure = &proof
	}
	if r.ToolEvidence != nil {
		copied := *r.ToolEvidence
		copied.Profile = tc.CloneProfile(copied.Profile)
		copied.Usage = cloneUsage(r.ToolEvidence.Usage)
		copied.ModelExecution = cloneModelEvidence(r.ToolEvidence.ModelExecution)
		copied.CodexExecution = cc.CloneEvidence(r.ToolEvidence.CodexExecution)
		copied.OpenCodeExecution = cloneOpenCodeEvidence(r.ToolEvidence.OpenCodeExecution)
		copied.OctopExecution = cloneOctopEvidence(r.ToolEvidence.OctopExecution)
		copied.ACPExecution = cloneACPEvidence(r.ToolEvidence.ACPExecution)
		r.ToolEvidence = &copied
	}
	r.Assignment = cloneModelAssignment(r.Assignment)
	e.Result = &r
	e.ArtifactPending = nil
	e.CandidatePending = nil
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}

func (j *Journal) checkpointArtifact(p artifactTransfer) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(p.Assignment)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.Assignment.WorkerID != p.Assignment.WorkerID {
		return fmt.Errorf("artifact assignment absent or final")
	}
	if e.ArtifactPending != nil {
		return fmt.Errorf("artifact execution already checkpointed")
	}
	previous := e
	e.ArtifactPending = &p
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}

func (j *Journal) pendingArtifacts() []artifactTransfer {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	for k, e := range j.entries {
		if e.ArtifactPending != nil && e.Result == nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	result := make([]artifactTransfer, 0, len(keys))
	for _, k := range keys {
		result = append(result, *j.entries[k].ArtifactPending)
	}
	return result
}

// RecordProgress makes a chunk durable before any network request. Its ordinal
// is part of a deterministic operation ID, retained through process restarts.
func (j *Journal) RecordProgress(a Assignment, message string) error {
	return j.recordProgress(a, message, nil)
}
func (j *Journal) RecordToolProgress(a Assignment, event tc.Event) error {
	if err := tc.ValidateEvent(event); err != nil {
		return err
	}
	return j.recordProgress(a, "", &event)
}
func (j *Journal) recordProgress(a Assignment, message string, event *tc.Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Assignment.WorkerID != a.WorkerID {
		return fmt.Errorf("assignment absent")
	}
	if e.Result != nil {
		return fmt.Errorf("final result already recorded")
	}
	message = progressText(message, nil)
	if strings.TrimSpace(message) == "" && event == nil {
		return nil
	}
	sdkProgress := 0
	for _, p := range e.Progress {
		if p.HostOperation == nil {
			sdkProgress++
		}
	}
	if sdkProgress >= 1024 {
		if !e.ProgressTruncated {
			previous := e
			e.ProgressTruncated = true
			j.entries[k] = e
			if err := j.save(); err != nil {
				j.entries[k] = previous
				return err
			}
		}
		return nil
	}
	previous := e
	e.Progress = append([]Progress(nil), e.Progress...)
	h := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/progress/%d", k, a.WorkerID, len(e.Progress)+1)))
	if event != nil {
		event = cloneToolEvent(event)
	}
	e.Progress = append(e.Progress, Progress{Assignment: cloneModelAssignment(a), OperationID: hex.EncodeToString(h[:]), Message: message, ToolEvent: event})
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}

func cloneUsageCount(v *int64) *int64 {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}
func cloneUsage(v *tc.Usage) *tc.Usage {
	if v == nil {
		return nil
	}
	return &tc.Usage{InputTokens: cloneUsageCount(v.InputTokens), CachedInputTokens: cloneUsageCount(v.CachedInputTokens), CacheWriteInputTokens: cloneUsageCount(v.CacheWriteInputTokens), OutputTokens: cloneUsageCount(v.OutputTokens), ReasoningOutputTokens: cloneUsageCount(v.ReasoningOutputTokens)}
}
func cloneToolEvent(v *tc.Event) *tc.Event {
	if v == nil {
		return nil
	}
	copy := *v
	copy.Usage = cloneUsage(v.Usage)
	if v.Diagnostic != nil {
		diagnostic := *v.Diagnostic
		if v.Diagnostic.MessageParams != nil {
			diagnostic.MessageParams = make(map[string]any, len(v.Diagnostic.MessageParams))
			for key, value := range v.Diagnostic.MessageParams {
				diagnostic.MessageParams[key] = value
			}
		}
		copy.Diagnostic = &diagnostic
	}
	return &copy
}

func validProgressDisposition(acked bool, rejection ProgressRejection, hostOperation bool) bool {
	return rejection == "" || rejection == ProgressRejectedStaleAttempt && !acked && !hostOperation
}

func (p Progress) pending() bool {
	return !p.Acked && p.Rejection == ""
}

func (j *Journal) PendingProgress() []Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := make([]string, 0, len(j.entries))
	for k := range j.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := []Progress{}
	for _, k := range keys {
		for _, p := range j.entries[k].Progress {
			if p.pending() {
				p.ToolEvent = cloneToolEvent(p.ToolEvent)
				p.HostOperation = cloneWorkspaceEvent(p.HostOperation)
				p.Assignment = cloneModelAssignment(p.Assignment)
				result = append(result, p)
			}
		}
	}
	return result
}
func (j *Journal) AckProgress(id string) error {
	return j.finalizeProgress(id, "")
}

// RejectStaleProgress retains the original progress and records a terminal
// rejection after the control plane proves its attempt is stale. This is not an
// acknowledgement and does not change the business result.
func (j *Journal) RejectStaleProgress(id string) error {
	return j.finalizeProgress(id, ProgressRejectedStaleAttempt)
}

func (j *Journal) finalizeProgress(id string, rejection ProgressRejection) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for k, e := range j.entries {
		for i, p := range e.Progress {
			if p.OperationID == id {
				acked := rejection == ""
				if !validProgressDisposition(acked, rejection, p.HostOperation != nil) {
					return fmt.Errorf("invalid progress disposition")
				}
				if !p.pending() {
					if p.Acked == acked && p.Rejection == rejection {
						return nil
					}
					return fmt.Errorf("progress already has a different disposition")
				}
				previous := e
				e.Progress = append([]Progress(nil), e.Progress...)
				e.Progress[i].Acked = acked
				e.Progress[i].Rejection = rejection
				j.entries[k] = e
				if err := j.save(); err != nil {
					j.entries[k] = previous
					return err
				}
				return nil
			}
		}
	}
	return fmt.Errorf("progress absent")
}
func (j *Journal) Pending() []Result {
	j.mu.Lock()
	defer j.mu.Unlock()
	results := []Result{}
	for _, e := range j.entries {
		if !e.resultDeliverySettled() && e.Result != nil && (e.Terminal == nil || e.Terminal.ReconciliationOperationID == "") {
			copy := *e.Result
			if copy.WorkBuddyCloudWait != nil {
				proof := *copy.WorkBuddyCloudWait
				copy.WorkBuddyCloudWait = &proof
			}
			copy.Assignment = cloneModelAssignment(copy.Assignment)
			if copy.StartupFailure != nil {
				proof := *copy.StartupFailure
				proof.Profile = tc.CloneProfile(proof.Profile)
				copy.StartupFailure = &proof
			}
			if copy.ToolEvidence != nil {
				evidence := *copy.ToolEvidence
				evidence.Profile = tc.CloneProfile(evidence.Profile)
				evidence.Usage = cloneUsage(evidence.Usage)
				evidence.ModelExecution = cloneModelEvidence(evidence.ModelExecution)
				evidence.CodexExecution = cc.CloneEvidence(evidence.CodexExecution)
				evidence.OpenCodeExecution = cloneOpenCodeEvidence(evidence.OpenCodeExecution)
				evidence.OctopExecution = cloneOctopEvidence(evidence.OctopExecution)
				evidence.ACPExecution = cloneACPEvidence(evidence.ACPExecution)
				copy.ToolEvidence = &evidence
			}
			results = append(results, copy)
		}
	}
	return results
}
func (j *Journal) Ack(id string) error {
	return j.finalizeResult(id, "")
}

// RejectClosedResult keeps the original result and its identity after the
// control plane proves delivery can never apply to the closed attempt.
func (j *Journal) RejectClosedResult(id string) error {
	return j.finalizeResult(id, ResultRejectedClosedAttempt)
}

func (j *Journal) finalizeResult(id string, rejection ResultRejection) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for k, e := range j.entries {
		if e.Result != nil && e.Result.OperationID == id {
			acked := rejection == ""
			if e.resultDeliverySettled() {
				if e.Acked == acked && e.ResultRejection == rejection {
					return nil
				}
				return fmt.Errorf("result already has a different disposition")
			}
			previous := e
			e.Acked, e.ResultRejection = acked, rejection
			if !e.validResultDisposition() {
				return fmt.Errorf("invalid result disposition")
			}
			j.entries[k] = e
			if err := j.save(); err != nil {
				j.entries[k] = previous
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("result absent")
}
