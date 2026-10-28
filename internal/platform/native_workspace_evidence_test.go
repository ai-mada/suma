package platform

import (
	"context"
	"encoding/json"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
	"testing"
)

func nativeEvidenceFixture() (map[string]any, *tc.Evidence) {
	policy := strings.Repeat("a", 64)
	base := strings.Repeat("b", 40)
	head := strings.Repeat("c", 40)
	files := []nwc.NativeWorkspaceFileEffect{{Path: "src/code.ts", OperationID: "write-op", SHA256: strings.Repeat("d", 64), Generation: 1}}
	evidence := nwc.NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: policy, Generation: 1, State: "closed", Files: files, Checks: []nwc.NativeWorkspaceCheckResult{{OperationID: "check-op", CheckID: "unit", ProfileSHA256: strings.Repeat("e", 64), WorkspaceSHA256: strings.Repeat("f", 64), Generation: 1, ExitCode: 0}}, Seal: &nwc.NativeWorkspaceSeal{SchemaVersion: 1, OperationID: "seal-op", PolicySHA256: policy, State: "sealed", PreviousCommit: base, Commit: head, Tree: strings.Repeat("d", 40), ChangedPaths: 1}}
	return map[string]any{"base_commit": base, "head_commit": head, "native_workspace": evidence}, &tc.Evidence{Profile: tc.Profile{Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: policy, WorkspaceToolsVersion: nwc.Version, ModelExecutionProtocol: "suma-model-execution-v1"}}
}

func nativeRepairEvidenceFixture(edited bool) (map[string]any, *tc.Evidence) {
	proof, tool := nativeEvidenceFixture()
	tool.Profile.Protocol = "suma-jsonl-v1"
	tool.Profile.ConflictRepairProtocol = ccmp.CandidateConflictRepairProtocol
	prefix := strings.Repeat("5", 40)
	tree := strings.Repeat("7", 40)
	parents := []string{strings.Repeat("4", 40), strings.Repeat("3", 40)}
	fact := ccmp.CandidateConflictRepairEvidence{Protocol: ccmp.CandidateConflictRepairProtocol, PlanID: "repair-plan", PlanVersion: 2, CompositionID: "original-composition", OriginalAssignment: cc.AssignmentRef{RunID: "original-run", NodeID: "compose", Attempt: 1, Fence: 1, WorkerID: "original-worker"}, OriginalEvidenceSHA256: strings.Repeat("e", 64), SourcesSHA256: strings.Repeat("f", 64), OperationID: "repair-operation", State: "completed", SourceHeads: []string{strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)}, MergeCommits: []string{strings.Repeat("4", 40), prefix}, Steps: []ccmp.CandidateConflictRepairStep{{SourceIndex: 2, LeftCommit: parents[0], RightCommit: parents[1], SeedTree: strings.Repeat("6", 40), ConflictSHA256: strings.Repeat("a", 64), ResultCommit: prefix, ResultTree: tree, Parents: parents}}, ResultCommit: prefix, ResultTree: tree, Parents: parents}
	e := proof["native_workspace"].(nwc.NativeWorkspaceEvidence)
	e.Files = []nwc.NativeWorkspaceFileEffect{}
	e.Seal = &nwc.NativeWorkspaceSeal{SchemaVersion: 1, OperationID: fact.OperationID, PolicySHA256: e.PolicySHA256, State: "no_change", PreviousCommit: prefix, Commit: prefix, Tree: tree, ChangedPaths: 0}
	if edited {
		fact.ResultCommit = strings.Repeat("8", 40)
		fact.ResultTree = strings.Repeat("9", 40)
		fact.Parents = []string{prefix}
		e.Files = []nwc.NativeWorkspaceFileEffect{{Path: "src/post-merge.ts", OperationID: "post-merge-write", SHA256: strings.Repeat("d", 64), Generation: 1}}
		e.Seal.State = "sealed"
		e.Seal.Commit = fact.ResultCommit
		e.Seal.Tree = fact.ResultTree
		e.Seal.ChangedPaths = 1
	}
	proof["head_commit"] = fact.ResultCommit
	proof["state"] = "completed"
	proof["clean"] = true
	proof["outcome_unknown"] = false
	proof["native_workspace"] = e
	proof["candidate_conflict_repair"] = fact
	return proof, tool
}

