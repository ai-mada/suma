package platform

import (
	"bytes"
	"encoding/json"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Native development checks are subordinate evidence. Formal acceptance still
// requires a separate successful test node covering this exact candidate SHA.
func validateNativeWorkspaceEvidence(proof map[string]any, tool *tc.Evidence) error {
	raw, exists := proof["native_workspace"]
	if !exists {
		if tool != nil && tool.Profile.WorkspaceToolsVersion != "" {
			return invalid("原生编码缺少固定工作区与封存证据")
		}
		return nil
	}
	if raw == nil || tool == nil || tool.Profile.WorkspaceToolsVersion != nwc.Version || tool.Profile.Adapter != "suma" || tool.Profile.ReadOnly {
		return invalid("原生编码证据与执行工具声明不一致")
	}
	data, err := json.Marshal(raw)
	if err != nil || len(data) > 1<<20 {
		return invalid("原生编码证据格式或大小无效")
	}
	var evidence nwc.NativeWorkspaceEvidence
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&evidence) != nil || evidence.SchemaVersion != 1 || evidence.State != "closed" || evidence.PolicySHA256 != tool.Profile.ProfileSHA256 || evidence.Generation < 0 || evidence.Generation > 2147483646 || len(evidence.Files) > 2000 || len(evidence.Checks) > 16 {
		return invalid("原生编码证据尚未闭合或执行策略已变化")
	}
	seal := evidence.Seal
	if seal == nil || seal.SchemaVersion != 1 || seal.PolicySHA256 != evidence.PolicySHA256 || seal.OperationID == "" || seal.Commit != proof["head_commit"] || !gitRevision.MatchString(seal.Tree) || seal.ChangedPaths < 0 || seal.ChangedPaths > 2000 {
		return invalid("原生候选提交与固定封存依据不一致")
	}
	previous := proof["base_commit"]
	if _, repair := proof["candidate_conflict_repair"]; repair || tool.Profile.ConflictRepairProtocol != "" {
		prefix, err := nativeConflictRepairSealPrevious(proof, tool.Profile, *seal)
		if err != nil {
			return err
		}
		previous = prefix
	}
	if seal.PreviousCommit != previous {
		return invalid("原生候选提交与固定封存依据不一致")
	}
	if seal.State != "sealed" && seal.State != "no_change" || seal.State == "sealed" && seal.Commit == seal.PreviousCommit || seal.State == "no_change" && (seal.Commit != seal.PreviousCommit || seal.ChangedPaths != 0) {
		return invalid("原生候选提交与固定封存依据不一致")
	}
	seen := map[string]bool{}
	for _, file := range evidence.Files {
		if file.Path == "" || seen[file.Path] || file.OperationID == "" || file.Generation < 1 || file.Generation > evidence.Generation || !file.Removed && !contentDigest.MatchString(file.SHA256) || file.Removed && !contentDigest.MatchString(file.PreviousSHA256) {
			return invalid("原生文件操作缺少完整版本与受理依据")
		}
		seen[file.Path] = true
	}
	checks := map[string]bool{}
	for _, check := range evidence.Checks {
		if check.CheckID == "" || checks[check.CheckID] || check.OperationID == "" || !contentDigest.MatchString(check.ProfileSHA256) || !contentDigest.MatchString(check.WorkspaceSHA256) || check.Generation != evidence.Generation || check.ExitCode != 0 || check.Stopped || check.OutcomeUnknown {
			return invalid("原生开发检查未覆盖最终代码或效果尚待核对")
		}
		checks[check.CheckID] = true
	}
	return nil
}

// This validates only the structural relation between two received host facts.
// It grants no repair authority: the result callback must separately validate
// the exact durable execution, source bindings and candidate transfer in SQL.
func nativeConflictRepairSealPrevious(proof map[string]any, profile tc.Profile, seal nwc.NativeWorkspaceSeal) (string, error) {
	invalidSeal := func() (string, error) { return "", invalid("原生候选提交与固定封存依据不一致") }
	if !candidateConflictRepairProfile(profile) || proof["state"] != "completed" || proof["clean"] != true || proof["outcome_unknown"] != false {
		return invalidSeal()
	}
	raw, err := json.Marshal(proof["candidate_conflict_repair"])
	if err != nil || len(raw) > 1<<20 {
		return invalidSeal()
	}
	var fact ccmp.CandidateConflictRepairEvidence
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&fact) != nil || fact.Protocol != ccmp.CandidateConflictRepairProtocol || !handoffID(fact.PlanID, false) || fact.PlanVersion < 1 || !handoffID(fact.CompositionID, false) || fact.CompositionID == fact.PlanID || !fact.OriginalAssignment.Valid() || !cc.ValidHash(fact.OriginalEvidenceSHA256) || !cc.ValidHash(fact.SourcesSHA256) || !diagnosisID(fact.OperationID, 128) || fact.State != "completed" || !cc.ValidCommit(fact.ResultCommit) || !cc.ValidCommit(fact.ResultTree) || fact.ResultCommit != proof["head_commit"] || fact.ResultTree != seal.Tree || fact.OperationID != seal.OperationID || len(fact.SourceHeads) < 2 || len(fact.SourceHeads) > 16 || len(fact.MergeCommits) != len(fact.SourceHeads)-1 || len(fact.Steps) < 1 || len(fact.Steps) > len(fact.SourceHeads)-1 {
		return invalidSeal()
	}
	base, ok := proof["base_commit"].(string)
	if !ok || !cc.ValidCommit(base) {
		return invalidSeal()
	}
	for _, commit := range append(append([]string{}, fact.SourceHeads...), fact.MergeCommits...) {
		if !cc.ValidCommit(commit) || len(commit) != len(base) {
			return invalidSeal()
		}
	}
	first := fact.Steps[0].SourceIndex
	if first < 1 || first >= len(fact.SourceHeads) || len(fact.Steps) != len(fact.SourceHeads)-first || !cc.ValidHash(fact.Steps[0].ConflictSHA256) {
		return invalidSeal()
	}
	current := fact.SourceHeads[0]
	if first > 1 {
		current = fact.MergeCommits[first-2]
	}
	for i, step := range fact.Steps {
		index := first + i
		right := fact.SourceHeads[index]
		if step.SourceIndex != index || step.LeftCommit != current || step.RightCommit != right || !cc.ValidCommit(step.SeedTree) || !cc.ValidCommit(step.ResultTree) || !cc.ValidCommit(step.ResultCommit) || len(step.SeedTree) != len(base) || len(step.ResultTree) != len(base) || len(step.ResultCommit) != len(base) || step.ResultCommit != fact.MergeCommits[index-1] || step.ConflictSHA256 != "" && !cc.ValidHash(step.ConflictSHA256) || len(step.Parents) < 1 || len(step.Parents) > 2 || step.Parents[0] != current || right == current && len(step.Parents) != 1 || right != current && (len(step.Parents) != 2 || step.Parents[1] != right) {
			return invalidSeal()
		}
		current = step.ResultCommit
	}
	last := fact.Steps[len(fact.Steps)-1]
	if fact.ResultCommit == current {
		if fact.ResultTree != last.ResultTree || stringify(fact.Parents) != stringify(last.Parents) || seal.State != "no_change" || seal.ChangedPaths != 0 {
			return invalidSeal()
		}
	} else if len(fact.Parents) != 1 || fact.Parents[0] != current || fact.ResultTree == last.ResultTree || seal.State != "sealed" || seal.ChangedPaths < 1 {
		return invalidSeal()
	}
	return current, nil
}
