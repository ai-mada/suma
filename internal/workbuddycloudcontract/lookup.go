package workbuddycloudcontract

import cc "github.com/ai-mada/suma/internal/candidatecontract"

// CloudLookupRequest locates an original submission without authorizing a new
// provider operation. Assignment is the complete original request identity.
type CloudLookupRequest struct {
	ProjectID    string           `json:"project_id"`
	DelegationID string           `json:"delegation_id"`
	Assignment   cc.AssignmentRef `json:"assignment"`
}

func (r CloudLookupRequest) Validate() error {
	if !validID(r.ProjectID, 64) || !validID(r.DelegationID, 64) || !validAssignment(r.ProjectID, r.Assignment) {
		return ErrInvalid
	}
	return nil
}

// CloudLookupResult reports stored facts only. not_found does not grant create
// permission, and prepared never establishes that an external POST was unsent.
type CloudLookupResult struct {
	ProjectID    string           `json:"project_id"`
	DelegationID string           `json:"delegation_id"`
	Assignment   cc.AssignmentRef `json:"assignment"`
	State        string           `json:"state"`
	Submission   *CloudSubmission `json:"submission,omitempty"`
}

func (r CloudLookupResult) Validate() error {
	if (CloudLookupRequest{ProjectID: r.ProjectID, DelegationID: r.DelegationID, Assignment: r.Assignment}).Validate() != nil {
		return ErrInvalid
	}
	if r.State == "not_found" {
		if r.Submission != nil {
			return ErrInvalid
		}
		return nil
	}
	if r.State != "prepared" && r.State != "unknown" && r.State != "task_known" {
		return ErrInvalid
	}
	s := r.Submission
	if s == nil || s.Validate() != nil || s.State != r.State || s.ProjectID != r.ProjectID || s.DelegationID != r.DelegationID || s.Assignment != r.Assignment || s.RunID != r.Assignment.RunID || s.NodeID != r.Assignment.NodeID {
		return ErrInvalid
	}
	return nil
}