func TestNativeWorkspaceConflictRepairSealUsesCompletedMergePrefix(t *testing.T) {
	for _, edited := range []bool{false, true} {
		name := "no_post_merge_change"
		if edited {
			name = "post_merge_edit"
		}
		t.Run(name, func(t *testing.T) {
			proof, tool := nativeRepairEvidenceFixture(edited)
			if err := validateNativeWorkspaceEvidence(proof, tool); err != nil {
				t.Fatal("actual-shaped repaired seal was rejected", err)
			}
		})
	}
	for _, name := range []string{"ordinary_capability", "missing_repair", "unknown_protocol", "unknown_fields", "not_closed", "wrong_head", "wrong_tree", "missing_source", "missing_step", "source_order", "wrong_prefix", "broken_parents", "wrong_seal_operation", "incomplete_step", "unbound_identity", "nochange_tree", "sealed_no_change"} {
		t.Run(name, func(t *testing.T) {
			proof, tool := nativeRepairEvidenceFixture(false)
			fact := proof["candidate_conflict_repair"].(ccmp.CandidateConflictRepairEvidence)
			e := proof["native_workspace"].(nwc.NativeWorkspaceEvidence)
			switch name {
			case "ordinary_capability":
				tool.Profile.ConflictRepairProtocol = ""
			case "missing_repair":
				delete(proof, "candidate_conflict_repair")
			case "unknown_protocol":
				tool.Profile.ConflictRepairProtocol = "unsupported-repair"
			case "unknown_fields":
				raw, _ := json.Marshal(fact)
				var fields map[string]any
				_ = json.Unmarshal(raw, &fields)
				fields["accepted"] = true
				proof["candidate_conflict_repair"] = fields
			case "not_closed":
				fact.State = "resolving"
			case "wrong_head":
				fact.ResultCommit = strings.Repeat("0", 40)
			case "wrong_tree":
				e.Seal.Tree = strings.Repeat("0", 40)
			case "missing_source":
				fact.SourceHeads = fact.SourceHeads[:2]
			case "missing_step":
				fact.Steps = nil
			case "source_order":
				fact.SourceHeads[1], fact.SourceHeads[2] = fact.SourceHeads[2], fact.SourceHeads[1]
			case "wrong_prefix":
				e.Seal.PreviousCommit = proof["base_commit"].(string)
			case "broken_parents":
				fact.Steps[0].Parents[0] = strings.Repeat("0", 40)
			case "wrong_seal_operation":
				e.Seal.OperationID = "other-operation"
			case "incomplete_step":
				fact.Steps[0].SourceIndex = 1
			case "unbound_identity":
				fact.PlanID = ""
			case "nochange_tree":
				fact.ResultTree = strings.Repeat("0", 40)
				e.Seal.Tree = fact.ResultTree
			case "sealed_no_change":
				e.Seal.State = "sealed"
				e.Seal.ChangedPaths = 1
			}
			if name != "missing_repair" && name != "unknown_fields" {
				proof["candidate_conflict_repair"] = fact
			}
			proof["native_workspace"] = e
			if err := validateNativeWorkspaceEvidence(proof, tool); err == nil {
				t.Fatal("malformed or unscoped repair facts relaxed native sealing")
			}
		})
	}
	t.Run("structural_facts_do_not_grant_unbound_execution", func(t *testing.T) {
		proof, _ := nativeRepairEvidenceFixture(false)
		f, ctx := newFixture(t), context.Background()
		tx, err := f.s.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		result := dag.Result{RunID: newID(), NodeID: "unbound-agent", Attempt: 1, Fence: 1, WorkerID: "unbound-worker", WorkspaceEvidence: proof}
		expertStatus(t, f.s.validateCandidateCompositionRepairResultTx(ctx, tx, f.user.TenantID, f.project, result), 403)
	})
}

func TestNativeWorkspaceAcceptanceRequiresCurrentClosedSeal(t *testing.T) {
	proof, tool := nativeEvidenceFixture()
	if err := validateNativeWorkspaceEvidence(proof, tool); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "unknown", "policy", "head", "check_generation", "check_failed", "check_unknown", "check_stopped", "seal_missing", "untyped"} {
		t.Run(name, func(t *testing.T) {
			proof, tool := nativeEvidenceFixture()
			e := proof["native_workspace"].(nwc.NativeWorkspaceEvidence)
			switch name {
			case "missing":
				delete(proof, "native_workspace")
			case "unknown":
				e.State = "unknown"
			case "policy":
				e.PolicySHA256 = strings.Repeat("b", 64)
			case "head":
				proof["head_commit"] = strings.Repeat("f", 40)
			case "check_generation":
				e.Checks[0].Generation = 0
			case "check_failed":
				e.Checks[0].ExitCode = 1
			case "check_unknown":
				e.Checks[0].OutcomeUnknown = true
			case "check_stopped":
				e.Checks[0].Stopped = true
			case "seal_missing":
				e.Seal = nil
			case "untyped":
				raw, _ := json.Marshal(e)
				var m map[string]any
				_ = json.Unmarshal(raw, &m)
				m["approved"] = true
				proof["native_workspace"] = m
			}
			if name != "missing" && name != "untyped" {
				proof["native_workspace"] = e
			}
			if err := validateNativeWorkspaceEvidence(proof, tool); err == nil {
				t.Fatal("invalid native evidence accepted")
			}
		})
	}
	if err := validateNativeWorkspaceEvidence(map[string]any{}, nil); err != nil {
		t.Fatal("legacy evidence was broadened", err)
	}
}
