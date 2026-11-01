package platform

import (
	"context"
	"encoding/json"
	"time"

	ic "github.com/ai-mada/suma/internal/indexcontract"
)

// Keep documents' actual indexed body in one place for the producer and the
// current-authorized readback. Title is separate metadata, not an offset prefix.
func indexedDocumentContent(record Record) string {
	content := record.Description
	if body, ok := record.Data["content"].(string); ok {
		content += "\n" + body
	}
	return content
}
func (s *Service) appendIndexEventTx(ctx context.Context, auth *authorizedCommand, request ic.Request) error {
	if auth == nil || request.TenantID != auth.target.tenantID {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, request.ProjectID); err != nil {
		return err
	}
	if err := ic.ValidateRequest(request); err != nil {
		return invalidDiagnostic("knowledge.index_source_invalid", "索引来源或正文超出当前合同范围", nil)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	// Both source transition and outbox insertion use the original authority/CAS
	// transaction. The immutable payload is later sent as these exact bytes.
	_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_index_outbox(id,tenant_id,project_id,source_kind,source_id,revision,content_revision,source_sha256,action,payload,state,created_at,receipt,indexed_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'','')`), request.EventID, request.TenantID, request.ProjectID, request.Source.Kind, request.Source.ID, request.Source.Sequence, request.Source.Revision, request.Source.SHA256, request.Action, string(raw), "pending", auth.checkedAt.UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Service) knowledgeIndexEventTx(ctx context.Context, auth *authorizedCommand, current ProjectKnowledgeDetail, revision ProjectKnowledgeRevision, action string) error {
	request := ic.Request{Protocol: ic.Protocol, EventID: newID(), TenantID: auth.target.tenantID, ProjectID: current.ProjectID, Source: ic.Source{Kind: "project_knowledge", ID: current.ID, Sequence: current.Version + 1}, Action: action}
	if action == "upsert" {
		request.Source.Revision = revision.Revision
		request.Source.SHA256 = revision.SHA256
		request.Title = revision.Title
		request.Content = revision.Content
	} else if current.Published != nil {
		request.Source.Revision = current.Published.Revision
		request.Source.SHA256 = current.Published.SHA256
	}
	return s.appendIndexEventTx(ctx, auth, request)
}
