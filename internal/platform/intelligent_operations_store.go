package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type intelligentOperationRecord struct {
	Analysis              IntelligentOperationAnalysis `json:"analysis"`
	CreatorCredentialHash string                       `json:"creator_credential_hash"`
	WorkerID              string                       `json:"worker_id"`
	Route                 dag.ProjectExecutionRoute    `json:"route"`
	// System attribution is protected domain state. Matching human text must
	// never become a host translation key.
	SystemReason bool `json:"system_reason"`
}

func (s *Service) InitializeIntelligentOperations(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_intelligent_operations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_intelligent_operation_executions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,analysis_id VARCHAR(64) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,result_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,result TEXT NOT NULL,PRIMARY KEY(analysis_id,attempt,fence,result_sha256))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_intelligent_operation_page", "suma_intelligent_operations", "tenant_id", "project_id", "created_at", "id")
}

func (s *Service) intelligentOperationTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out intelligentOperationRecord, err error) {
	var raw, state string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,state,payload FROM suma_intelligent_operations WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&version, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.Analysis.ID != id || out.Analysis.ProjectID != project || out.Analysis.Version != version || out.Analysis.State != state {
		return out, conflict(operationVersion)
	}
	return out, nil
}

func (s *Service) saveIntelligentOperationTx(ctx context.Context, tx *sql.Tx, tenant string, record intelligentOperationRecord, old int) error {
	a := record.Analysis
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_intelligent_operations SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(record), tenant, a.ProjectID, a.ID, old)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return conflict(operationVersion)
	}
	return nil
}

func (s *Service) intelligentOperationExecutionRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (record intelligentOperationRecord, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_intelligent_operations WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, nil
	}
	if err != nil {
		return record, false, err
	}
	record, err = s.intelligentOperationTx(ctx, tx, tenant, project, id)
	return record, true, err
}

func intelligentOperationProfile(p ToolProfile) bool {
	return p.State == "enabled" && p.Profile.Adapter == "suma" && p.Profile.ReadOnly && p.Profile.WorkspaceToolsVersion == "" && p.Profile.ModelExecutionProtocol == "" && p.Profile.Planning == nil && p.Profile.KnowledgeExtraction == nil && p.Profile.FailureDiagnosis == nil && p.Profile.IntelligentOperations != nil && p.Profile.IntelligentOperations.Protocol == toolcontract.IntelligentOperationsProtocol
}

// Only these visible text fields may enter the fixed model request. Record.Data,
// document metadata, configuration and credentials are deliberately excluded.
func operationSourceText(kind string, r Record) string {
	parts := []string{}
	if r.Description != "" {
		parts = append(parts, r.Description)
	}
	if (kind == "requirements" || kind == "tasks") && len(r.AcceptanceCriteria) > 0 {
		parts = append(parts, strings.Join(r.AcceptanceCriteria, "\n"))
	}
	return strings.Join(parts, "\n")
}

func (s *Service) operationSourceTx(ctx context.Context, auth *authorizedCommand, project string, inputs []ProjectKnowledgeSourceInput) (source IntelligentOperationSource, err error) {
	source.Sources = []ProjectKnowledgeSource{}
	if inputs == nil || len(inputs) > 8 {
		return source, invalid(operationInvalid)
	}
	seen := map[string]bool{}
	for _, input := range inputs {
		key := knowledgeSourceKey(input.Kind, input.ID)
		if !knowledgeSourceKind(input.Kind) || !handoffID(input.ID, false) || input.Version < 1 || input.Version > 2147483646 || !knowledgeText(input.Quote, 16000) || seen[key] {
			return source, invalid(operationInvalid)
		}
		seen[key] = true
		r, e := s.getRecordTx(ctx, auth, project, input.Kind, input.ID)
		if e != nil {
			return source, e
		}
		if r.Version != input.Version || r.Status == "archived" || r.Status == "deleted" {
			return source, conflict(operationSource)
		}
		text := operationSourceText(input.Kind, r)
		canonicalText := knowledgeSourceText(input.Kind, r)
		quote := input.Quote
		if quote == "" {
			quote = text
		}
		if !planningText(quote, 16000) || !strings.Contains(text, quote) || !strings.Contains(canonicalText, quote) {
			return source, invalid(operationInvalid)
		}
		if !operationSafeText(r.Title) || !operationSafeText(quote) {
			return source, invalid(operationSensitive)
		}
		// Hash the same formal source body used by ProjectKnowledgeSource. Only
		// the safe quote and this digest enter the model, never Record.Data.
		source.Sources = append(source.Sources, ProjectKnowledgeSource{Kind: input.Kind, ID: input.ID, Version: input.Version, Title: r.Title, Quote: quote, SHA256: digest(canonicalText)})
	}
	source.SHA256 = digest(stringify(source.Sources))
	return source, s.checkIntelligentOperationSourceTx(ctx, auth.tx, auth.target.tenantID, project, source)
}

func (s *Service) checkIntelligentOperationSourceTx(ctx context.Context, tx *sql.Tx, tenant, project string, source IntelligentOperationSource) error {
	if source.Sources == nil || len(source.Sources) > 8 || digest(stringify(source.Sources)) != source.SHA256 {
		return conflict(operationSource)
	}
	var status string
	err := tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_projects WHERE tenant_id=? AND id=?`), tenant, project).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return conflict(operationSource)
	}
	if err != nil {
		return err
	}
	if status == "archived" || status == "deleted" {
		return conflict(operationSource)
	}
	for _, ref := range source.Sources {
		if !knowledgeSourceKind(ref.Kind) {
			return conflict(operationSource)
		}
		r, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+ref.Kind+` WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, ref.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return conflict(operationSource)
		}
		if err != nil {
			return err
		}
		text := operationSourceText(ref.Kind, r)
		canonicalText := knowledgeSourceText(ref.Kind, r)
		if r.Status == "archived" || r.Status == "deleted" || r.Version != ref.Version || r.Title != ref.Title || ref.SHA256 != digest(canonicalText) || ref.Quote == "" || !strings.Contains(text, ref.Quote) || !strings.Contains(canonicalText, ref.Quote) {
			return conflict(operationSource)
		}
	}
	return nil
}
