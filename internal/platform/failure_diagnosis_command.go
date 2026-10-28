package platform

import (
	"context"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) CommandFailureDiagnosis(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req FailureDiagnosisCommandRequest) (out FailureDiagnosisAnalysis, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || req.Action != "revise" && req.Action != "adopt" && req.Action != "dismiss" || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) {
		return out, invalid(diagnosisInvalid)
	}
	if req.Action == "revise" {
		if req.Repair == nil || !validateFailureRepair(*req.Repair) {
			return out, invalid(diagnosisOutput)
		}
	} else if req.Repair != nil {
		return out, invalid(diagnosisInvalid)
	}
	if req.Action == "adopt" && !req.AcknowledgeDraft {
		return out, invalid(diagnosisDraft)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"failure_diagnosis_command": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return s.failureDiagnosisProjectionTx(ctx, auth, out)
	}
	r, err := s.failureDiagnosisTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	out = r.Analysis
	if out.Version != req.ExpectedVersion || req.Action == "dismiss" && out.State != "ready" && out.State != "failed" || req.Action != "dismiss" && out.State != "ready" {
		return out, conflict(diagnosisVersion)
	}
	switch req.Action {
	case "revise":
		copy := *req.Repair
		copy.AcceptanceCriteria = append([]string{}, req.Repair.AcceptanceCriteria...)
		out.Repair = &copy
	case "dismiss":
		out.State = "dismissed"
	case "adopt":
		existing, found, e := s.lookupIssueRequirementTx(ctx, auth, project, out.Source.IssueID)
		if e != nil {
			return out, e
		}
		if found {
			out.AdoptedRequirementID = existing.ID
			out.Reason = diagnosisExisting
			out, e = s.failureDiagnosisProjectionTx(ctx, auth, out)
			if e != nil {
				return out, e
			}
			if e = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); e != nil {
				return out, e
			}
			return out, auth.tx.Commit()
		}
		if out.Repair == nil || !validateFailureRepair(*out.Repair) {
			return out, invalid(diagnosisOutput)
		}
		if err = s.checkFailureDiagnosisSourceTx(ctx, auth.tx, auth.target.tenantID, project, out.Source); err != nil {
			return out, err
		}
		issue, e := s.getRecordTx(ctx, auth, project, "issues", out.Source.IssueID)
		if e != nil {
			return out, e
		}
		draft := out.Repair
		criteria := make([]any, len(draft.AcceptanceCriteria))
		for i, text := range draft.AcceptanceCriteria {
			criteria[i] = text
		}
		requirement, e := s.saveSharedIssueRequirementTx(ctx, auth, project, issue.ID, map[string]any{"title": draft.Title, "description": draft.Description, "priority": draft.Priority, "acceptance_criteria": criteria, "status": "draft", "data": map[string]any{"source_issue_id": issue.ID, "source": issue.Data, "failure_diagnosis_id": id}})
		if e != nil {
			return out, e
		}
		lineage := FailureDiagnosisAdoption{AnalysisID: id, AnalysisVersion: out.Version, DiagnosisRunID: out.RunID, SourceRunID: out.Source.RunID, SourceNodeID: out.Source.NodeID, SourceAttempt: out.Source.Attempt, SourceFence: out.Source.Fence, IssueID: issue.ID, ToolProfileSHA256: out.ToolProfileSHA256, SourceSHA256: out.Source.SHA256, OriginalOutputSHA256: out.OriginalOutputSHA256, RepairSHA256: digest(stringify(draft)), RequirementID: requirement.ID, RequirementVersion: requirement.Version, AdoptedBy: auth.currentPrincipal.ID, AdoptedAt: now(), Reason: req.Reason}
		if _, e = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_failure_diagnosis_adoptions(tenant_id,project_id,issue_id,requirement_id,analysis_id,created_at,snapshot)VALUES(?,?,?,?,?,?,?)`), auth.target.tenantID, project, issue.ID, requirement.ID, id, lineage.AdoptedAt, stringify(lineage)); e != nil {
			return out, e
		}
		out.State = "adopted"
		out.AdoptedRequirementID = requirement.ID
	}
	old := out.Version
	out.Version++
	out.UpdatedAt = now()
	out.Reason = req.Reason
	r.Analysis = out
	if err = s.saveFailureDiagnosisTx(ctx, auth.tx, auth.target.tenantID, r, old); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "failure.diagnosis_"+req.Action, id, req.Reason); err != nil {
		return out, err
	}
	out, err = s.failureDiagnosisProjectionTx(ctx, auth, out)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
