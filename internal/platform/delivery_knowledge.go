package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

const (
	deliveryKnowledgeProtocol = "suma-accepted-delivery-v1"
	deliveryKnowledgeChanged  = "正式验收交付依据已变化或不可用"
	deliveryKnowledgeLimited  = "已验收交付证据超出知识来源范围，不会自动截断"
)

// This selector carries only the storage tenant into existing fact readers.
// It is not an authenticated Actor, a reviewer or a permission grant. Every
// caller must already own current project authority and the transaction.
// current selects snapshot/current SQL for the new reads and shared readers
// that support it. Reused composition/candidate gates retain their existing
// strong reads; false does not promise that the entire call graph is lock-free.
// The outer transaction must therefore support those existing gates.
func (s *Service) resolveDeliveryKnowledgeSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, taskID string, current bool) (out ProjectKnowledgeSource, err error) {
	task, err := scanRecord(tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, current)), tenant, project, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	selector := Principal{TenantID: tenant}
	accepted, err := s.releaseTaskBasisReadTx(ctx, tx, selector, project, taskID, task.RunID, current)
	if err != nil {
		return out, err
	}
	if err = s.checkTaskRequirementDesignRunReadTx(ctx, tx, tenant, project, taskID, task.RunID, true, current); err != nil {
		return out, err
	}
	if err = s.checkCandidateCompositionAcceptanceTx(ctx, tx, selector, project, task.RunID); err != nil {
		return out, err
	}
	if err = s.checkCurrentTaskTestPlanRunTx(ctx, tx, tenant, project, taskID, task.RunID); err != nil {
		return out, err
	}
	basis := DeliveryKnowledgeBasis{Task: accepted, Tests: []DeliveryKnowledgeTestEvidence{}, Candidates: []DeliveryKnowledgeCandidateEvidence{}}
	if err = s.deliveryKnowledgeRunEvidenceTx(ctx, tx, tenant, project, task.RunID, current, 0, map[string]bool{}, &basis); err != nil {
		return out, err
	}
	if len(basis.Tests) == 0 {
		return out, conflict(deliveryKnowledgeChanged)
	}
	if err = deliveryKnowledgeFinalizeBasis(&basis); err != nil {
		return out, err
	}
	var reason string
	if err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT reason FROM suma_reviews WHERE tenant_id=? AND project_id=? AND id=? AND task_id=? AND task_version=? AND run_id=? AND decision='accepted'`, current)), tenant, project, accepted.ReviewID, task.ID, task.Version-1, task.RunID).Scan(&reason); err != nil {
		return out, deliveryKnowledgeFactError(err)
	}
	// The visible body names the complete typed basis by digest and counts. The
	// basis itself carries every exact proof; no raw result, command, path, XML,
	// private conversation or editable configuration becomes model context.
	parts := []string{"# " + task.Title}
	if task.Description != "" {
		parts = append(parts, task.Description)
	}
	if len(task.AcceptanceCriteria) > 0 {
		parts = append(parts, "## 验收条件\n"+strings.Join(task.AcceptanceCriteria, "\n"))
	}
	parts = append(parts, "## 正式评审\n"+reason, "## 交付依据\n"+stringify(struct {
		Protocol   string           `json:"protocol"`
		Task       ReleaseTaskBasis `json:"task"`
		RunVersion int              `json:"run_version"`
		Tests      int              `json:"tests"`
		Candidates int              `json:"candidates"`
		SHA256     string           `json:"basis_sha256"`
	}{basis.Protocol, basis.Task, basis.RunVersion, len(basis.Tests), len(basis.Candidates), basis.SHA256}))
	body := strings.Join(parts, "\n\n")
	if !knowledgeText(body, 16000) {
		return out, conflict(deliveryKnowledgeLimited)
	}
	return ProjectKnowledgeSource{Kind: "accepted_delivery", ID: task.ID, Version: task.Version, Title: task.Title, Quote: body, SHA256: digest(body), Delivery: &basis}, nil
}

func (s *Service) GetDeliveryKnowledgeSource(ctx context.Context, actor Actor, project, taskID string) (out DeliveryKnowledgeSourceView, err error) {
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	source, err := s.resolveDeliveryKnowledgeSourceTx(ctx, auth.tx, auth.target.tenantID, project, taskID, true)
	if err != nil {
		return out, err
	}
	return DeliveryKnowledgeSourceView{ProjectID: project, Source: source}, nil
}

func deliveryKnowledgeFinalizeBasis(basis *DeliveryKnowledgeBasis) error {
	if len(basis.Tests) > 256 || len(basis.Candidates) > 128 {
		return conflict(deliveryKnowledgeLimited)
	}
	if basis.Tests == nil {
		basis.Tests = []DeliveryKnowledgeTestEvidence{}
	}
	if basis.Candidates == nil {
		basis.Candidates = []DeliveryKnowledgeCandidateEvidence{}
	}
	less := func(ar, an string, aa int, af int64, aw, br, bn string, ba int, bf int64, bw string) bool {
		if ar != br {
			return ar < br
		}
		if an != bn {
			return an < bn
		}
		if aa != ba {
			return aa < ba
		}
		if af != bf {
			return af < bf
		}
		return aw < bw
	}
	sort.Slice(basis.Tests, func(i, j int) bool {
		a, b := basis.Tests[i], basis.Tests[j]
		return less(a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID, b.RunID, b.NodeID, b.Attempt, b.Fence, b.WorkerID)
	})
	sort.Slice(basis.Candidates, func(i, j int) bool {
		a, b := basis.Candidates[i], basis.Candidates[j]
		return less(a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID, b.RunID, b.NodeID, b.Attempt, b.Fence, b.WorkerID)
	})
	basis.Protocol, basis.SHA256 = deliveryKnowledgeProtocol, ""
	basis.SHA256 = digest(stringify(*basis))
	return nil
}

func deliveryKnowledgeEvidenceKey(node string, attempt int, fence int64, worker string) string {
	return stringify([]any{node, attempt, fence, worker})
}

func deliveryKnowledgeFactError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return conflict(deliveryKnowledgeChanged)
	}
	return err
}

// Read persisted exact attempts, including selected subruns. A successful
// container node is not a substitute for its actual test execution evidence.
func (s *Service) deliveryKnowledgeRunEvidenceTx(ctx context.Context, tx *sql.Tx, tenant, project, runID string, current bool, depth int, seen map[string]bool, basis *DeliveryKnowledgeBasis) error {
	if depth > 32 || seen[runID] {
		return conflict(deliveryKnowledgeChanged)
	}
	seen[runID] = true
	var state string
	var version int
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`, current)), tenant, project, runID).Scan(&version, &state); err != nil {
		return deliveryKnowledgeFactError(err)
	}
	if state != "succeeded" {
		return conflict(deliveryKnowledgeChanged)
	}
	if depth == 0 {
		basis.RunVersion = version
	}
	rows, err := tx.QueryContext(ctx, s.q(s.executionReadSQL(`SELECT id,state,attempt,fence,worker_id,spec FROM dag_nodes WHERE run_id=? ORDER BY id`, current)), runID)
	if err != nil {
		return err
	}
	type nodeFact struct {
		id, state, worker, raw string
		attempt                int
		fence                  int64
	}
	nodes := []nodeFact{}
	for rows.Next() {
		var n nodeFact
		if err = rows.Scan(&n.id, &n.state, &n.attempt, &n.fence, &n.worker, &n.raw); err != nil {
			break
		}
		nodes = append(nodes, n)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	reports, err := s.testPlanReportsTx(ctx, tx, tenant, project, runID)
	if err != nil {
		return err
	}
	reportByAttempt := map[string]TestPlanReport{}
	for _, report := range reports {
		key := deliveryKnowledgeEvidenceKey(report.NodeID, report.Attempt, report.Fence, report.WorkerID)
		if _, exists := reportByAttempt[key]; exists {
			return conflict(deliveryKnowledgeChanged)
		}
		reportByAttempt[key] = report
	}
	plan, hasPlan, err := s.testPlanRunTx(ctx, tx, tenant, project, runID)
	if err != nil {
		return err
	}
	run := dag.Run{ID: runID, State: state, Nodes: []dag.NodeRun{}}
	for _, n := range nodes {
		var spec dag.Node
		if json.Unmarshal([]byte(n.raw), &spec) != nil {
			return conflict(deliveryKnowledgeChanged)
		}
		if n.state == "skipped" || spec.Type != "test" && spec.Type != "command" && spec.Type != "agent" {
			continue
		}
		if n.state != "succeeded" || n.attempt < 1 || n.fence < 1 || n.worker == "" {
			return conflict(deliveryKnowledgeChanged)
		}
		if spec.Type == "test" && len(basis.Tests) >= 256 || spec.Type != "test" && len(basis.Candidates) >= 128 {
			return conflict(deliveryKnowledgeLimited)
		}
		var raw, attemptState string
		if err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT state,result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`, current)), runID, n.id, n.attempt, n.fence, n.worker).Scan(&attemptState, &raw); err != nil {
			return deliveryKnowledgeFactError(err)
		}
		var result dag.Result
		if json.Unmarshal([]byte(raw), &result) != nil || attemptState != "succeeded" || result.RunID != runID || result.NodeID != n.id || result.Attempt != n.attempt || result.Fence != n.fence || result.WorkerID != n.worker || result.ExitCode != 0 || result.Error != "" || result.Stopped || result.OutcomeUnknown {
			return conflict(deliveryKnowledgeChanged)
		}
		run.Nodes = append(run.Nodes, dag.NodeRun{ID: n.id, Type: spec.Type, State: n.state, Attempt: n.attempt, Fence: n.fence, WorkerID: n.worker, WorkspaceEvidence: result.WorkspaceEvidence, ToolEvidence: result.ToolEvidence})
		if spec.Type == "test" {
			proof := DeliveryKnowledgeTestEvidence{RunID: runID, NodeID: n.id, Attempt: n.attempt, Fence: n.fence, WorkerID: n.worker, ResultSHA256: digest(raw)}
			if len(result.WorkspaceEvidence) > 0 {
				proof.SourceCommit, err = workspaceRevision(result.WorkspaceEvidence)
				if err != nil {
					return err
				}
			}
			if hasPlan && plan.Revision != nil {
				proof.PlanID, proof.PlanRevision, proof.PlanSHA256 = plan.PlanID, plan.Revision.Revision, plan.Revision.SHA256
			}
			if report, found := reportByAttempt[deliveryKnowledgeEvidenceKey(n.id, n.attempt, n.fence, n.worker)]; found {
				if report.RunID != runID || report.Eligibility != "current" || report.ProcessExitCode != 0 || report.OutcomeUnknown || report.SourceCommit != proof.SourceCommit || !contentDigest.MatchString(report.SHA256) || result.TestReport == nil || report.SHA256 != result.TestReport.SHA256 {
					return conflict(deliveryKnowledgeChanged)
				}
				proof.ReportID, proof.ReportSHA256 = report.ID, report.SHA256
			}
			basis.Tests = append(basis.Tests, proof)
		} else {
			head, e := workspaceRevision(result.WorkspaceEvidence)
			if e != nil {
				return e
			}
			proof := DeliveryKnowledgeCandidateEvidence{Kind: "workspace_revision", RunID: runID, NodeID: n.id, Attempt: n.attempt, Fence: n.fence, WorkerID: n.worker, HeadCommit: head, EvidenceSHA256: result.WorkspaceEvidence["evidence_sha256"].(string)}
			if bindingRaw, hasBundle := result.WorkspaceEvidence["candidate_bundle"]; hasBundle {
				var binding cc.Binding
				if json.Unmarshal([]byte(stringify(bindingRaw)), &binding) != nil || !binding.ValidSealed() || binding.ProjectID != project || binding.Source.RunID != runID || binding.Source.NodeID != n.id || binding.Source.Attempt != n.attempt || binding.Source.Fence != n.fence || binding.Source.WorkerID != n.worker {
					return conflict(deliveryKnowledgeChanged)
				}
				var stored string
				if err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT binding FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND id=?`, current)), tenant, project, binding.TransferID).Scan(&stored); err != nil {
					return deliveryKnowledgeFactError(err)
				}
				var exact cc.Binding
				if json.Unmarshal([]byte(stored), &exact) != nil || exact != binding {
					return conflict(deliveryKnowledgeChanged)
				}
				if err = s.candidateSourceTx(ctx, tx, candidateSubject{tenant: tenant, project: project}, binding); err != nil {
					return err
				}
				proof.Kind, proof.TransferID, proof.BundleSHA256 = "sealed_bundle", binding.TransferID, binding.BundleSHA256
			}
			basis.Candidates = append(basis.Candidates, proof)
		}
	}
	if err = validateCodeTestEvidence(run); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, s.q(s.executionReadSQL(`SELECT x.child_run_id FROM dag_subruns x JOIN dag_nodes n ON n.run_id=x.parent_run_id AND n.id=x.node_id AND n.attempt=x.attempt WHERE x.parent_run_id=? AND n.state<>'skipped' ORDER BY x.child_run_id`, current)), runID)
	if err != nil {
		return err
	}
	children := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		children = append(children, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = s.deliveryKnowledgeRunEvidenceTx(ctx, tx, tenant, project, child, current, depth+1, seen, basis); err != nil {
			return err
		}
	}
	return nil
}
