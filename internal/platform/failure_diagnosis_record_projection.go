package platform

import "context"

// IssueRequirement is a current-authorized read of the durable intake slot.
// Refreshing a page never creates a draft or replays somebody else's mutation.
func (s *Service) IssueRequirement(ctx context.Context, actor Actor, project, issueID string) (Record, error) {
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, issueID, purposeProjectRead)
	if err != nil {
		return Record{}, err
	}
	defer close()
	if _, err := s.getRecordTx(ctx, auth, project, "issues", issueID); err != nil {
		return Record{}, err
	}
	current, found, err := s.lookupIssueRequirementTx(ctx, auth, project, issueID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, missing()
	}
	current.FailureDiagnosisAdoption, err = s.failureDiagnosisAdoptionTx(ctx, auth.tx, auth.target.tenantID, project, current.ID)
	return current, err
}

// Diagnosis provenance is a server-owned immutable relation. Generic record
// edits may change an ordinary requirement, but cannot manufacture this proof.
func rejectFailureAdoptionInput(body map[string]any) error {
	if _, present := body["failure_diagnosis_adoption"]; present {
		return invalid(diagnosisInvalid)
	}
	if data, ok := body["data"].(map[string]any); ok {
		if _, present := data["failure_diagnosis_adoption"]; present {
			return invalid(diagnosisInvalid)
		}
	}
	return nil
}
