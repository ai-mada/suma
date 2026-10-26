package candidatecompositioncontract

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const CandidateConflictRepairProtocol = "suma.candidate-conflict-repair.v1"

type CandidateConflictStage struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Object string `json:"object"`
	Stage  int    `json:"stage"`
}

type CandidateConflictRepairExecution struct {
	Protocol               string                        `json:"protocol"`
	PlanID                 string                        `json:"plan_id"`
	PlanVersion            int                           `json:"plan_version"`
	ProjectID              string                        `json:"project_id"`
	CompositionID          string                        `json:"composition_id"`
	CompositionVersion     int                           `json:"composition_version"`
	OriginalAssignment     cc.AssignmentRef              `json:"original_assignment"`
	OriginalEvidenceSHA256 string                        `json:"original_evidence_sha256"`
	Composition            CandidateCompositionExecution `json:"composition"`
	Conflict               CandidateCompositionEvidence  `json:"conflict"`
	Instructions           string                        `json:"instructions"`
	FixedTimestamp         string                        `json:"fixed_timestamp"`
	OperationID            string                        `json:"operation_id"`
}

type CandidateConflictRepairStep struct {
	SourceIndex    int      `json:"source_index"`
	LeftCommit     string   `json:"left_commit"`
	RightCommit    string   `json:"right_commit"`
	SeedTree       string   `json:"seed_tree"`
	ConflictSHA256 string   `json:"conflict_sha256,omitempty"`
	ResultCommit   string   `json:"result_commit"`
	ResultTree     string   `json:"result_tree"`
	Parents        []string `json:"parents"`
}

type CandidateConflictRepairEvidence struct {
	Protocol               string                        `json:"protocol"`
	PlanID                 string                        `json:"plan_id"`
	PlanVersion            int                           `json:"plan_version"`
	CompositionID          string                        `json:"composition_id"`
	OriginalAssignment     cc.AssignmentRef              `json:"original_assignment"`
	OriginalEvidenceSHA256 string                        `json:"original_evidence_sha256"`
	SourcesSHA256          string                        `json:"sources_sha256"`
	OperationID            string                        `json:"operation_id"`
	State                  string                        `json:"state"`
	Steps                  []CandidateConflictRepairStep `json:"steps"`
	ResultCommit           string                        `json:"result_commit,omitempty"`
	ResultTree             string                        `json:"result_tree,omitempty"`
	Parents                []string                      `json:"parents"`
	SourceHeads            []string                      `json:"source_heads"`
	MergeCommits           []string                      `json:"merge_commits"`
}

func CandidateCompositionConflictDigest(e CandidateCompositionEvidence) (string, error) {
	e.ConflictSHA256 = ""
	return canonicalSHA(e)
}

// A prior path-only report cannot authorize a repair. The original failed
// attempt remains an immutable fact; no guessed conflict tree is substituted.
func ValidateCandidateConflictRepairExecution(e CandidateConflictRepairExecution) error {
	invalid := func() error { return fmt.Errorf("invalid fixed candidate conflict repair execution") }
	if e.Protocol != CandidateConflictRepairProtocol || !validID(e.PlanID, 64) || e.PlanVersion < 1 || !validID(e.ProjectID, 64) || !validID(e.CompositionID, 64) || e.CompositionVersion < 1 || !e.OriginalAssignment.Valid() || !cc.ValidHash(e.OriginalEvidenceSHA256) || !validID(e.OperationID, 128) || ValidateCandidateCompositionExecution(e.Composition) != nil || e.Composition.ProjectID != e.ProjectID || e.Composition.PlanID != e.CompositionID || e.PlanID == e.CompositionID || strings.TrimSpace(e.Instructions) == "" || len(e.Instructions) > 32<<10 || !utf8.ValidString(e.Instructions) || strings.ContainsRune(e.Instructions, 0) {
		return invalid()
	}
	t, err := time.Parse(time.RFC3339Nano, e.FixedTimestamp)
	if err != nil || t.IsZero() || t.Format(time.RFC3339Nano) != e.FixedTimestamp || !strings.HasSuffix(e.FixedTimestamp, "Z") {
		return invalid()
	}
	f := e.Conflict
	if f.Protocol != e.Composition.Protocol || f.PlanID != e.CompositionID || f.PlanVersion != e.Composition.PlanVersion || f.SourcesSHA256 != e.Composition.SourcesSHA256 || f.OperationID != e.Composition.OperationID || f.BaseCommit != e.Composition.BaseCommit || f.State != "conflicted" || f.ResultCommit != "" || !cc.ValidCommit(f.ConflictTree) || f.ConflictSourceIndex < 1 || f.ConflictSourceIndex >= len(e.Composition.Sources) || len(f.ConflictStages) == 0 || len(f.ConflictStages) > 6000 || len(f.Conflicts) == 0 || len(f.Conflicts) > 2000 || len(f.MergeCommits) != f.ConflictSourceIndex-1 || len(f.SourceHeads) != len(e.Composition.Sources) {
		return invalid()
	}
	for i, source := range e.Composition.Sources {
		if f.SourceHeads[i] != source.HeadCommit {
			return invalid()
		}
	}
	for _, commit := range f.MergeCommits {
		if !cc.ValidCommit(commit) {
			return invalid()
		}
	}
	paths := map[string]bool{}
	total := 0
	for _, p := range f.Conflicts {
		if p == "" || len(p) > 512 || !utf8.ValidString(p) || strings.ContainsAny(p, "\\\x00\r\n") || paths[p] {
			return invalid()
		}
		total += len(p)
		paths[p] = true
	}
	seen := map[string]bool{}
	for _, s := range f.ConflictStages {
		key := fmt.Sprint(s.Stage) + ":" + s.Path
		if !paths[s.Path] || seen[key] || s.Stage < 1 || s.Stage > 3 || s.Mode != "100644" && s.Mode != "100755" || !cc.ValidCommit(s.Object) || len(s.Object) != len(e.Composition.BaseCommit) {
			return invalid()
		}
		seen[key] = true
	}
	if total > 64<<10 || !cc.ValidHash(f.ConflictSHA256) {
		return invalid()
	}
	sha, err := CandidateCompositionConflictDigest(f)
	if err != nil || sha != f.ConflictSHA256 {
		return invalid()
	}
	originalSHA, err := canonicalSHA(f)
	if err != nil || originalSHA != e.OriginalEvidenceSHA256 {
		return invalid()
	}
	return nil
}

