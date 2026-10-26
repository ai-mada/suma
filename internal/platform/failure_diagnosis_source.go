package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func failureDiagnosisProfile(p ToolProfile) bool {
	f := p.Profile
	return p.State == "enabled" && f.Adapter == "suma" && f.ReadOnly && f.WorkspaceToolsVersion == "" && f.Planning == nil && f.KnowledgeExtraction == nil && f.FailureDiagnosis != nil && f.FailureDiagnosis.Protocol == toolcontract.FailureDiagnosisProtocol
}
func diagnosisID(v string, max int) bool {
	return len(v) > 0 && len(v) <= max && utf8.ValidString(v) && !strings.ContainsAny(v, "\x00\r\n/\\")
}
func diagnosisEvidence(kind, text string) (FailureDiagnosisEvidence, error) {
	if !utf8.ValidString(text) {
		return FailureDiagnosisEvidence{}, conflict(diagnosisUncertain)
	}
	e := FailureDiagnosisEvidence{Kind: kind, Text: text, SHA256: digest(text), TotalBytes: len(text), SelectedBytes: len(text), Selection: "full"}
	if len(text) > 8000 {
		start := len(text) - 8000
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		e.Text = text[start:]
		e.SelectedBytes = len(e.Text)
		e.Selection = "tail"
	}
	return e, nil
}
func (s *Service) failureIssueForRunTx(ctx context.Context, auth *authorizedCommand, project, run string) (issue Record, found bool, err error) {
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), auth.target.tenantID, project, "failure-"+run).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return issue, false, nil
	}
	if err != nil {
		return issue, false, err
	}
	var ref struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(raw), &ref) != nil || !handoffID(ref.ID, false) {
		return issue, false, conflict(diagnosisIssue)
	}
	issue, err = s.getRecordTx(ctx, auth, project, "issues", ref.ID)
	return issue, err == nil, err
}
func (s *Service) failureDiagnosisSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, issueID, run, node string, attempt int) (out FailureDiagnosisSource, err error) {
	issue, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_issues WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, issueID))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if issue.Data["run_id"] != run {
		return out, conflict(diagnosisSource)
	}
	var output, errorText, attemptState, raw string
	var storedFence int64
	var storedWorker string
	out = FailureDiagnosisSource{IssueID: issue.ID, IssueVersion: issue.Version, RunID: run, NodeID: node, Evidence: []FailureDiagnosisEvidence{}}
	err = tx.QueryRowContext(ctx, s.q(`SELECT r.version,r.state,n.state,n.attempt,n.fence,n.worker_id,n.output,n.error_text,a.state,a.fence,a.worker_id,a.result FROM dag_runs r JOIN dag_nodes n ON n.run_id=r.id JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), tenant, project, run, node).Scan(&out.RunVersion, &out.RunState, &out.NodeState, &out.Attempt, &out.Fence, &out.WorkerID, &output, &errorText, &attemptState, &storedFence, &storedWorker, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, conflict(diagnosisUncertain)
	}
	if err != nil {
		return out, err
	}
	var result dag.Result
	if out.NodeState != "failed" || attemptState != "failed" || out.RunState == "reconciling" || out.Attempt != attempt || attempt < 1 || out.Fence < 1 || storedFence != out.Fence || storedWorker != out.WorkerID || json.Unmarshal([]byte(raw), &result) != nil || result.OutcomeUnknown || result.RunID != run || result.NodeID != node || result.Attempt != attempt || result.Fence != out.Fence || result.WorkerID != out.WorkerID {
		return out, conflict(diagnosisUncertain)
	}
	var unknown int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND state='reconciling'`), run).Scan(&unknown); err != nil {
		return out, err
	}
	if unknown > 0 {
		return out, conflict(diagnosisUncertain)
	}
	texts := []struct{ kind, text string }{{"issue", issue.Title + "\n" + issue.Description}}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND run_id=? ORDER BY id LIMIT 2`), tenant, project, run)
	if err != nil {
		return out, err
	}
	tasks := []Record{}
	for rows.Next() {
		task, e := scanRecord(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		tasks = append(tasks, task)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(tasks) > 1 {
		return out, conflict(diagnosisSource)
	}
	if len(tasks) == 1 {
		task := tasks[0]
		out.TaskID, out.TaskVersion, out.TaskTitle = task.ID, task.Version, task.Title
		texts = append(texts, struct{ kind, text string }{"task_description", task.Description}, struct{ kind, text string }{"task_acceptance", stringify(task.AcceptanceCriteria)})
		if task.RequirementID != "" {
			requirement, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, task.RequirementID))
			if e != nil {
				return out, e
			}
			out.RequirementID, out.RequirementVersion = requirement.ID, requirement.Version
			texts = append(texts, struct{ kind, text string }{"requirement", requirement.Title + "\n" + requirement.Description})
		}
	}
	texts = append(texts, struct{ kind, text string }{"node_error", errorText}, struct{ kind, text string }{"node_output", output})
	for _, text := range texts {
		e, e2 := diagnosisEvidence(text.kind, text.text)
		if e2 != nil {
			return out, e2
		}
		out.Evidence = append(out.Evidence, e)
	}
	out.SHA256 = digest(stringify(out))
	return out, nil
}
func (s *Service) checkFailureDiagnosisSourceTx(ctx context.Context, tx *sql.Tx, tenant, project string, source FailureDiagnosisSource) error {
	current, err := s.failureDiagnosisSourceTx(ctx, tx, tenant, project, source.IssueID, source.RunID, source.NodeID, source.Attempt)
	if err != nil {
		return err
	}
	if current.SHA256 != source.SHA256 {
		return conflict(diagnosisSource)
	}
	return nil
}

func diagnosisObject(raw json.RawMessage, keys ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(keys) {
		return false
	}
	for _, key := range keys {
		v, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return false
		}
	}
	return true
}
func validateFailureRepair(d FailureRepairDraft) bool {
	if !planningText(d.Title, 200) || !planningText(d.Description, 16000) || d.Priority != "low" && d.Priority != "medium" && d.Priority != "high" || len(d.AcceptanceCriteria) < 1 || len(d.AcceptanceCriteria) > 16 {
		return false
	}
	for _, v := range d.AcceptanceCriteria {
		if !planningText(v, 1000) {
			return false
		}
	}
	return true
}
func diagnosisReferences(indices []int, count int) bool {
	if len(indices) < 1 || len(indices) > 6 {
		return false
	}
	seen := map[int]bool{}
	for _, i := range indices {
		if i < 0 || i >= count || seen[i] {
			return false
		}
		seen[i] = true
	}
	return true
}
func parseFailureDiagnosisOutput(raw string, source FailureDiagnosisSource) (out FailureDiagnosisOutput, err error) {
	bad := func() (FailureDiagnosisOutput, error) { return FailureDiagnosisOutput{}, invalid(diagnosisOutput) }
	if len(raw) > 64<<10 || !utf8.ValidString(raw) || validateIngressJSON([]byte(raw)) != nil || !diagnosisObject([]byte(raw), "schema_version", "summary", "observations", "hypotheses", "recommended_actions", "test_plan", "repair") {
		return bad()
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	if !diagnosisObject(fields["repair"], "title", "description", "priority", "acceptance_criteria") {
		return bad()
	}
	for _, key := range []string{"observations", "hypotheses"} {
		var entries []json.RawMessage
		if json.Unmarshal(fields[key], &entries) != nil {
			return bad()
		}
		for _, entry := range entries {
			keys := []string{"statement", "evidence_indices"}
			if key == "hypotheses" {
				keys = append(keys, "confidence")
			}
			if !diagnosisObject(entry, keys...) {
				return bad()
			}
		}
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.SchemaVersion != 1 || !planningText(out.Summary, 2000) || len(out.Observations) < 1 || len(out.Observations) > 8 || len(out.Hypotheses) > 8 || out.Hypotheses == nil || !validateFailureRepair(out.Repair) {
		return bad()
	}
	for _, o := range out.Observations {
		if !planningText(o.Statement, 2000) || !diagnosisReferences(o.EvidenceIndices, len(source.Evidence)) {
			return bad()
		}
	}
	for _, h := range out.Hypotheses {
		if !planningText(h.Statement, 2000) || !diagnosisReferences(h.EvidenceIndices, len(source.Evidence)) || h.Confidence != "low" && h.Confidence != "medium" && h.Confidence != "high" {
			return bad()
		}
	}
	for _, list := range [][]string{out.RecommendedActions, out.TestPlan} {
		if len(list) < 1 || len(list) > 8 {
			return bad()
		}
		for _, v := range list {
			if !planningText(v, 2000) {
				return bad()
			}
		}
	}
	return out, nil
}
