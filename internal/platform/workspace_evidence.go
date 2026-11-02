package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	"regexp"
)

var gitRevision = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
var contentDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validProofZero(v any) bool {
	switch n := v.(type) {
	case int:
		return n == 0
	case float64:
		return n == 0
	case int64:
		return n == 0
	}
	return false
}
func workspaceRevision(proof map[string]any) (string, error) {
	head, _ := proof["head_commit"].(string)
	base, _ := proof["base_commit"].(string)
	digest, _ := proof["evidence_sha256"].(string)
	if !gitRevision.MatchString(head) || !gitRevision.MatchString(base) || !contentDigest.MatchString(digest) || proof["clean"] != true || proof["state"] != "completed" || proof["outcome_unknown"] != false || !validProofZero(proof["exit_code"]) {
		return "", invalid("工作树证据必须包含明确成功、干净且已提交的固定代码版本")
	}
	return head, nil
}

// Code-producing runs require tests of every selected candidate. An explicit
// test node and administrator-designated test command are separate facts.
// Test-only runs without a code workspace retain non-code test semantics; they
// cannot be used to certify a code-producing run or merge a code candidate.
func validateCodeTestEvidence(run dag.Run) error {
	tested := map[string][]dag.NodeRun{}
	candidates := []dag.NodeRun{}
	hasWorkspace := false
	hasCode := false
	for _, node := range run.Nodes {
		if node.State != "skipped" && (node.Type == "command" || node.Type == "agent") {
			hasCode = true
		}
	}
	identities := map[string]bool{}
	for _, n := range run.Nodes {
		if n.State == "skipped" {
			continue
		}
		if hasCode && (n.Type == "command" || n.Type == "agent" || n.Type == "test") {
			// Both nodes may be on attempt 1. Independence is a different
			// authenticated Worker and node, not unequal numeric attempts.
			if n.ID == "" || identities[n.ID] || n.WorkerID == "" || n.Attempt < 1 || n.Fence < 1 || n.State != "succeeded" || n.Error != "" {
				return invalid("代码和测试必须有明确成功的当前执行节点与尝试证据")
			}
			identities[n.ID] = true
		}
		if len(n.WorkspaceEvidence) == 0 {
			continue
		}
		if err := validateNativeWorkspaceEvidence(n.WorkspaceEvidence, n.ToolEvidence); err != nil {
			return err
		}
		hasWorkspace = true
		head, e := workspaceRevision(n.WorkspaceEvidence)
		if e != nil {
			return e
		}
		if n.Type == "test" {
			if n.WorkspaceEvidence["test_command"] != true || n.WorkspaceEvidence["base_commit"] != head {
				return invalid("测试必须由执行节点管理员指定，并保持固定候选提交不变")
			}
			source, _ := n.WorkspaceEvidence["candidate_from"].(string)
			tested[source] = append(tested[source], n)
		} else if n.Type == "command" || n.Type == "agent" {
			candidates = append(candidates, n)
		}
	}
	if hasCode || hasWorkspace {
		for _, n := range run.Nodes {
			if n.State == "skipped" {
				continue
			}
			if n.Type == "test" || n.Type == "command" || n.Type == "agent" {
				if len(n.WorkspaceEvidence) == 0 {
					return invalid("缺少代码或测试的固定工作树证据，不能正式验收")
				}
			}
		}
		for _, candidate := range candidates {
			covered := false
			for _, test := range tested[candidate.ID] {
				if test.WorkerID == candidate.WorkerID || test.WorkspaceEvidence["head_commit"] != candidate.WorkspaceEvidence["head_commit"] {
					continue
				}
				sha, _ := candidate.WorkspaceEvidence["evidence_sha256"].(string)
				if codeCandidateSourceProofMatches(test.WorkspaceEvidence, candidate.ID, candidate.Attempt, candidate.Fence, sha) {
					covered = true
					break
				}
			}
			if !covered {
				return invalid("每个候选代码必须由另一执行节点按当前来源尝试独立测试")
			}
		}
	}
	return nil
}