// This checks the exact frozen ordered lineage report. Runner additionally
// proves each actual Git parent/tree and complete object closure before sealing.
func ValidateCandidateConflictRepairEvidence(f CandidateConflictRepairEvidence, e CandidateConflictRepairExecution) error {
	invalid := func() error { return fmt.Errorf("invalid completed candidate conflict repair evidence") }
	if ValidateCandidateConflictRepairExecution(e) != nil || f.Protocol != e.Protocol || f.PlanID != e.PlanID || f.PlanVersion != e.PlanVersion || f.CompositionID != e.CompositionID || f.OriginalAssignment != e.OriginalAssignment || f.OriginalEvidenceSHA256 != e.OriginalEvidenceSHA256 || f.SourcesSHA256 != e.Composition.SourcesSHA256 || f.OperationID != e.OperationID || f.State != "completed" || !cc.ValidCommit(f.ResultCommit) || !cc.ValidCommit(f.ResultTree) || len(f.SourceHeads) != len(e.Composition.Sources) || len(f.Steps) != len(e.Composition.Sources)-e.Conflict.ConflictSourceIndex || len(f.MergeCommits) != len(e.Composition.Sources)-1 {
		return invalid()
	}
	for i, source := range e.Composition.Sources {
		if f.SourceHeads[i] != source.HeadCommit {
			return invalid()
		}
	}
	current := e.Composition.Sources[0].HeadCommit
	for i, commit := range e.Conflict.MergeCommits {
		if f.MergeCommits[i] != commit {
			return invalid()
		}
		current = commit
	}
	for i, step := range f.Steps {
		index := e.Conflict.ConflictSourceIndex + i
		right := e.Composition.Sources[index].HeadCommit
		if i == 0 && (step.SeedTree != e.Conflict.ConflictTree || step.ConflictSHA256 != e.Conflict.ConflictSHA256) {
			return invalid()
		}
		if step.SourceIndex != index || step.LeftCommit != current || step.RightCommit != right || !cc.ValidCommit(step.SeedTree) || !cc.ValidCommit(step.ResultTree) || !cc.ValidCommit(step.ResultCommit) || f.MergeCommits[index-1] != step.ResultCommit || len(step.Parents) < 1 || len(step.Parents) > 2 || step.Parents[0] != current || right != current && (len(step.Parents) != 2 || step.Parents[1] != right) || right == current && len(step.Parents) != 1 {
			return invalid()
		}
		current = step.ResultCommit
	}
	last := f.Steps[len(f.Steps)-1]
	if f.ResultCommit == current {
		if f.ResultTree != last.ResultTree || len(f.Parents) != len(last.Parents) {
			return invalid()
		}
		for i, parent := range last.Parents {
			if f.Parents[i] != parent {
				return invalid()
			}
		}
	} else if len(f.Parents) != 1 || f.Parents[0] != current {
		return invalid()
	}
	return nil
}
