package platform

import (
	"bytes"
	"encoding/json"
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
	if seal == nil || seal.SchemaVersion != 1 || seal.PolicySHA256 != evidence.PolicySHA256 || seal.OperationID == "" || seal.Commit != proof["head_commit"] || seal.PreviousCommit != proof["base_commit"] || !gitRevision.MatchString(seal.Tree) || seal.ChangedPaths < 0 || seal.ChangedPaths > 2000 {
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
