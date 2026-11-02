// Package candidatecompositioncontract fixes host-executed composition inputs
// and explicitly approved downstream code baselines. These envelopes carry no
// filesystem paths, shell commands or independent test/acceptance authority.
package candidatecompositioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const CandidateCompositionProtocol = "suma.candidate-composition.v1"
const CodeBaselineProtocol = "suma.code-baseline.v1"
const CompositionCommand = "suma-compose"

type CandidateCompositionExecution struct {
	Protocol       string       `json:"protocol"`
	PlanID         string       `json:"plan_id"`
	PlanVersion    int          `json:"plan_version"`
	ProjectID      string       `json:"project_id"`
	BaseCommit     string       `json:"base_commit"`
	Sources        []cc.Binding `json:"sources"`
	SourcesSHA256  string       `json:"sources_sha256"`
	FixedTimestamp string       `json:"fixed_timestamp"`
	OperationID    string       `json:"operation_id"`
}

type CodeBaselineBinding struct {
	Protocol                     string `json:"protocol"`
	ID                           string `json:"id"`
	ProjectID                    string `json:"project_id"`
	TaskID                       string `json:"task_id"`
	TaskVersion                  int    `json:"task_version"`
	CompositionID                string `json:"composition_id,omitempty"`
	CompositionVersion           int    `json:"composition_version,omitempty"`
	TerminalCandidatePlanID      string `json:"terminal_candidate_plan_id,omitempty"`
	TerminalCandidatePlanVersion int    `json:"terminal_candidate_plan_version,omitempty"`
	// Ordinary accepted-task origins retain comparable scalar identity; the
	// original Source envelope owns the exact run/node/attempt/commit tuple.
	SourceTaskID        string     `json:"source_task_id,omitempty"`
	SourceTaskVersion   int        `json:"source_task_version,omitempty"`
	AcceptedBasisSHA256 string     `json:"accepted_basis_sha256"`
	Source              cc.Binding `json:"source"`
	BindingSHA256       string     `json:"binding_sha256"`
}

// A bundle read requires the current consumer's full assignment. It is not a
// general transfer lookup and does not relax legacy same-run candidate_from.
type CodeInputRequest struct {
	ProjectID  string           `json:"project_id"`
	Assignment cc.AssignmentRef `json:"assignment"`
	InputKind  string           `json:"input_kind"`
	BindingID  string           `json:"binding_id"`
	TransferID string           `json:"transfer_id"`
}

// CandidateCompositionEvidence describes host Git facts, not formal tests or
// acceptance. Parents are the final commit's actual ordered parents.
type CandidateCompositionEvidence struct {
	Protocol            string                   `json:"protocol"`
	PlanID              string                   `json:"plan_id"`
	PlanVersion         int                      `json:"plan_version"`
	SourcesSHA256       string                   `json:"sources_sha256"`
	OperationID         string                   `json:"operation_id"`
	BaseCommit          string                   `json:"base_commit"`
	State               string                   `json:"state"`
	ResultCommit        string                   `json:"result_commit,omitempty"`
	ResultTree          string                   `json:"result_tree,omitempty"`
	Parents             []string                 `json:"parents"`
	SourceHeads         []string                 `json:"source_heads"`
	MergeCommits        []string                 `json:"merge_commits"`
	Conflicts           []string                 `json:"conflicts"`
	ConflictSourceIndex int                      `json:"conflict_source_index,omitempty"`
	ConflictTree        string                   `json:"conflict_tree,omitempty"`
	ConflictStages      []CandidateConflictStage `json:"conflict_stages,omitempty"`
	ConflictSHA256      string                   `json:"conflict_sha256,omitempty"`
}

func validID(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\\x00\r\n\t")
}
func canonicalSHA(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}

// SourcesDigest preserves the platform-fixed source order; callers must never
// sort it again or omit a no-delta source while replaying the same operation.
func CandidateCompositionSourcesDigest(sources []cc.Binding) (string, error) {
	return canonicalSHA(sources)
}
func CodeBaselineBindingDigest(binding CodeBaselineBinding) (string, error) {
	binding.BindingSHA256 = ""
	return canonicalSHA(binding)
}

func ValidateCandidateCompositionExecution(e CandidateCompositionExecution) error {
	invalid := func() error { return fmt.Errorf("invalid fixed candidate composition execution") }
	if e.Protocol != CandidateCompositionProtocol || !validID(e.PlanID, 64) || e.PlanVersion < 1 || !validID(e.ProjectID, 64) || !validID(e.OperationID, 128) || !cc.ValidCommit(e.BaseCommit) || len(e.Sources) < 2 || len(e.Sources) > 16 || !cc.ValidHash(e.SourcesSHA256) {
		return invalid()
	}
	timestamp, err := time.Parse(time.RFC3339Nano, e.FixedTimestamp)
	if err != nil || timestamp.IsZero() || timestamp.Format(time.RFC3339Nano) != e.FixedTimestamp || !strings.HasSuffix(e.FixedTimestamp, "Z") {
		return invalid()
	}
	seen := map[string]bool{}
	for _, source := range e.Sources {
		if !source.ValidSealed() || source.ProjectID != e.ProjectID || source.BundleBase() != e.BaseCommit || len(source.HeadCommit) != len(e.BaseCommit) || seen[source.TransferID] {
			return invalid()
		}
		seen[source.TransferID] = true
	}
	sha, err := CandidateCompositionSourcesDigest(e.Sources)
	if err != nil || sha != e.SourcesSHA256 {
		return invalid()
	}
	return nil
}
func ValidateCodeBaselineBinding(b CodeBaselineBinding) error {
	composition := validID(b.CompositionID, 64) && b.CompositionVersion > 0 && b.TerminalCandidatePlanID == "" && b.TerminalCandidatePlanVersion == 0 && b.SourceTaskID == "" && b.SourceTaskVersion == 0
	terminal := validID(b.TerminalCandidatePlanID, 64) && b.TerminalCandidatePlanVersion > 0 && b.CompositionID == "" && b.CompositionVersion == 0 && b.SourceTaskID == "" && b.SourceTaskVersion == 0
	task := validID(b.SourceTaskID, 64) && b.SourceTaskID != b.TaskID && b.SourceTaskVersion > 0 && b.CompositionID == "" && b.CompositionVersion == 0 && b.TerminalCandidatePlanID == "" && b.TerminalCandidatePlanVersion == 0
	if b.Protocol != CodeBaselineProtocol || !validID(b.ID, 64) || !validID(b.ProjectID, 64) || !validID(b.TaskID, 64) || b.TaskVersion < 1 || (!composition && !terminal && !task) || !cc.ValidHash(b.AcceptedBasisSHA256) || !cc.ValidHash(b.BindingSHA256) || !b.Source.ValidSealed() || b.Source.ProjectID != b.ProjectID {
		return fmt.Errorf("invalid fixed code baseline binding")
	}
	sha, err := CodeBaselineBindingDigest(b)
	if err != nil || sha != b.BindingSHA256 {
		return fmt.Errorf("invalid fixed code baseline digest")
	}
	return nil
}
func ValidateCodeInputRequest(r CodeInputRequest) error {
	if !validID(r.ProjectID, 64) || !r.Assignment.Valid() || r.InputKind != "composition" && r.InputKind != "code_baseline" && r.InputKind != "conflict_repair" && r.InputKind != "development_continuation" || !validID(r.BindingID, 64) || !validID(r.TransferID, 64) {
		return fmt.Errorf("invalid current-assignment code input request")
	}
	return nil
}
