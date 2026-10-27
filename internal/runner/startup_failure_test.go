package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ec "github.com/ai-mada/suma/internal/executioncontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func startupRepairAssignment(t *testing.T) Assignment {
	t.Helper()
	base := strings.Repeat("a", 40)
	sources := []cc.Binding{}
	for i, head := range []string{strings.Repeat("b", 40), strings.Repeat("c", 40)} {
		sources = append(sources, cc.Binding{SchemaVersion: 1, TransferID: string(rune('a' + i)), ProjectID: "project", State: "sealed", Source: cc.AssignmentRef{RunID: string(rune('a' + i)), NodeID: "code", WorkerID: "source", Attempt: 1, Fence: 1}, BaseCommit: base, HeadCommit: head, EvidenceSHA256: strings.Repeat("d", 64), BundleSHA256: strings.Repeat("e", 64), SizeBytes: 100})
	}
	original := compositionAssignment(t, base, sources)
	facts := ccc.CandidateCompositionEvidence{Protocol: ccc.CandidateCompositionProtocol, PlanID: "plan", PlanVersion: 1, SourcesSHA256: original.CandidateComposition.SourcesSHA256, OperationID: "compose-op", BaseCommit: base, State: "conflicted", Parents: []string{}, SourceHeads: []string{sources[0].HeadCommit, sources[1].HeadCommit}, MergeCommits: []string{}, Conflicts: []string{"file.txt"}, ConflictSourceIndex: 1, ConflictTree: strings.Repeat("d", 40), ConflictStages: []ccc.CandidateConflictStage{{Path: "file.txt", Mode: "100644", Object: strings.Repeat("e", 40), Stage: 1}, {Path: "file.txt", Mode: "100644", Object: strings.Repeat("f", 40), Stage: 2}, {Path: "file.txt", Mode: "100644", Object: strings.Repeat("a", 40), Stage: 3}}}
	facts.ConflictSHA256, _ = ccc.CandidateCompositionConflictDigest(facts)
	raw, _ := json.Marshal(facts)
	return Assignment{RunID: "repair-run", NodeID: "repair", WorkerID: "repairer", Attempt: 1, Fence: 2, Type: "agent", Command: []string{"native"}, Config: map[string]any{"purpose": "candidate_conflict_repair", "goal": "Resolve the frozen conflict."}, CandidateConflictRepair: &ccc.CandidateConflictRepairExecution{Protocol: ccc.CandidateConflictRepairProtocol, PlanID: "repair-plan", PlanVersion: 1, ProjectID: "project", CompositionID: "plan", CompositionVersion: 2, OriginalAssignment: candidateRef(original), OriginalEvidenceSHA256: digest(raw), Composition: *original.CandidateComposition, Conflict: facts, Instructions: "Resolve the exact frozen conflict.", FixedTimestamp: "2026-10-27T01:00:00Z", OperationID: "repair-operation"}}
}

func TestStartupFailureJournalRetainsOriginalReceipt(t *testing.T) {
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	a := Assignment{RunID: "startup-journal", NodeID: "repair", Attempt: 1, Fence: 3, WorkerID: "worker"}
	profile := tc.Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}
	proof := &ec.StartupFailureProof{Protocol: ec.StartupFailureProtocol, Stage: "before_workspace", Assignment: candidateRef(a), Profile: profile, Purpose: "candidate_conflict_repair"}
	if err = journal.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = journal.Finish(Result{Assignment: a, ExitCode: -1, Error: "Known startup failure", StartupFailure: proof}); err != nil {
		t.Fatal(err)
	}
	proof.Assignment.Fence++
	first := journal.Pending()
	if len(first) != 1 || first[0].StartupFailure == nil || first[0].StartupFailure.Assignment.Fence != 3 || first[0].OperationID == "" {
		t.Fatal("caller altered the original receipt", first)
	}
	first[0].StartupFailure.ProcessStarted = true
	second := journal.Pending()
	if second[0].StartupFailure.ProcessStarted || second[0].OperationID != first[0].OperationID {
		t.Fatal("pending projection altered durable proof", second)
	}
	var restored Result
	raw, _ := json.Marshal(second[0])
	if err = json.Unmarshal(raw, &restored); err != nil || restored.StartupFailure == nil || restored.StartupFailure.Assignment.Fence != 3 {
		t.Fatal("wire lost original startup proof", string(raw), err)
	}
}

// A missing configured root is checked before preparation; failure inside
// prepareWorkspace must never receive the same certification.
func TestStartupFailureCertifiesOnlyBeforeWorkspaceEntry(t *testing.T) {
	cfg, _ := nativeHostFixture(t)
	spec := cfg.Commands["native"]
	spec.NativeWorkspace.ConflictRepair = true
	cfg.Commands["native"] = spec
	_, executableSHA, err := commandExecutable(spec)
	if err != nil {
		t.Fatal(err)
	}
	profileSHA, err := effectiveCommandProfileDigest(cfg, spec, executableSHA)
	if err != nil {
		t.Fatal(err)
	}
	profile := tc.Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: profileSHA, ExecutableSHA256: executableSHA, ToolVersion: "host-startup-fixture", WorkspaceToolsVersion: NativeWorkspaceVersion, ConflictRepairProtocol: ccc.CandidateConflictRepairProtocol}
	cfg.ToolInventory = []tc.Observation{{Profile: profile, Available: true}}
	a := startupRepairAssignment(t)
	a.Config["tool_profile_id"], a.Config["tool_profile_sha256"] = "native-profile", profileSHA
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before-entry", false: "inside-preparation"}[before], func(t *testing.T) {
			next := cfg
			if before {
				next.Workspace = filepath.Join(t.TempDir(), "missing-owned-root")
			}
			result := Execute(context.Background(), a, next)
			if result.ExitCode == 0 || result.Error == "" || result.ToolEvidence != nil || result.WorkspaceEvidence != nil || result.OutcomeUnknown {
				t.Fatalf("unexpected startup facts: %+v", result)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			proof, present := fields["startup_failure"]
			if present != before {
				t.Fatalf("before workspace=%v, certified=%v: %s", before, present, raw)
			}
			if present {
				var value map[string]any
				if json.Unmarshal(proof, &value) != nil || value["stage"] != "before_workspace" || value["workspace_started"] != false || value["process_started"] != false {
					t.Fatal("invalid typed startup certificate", string(proof))
				}
			}
			if _, err := os.Stat(filepath.Join(cfg.Repository, ".cache", "suma-worktrees", attemptKey(a))); !os.IsNotExist(err) {
				t.Fatal("startup fixture unexpectedly entered an attempt workspace", err)
			}
		})
	}
}
