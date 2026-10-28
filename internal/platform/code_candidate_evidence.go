package platform

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// Formal task acceptance is the common authority. For code-bearing tasks we
// additionally retain the original candidate-to-independent-test relationship;
// a successful test-only result cannot substitute a missing code producer.
func (s *Service) currentCodeCandidateEvidenceTx(ctx context.Context, auth *authorizedCommand, project, run string) (bool, string, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return false, "", err
	}
	return s.currentCodeCandidateEvidenceReadTx(ctx, auth.tx, auth.currentPrincipal, project, run)
}

// A successful wrapper is not evidence that its children were non-code. Only
// current scheduler-owned child links may extend this proof; arbitrary output
// references and older subworkflow attempts cannot introduce a child result.
func (s *Service) currentCodeCandidateEvidenceTreeReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, run string, depth int, visiting map[string]bool, total *int) (bool, string, error) {
	unsupported := "code_source_unsupported"
	if run == "" || depth > 32 || visiting[run] {
		return false, unsupported, nil
	}
	visiting[run] = true
	defer delete(visiting, run)
	var state string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, run).Scan(&state); err != nil {
		return false, "", err
	}
	if state != "succeeded" {
		return false, "code_evidence_stale", nil
	}
	nodes, err := s.testPlanNodesTx(ctx, tx, p.TenantID, project, run)
	if err != nil {
		return false, "", err
	}
	*total += len(nodes)
	if *total > 10000 {
		return false, unsupported, nil
	}
	for _, node := range nodes {
		if node.State == "skipped" {
			continue
		}
		if node.Spec.Type == "map" || node.Spec.Type == "external_workflow" {
			return false, unsupported, nil
		}
		if node.Spec.Type == "test" {
			if _, ok := node.Spec.Config["test_from"]; ok {
				return false, unsupported, nil
			}
			if _, ok := node.Spec.Config["candidate_composition"]; ok {
				return false, unsupported, nil
			}
		}
		if head, _ := node.Workspace["head_commit"].(string); head != "" && node.Spec.Type != "agent" && node.Spec.Type != "command" && node.Spec.Type != "test" {
			return false, unsupported, nil
		}
	}
	for _, producer := range nodes {
		if producer.Spec.Type != "agent" && producer.Spec.Type != "command" {
			continue
		}
		head, _ := producer.Workspace["head_commit"].(string)
		if head == "" {
			continue
		} // Non-code actions retain ordinary formal task evidence.
		covered := false
		for _, test := range nodes {
			source, _ := test.Spec.Config["candidate_from"].(string)
			if test.Spec.Type != "test" || source != producer.ID {
				continue
			}
			result, found, e := s.codeCandidateNodeResultReadTx(ctx, tx, p, project, run, test.ID)
			if e != nil {
				return false, "", e
			}
			if !found || result.Result.ToolEvidence == nil {
				continue
			}
			profileID, profileSHA, bound, e := dag.ToolBinding(test.Spec)
			if e != nil || !bound {
				continue
			}
			proof := result.Result.ToolEvidence
			if toolcontract.ValidateEvidence(*proof) != nil || proof.Terminal != "completed" || proof.Profile.ProfileSHA256 != profileSHA {
				continue
			}
			profile, e := s.toolProfileTx(ctx, tx, p, project, profileID)
			if e != nil {
				var api *APIError
				if errors.As(e, &api) {
					continue
				}
				return false, "", e
			}
			if profile.WorkerID != test.Worker || !toolcontract.EqualProfile(profile.Profile, proof.Profile) {
				continue
			}
			ok, e := s.codeCandidateIndependentTestReadTx(ctx, tx, p, project, run, TestCaseEvidence{NodeID: test.ID, Attempt: test.Attempt, Fence: test.Fence, WorkerID: test.Worker, SourceCommit: head})
			if e != nil {
				return false, "", e
			}
			if ok {
				covered = true
				break
			}
		}
		if !covered {
			return false, "code_evidence_stale", nil
		}
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT x.node_id,x.child_run_id,COALESCE(c.tenant_id,''),COALESCE(c.project_id,''),COALESCE(c.state,'') FROM dag_subruns x JOIN dag_nodes n ON n.run_id=x.parent_run_id AND n.id=x.node_id AND n.attempt=x.attempt JOIN dag_runs p ON p.id=x.parent_run_id LEFT JOIN dag_runs c ON c.id=x.child_run_id WHERE p.tenant_id=? AND p.project_id=? AND x.parent_run_id=? ORDER BY x.node_id`), p.TenantID, project, run)
	if err != nil {
		return false, "", err
	}
	type childProof struct{ node, id, tenant, project, state string }
	children := []childProof{}
	for rows.Next() {
		var child childProof
		if err = rows.Scan(&child.node, &child.id, &child.tenant, &child.project, &child.state); err != nil {
			break
		}
		children = append(children, child)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, "", err
	}
	linked := map[string]bool{}
	for _, child := range children {
		node, ok := nodes[child.node]
		if !ok || node.Spec.Type != "subworkflow" || node.State != "succeeded" || node.Attempt < 1 || linked[child.node] || child.tenant != p.TenantID || child.project != project || child.state != "succeeded" {
			return false, unsupported, nil
		}
		linked[child.node] = true
		ok, code, e := s.currentCodeCandidateEvidenceTreeReadTx(ctx, tx, p, project, child.id, depth+1, visiting, total)
		if e != nil || !ok {
			return ok, code, e
		}
	}
	for _, node := range nodes {
		if node.Spec.Type == "subworkflow" && node.State != "skipped" && !linked[node.ID] {
			return false, unsupported, nil
		}
	}
	return true, "", nil
}

// Caller must already hold current project authority and governance guards.
func (s *Service) currentCodeCandidateEvidenceReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, run string) (bool, string, error) {
	return s.currentCodeCandidateEvidenceTreeReadTx(ctx, tx, p, project, run, 0, map[string]bool{}, new(int))
}
