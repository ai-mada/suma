package platform

import (
	"context"
	"encoding/json"
	"strings"
)

const issueRequirementReceiptKind = "issue_requirement_intake"
const reservedRecordOperationKey = "Operation key prefix is reserved for internal workflow commands"

type issueRequirementReceipt struct {
	Kind          string `json:"kind"`
	IssueID       string `json:"issue_id"`
	RequirementID string `json:"requirement_id"`
}

func rejectReservedRecordOperationKey(key string) error {
	for _, prefix := range []string{"issue-intake-", "failure-", "review-rejected-"} {
		if strings.HasPrefix(key, prefix) {
			return invalid(reservedRecordOperationKey)
		}
	}
	return nil
}

// This domain hash is deliberately distinct from every public record-command
// hash. A generic receipt or editable source_issue_id can never establish intake.
func issueRequirementReceiptHash(tenant, project string, receipt issueRequirementReceipt) string {
	return digest(stringify(map[string]any{"domain": issueRequirementReceiptKind, "tenant_id": tenant, "project_id": project, "issue_id": receipt.IssueID, "requirement_id": receipt.RequirementID}))
}

func parseIssueRequirementReceipt(raw, hash, tenant, project, issue string) (issueRequirementReceipt, error) {
	var receipt issueRequirementReceipt
	var shape map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &shape) != nil || len(shape) != 3 || json.Unmarshal([]byte(raw), &receipt) != nil || receipt.Kind != issueRequirementReceiptKind || receipt.IssueID != issue || !handoffID(receipt.RequirementID, false) || hash != issueRequirementReceiptHash(tenant, project, receipt) {
		return receipt, conflict(diagnosisExisting)
	}
	return receipt, nil
}

// Both ordinary intake and reviewed diagnosis adoption use this single formal
// mapping. It writes one requirement plus its canonical domain receipt atomically;
// there is no secondary generic receipt, old receipt interpretation or backfill.
func (s *Service) saveSharedIssueRequirementTx(ctx context.Context, auth *authorizedCommand, project, issue string, body map[string]any) (Record, error) {
	if auth == nil {
		return Record{}, forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return Record{}, err
	}
	if existing, found, err := s.lookupIssueRequirementTx(ctx, auth, project, issue); err != nil {
		return Record{}, err
	} else if found {
		return existing, nil
	}
	if _, err := s.getRecordTx(ctx, auth, project, "issues", issue); err != nil {
		return Record{}, err
	}
	if err := rejectProtectedRecordProvenanceInput(body); err != nil {
		return Record{}, err
	}
	r := recordFrom(auth.currentPrincipal, project, body)
	r.Status = "draft"
	data := map[string]any{}
	for key, value := range r.Data {
		data[key] = value
	}
	data["source_issue_id"] = issue
	r.Data = data
	if err := s.validateRecord(ctx, auth.tx, auth, r, "requirements"); err != nil {
		return r, err
	}
	if err := s.insertCommandRecord(ctx, auth.tx, auth, r, "requirements"); err != nil {
		return r, err
	}
	if err := s.commandActivity(ctx, auth, "requirements.created", r.ID, r.Title); err != nil {
		return r, err
	}
	receipt := issueRequirementReceipt{Kind: issueRequirementReceiptKind, IssueID: issue, RequirementID: r.ID}
	if err := s.saveCommandReceipt(ctx, auth, "issue-intake-"+issue, issueRequirementReceiptHash(auth.target.tenantID, project, receipt), receipt); err != nil {
		return r, err
	}
	return r, nil
}
