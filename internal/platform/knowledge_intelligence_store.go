package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

const (
	intelligenceInvalid   = "Invalid model-assisted knowledge request"
	intelligenceOutput    = "The model output does not satisfy the fixed knowledge proposal contract"
	intelligenceSource    = "Knowledge sources have changed; create a new extraction"
	intelligenceProfile   = "Select a current enabled read-only Suma knowledge extraction profile"
	intelligenceVersion   = "The knowledge extraction has changed; reload before deciding"
	intelligenceConsent   = "Confirm that the selected source text will be sent to the configured model"
	intelligenceDraft     = "Confirm adoption as a model-assisted project draft requiring publication review"
	intelligenceFailed    = "The knowledge extraction execution did not complete successfully"
	intelligenceUnknown   = "The knowledge extraction outcome is unknown; inspect the original run"
	intelligencePrompt    = "Knowledge extraction context exceeds 32000 UTF-8 bytes; select smaller source excerpts"
	intelligenceAuthority = "The extraction creator no longer has current execution authority"
)

type knowledgeIntelligenceRecord struct {
	Analysis              KnowledgeIntelligenceAnalysis `json:"analysis"`
	CreatorCredentialHash string                        `json:"creator_credential_hash"`
	WorkerID              string                        `json:"worker_id"`
	Route                 dag.ProjectExecutionRoute     `json:"route"`
}

func (s *Service) MigrateKnowledgeIntelligence(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS suma_knowledge_intelligence(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_knowledge_intelligence_outcomes(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,analysis_id VARCHAR(64) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,result_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,result TEXT NOT NULL,PRIMARY KEY(analysis_id,attempt,fence,result_sha256))`,
		`CREATE TABLE IF NOT EXISTS suma_knowledge_intelligence_adoptions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,knowledge_id VARCHAR(64) NOT NULL,analysis_id VARCHAR(64) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,knowledge_id))`,
	} {
		if _, e := s.DB.ExecContext(ctx, s.ddl(q)); e != nil {
			return e
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_knowledge_intelligence_page", "suma_knowledge_intelligence", "tenant_id", "project_id", "created_at", "id"); err != nil {
		return err
	}
	return s.MigrateFailureDiagnosis(ctx)
}
func (s *Service) knowledgeIntelligenceTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out knowledgeIntelligenceRecord, err error) {
	var raw, state string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,state,payload FROM suma_knowledge_intelligence WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&version, &state, &raw)
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
		return out, conflict(intelligenceVersion)
	}
	return
}
func (s *Service) saveKnowledgeIntelligenceTx(ctx context.Context, tx *sql.Tx, tenant string, r knowledgeIntelligenceRecord, old int) error {
	a := r.Analysis
	result, e := tx.ExecContext(ctx, s.q(`UPDATE suma_knowledge_intelligence SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(r), tenant, a.ProjectID, a.ID, old)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict(intelligenceVersion)
	}
	return nil
}
func (s *Service) knowledgeIntelligenceExecutionRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (r knowledgeIntelligenceRecord, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_knowledge_intelligence WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r, err = s.knowledgeIntelligenceTx(ctx, tx, tenant, project, id)
	return r, true, err
}
func (s *Service) knowledgeIntelligenceAdoptionTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out *KnowledgeIntelligenceAdoption, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_knowledge_intelligence_adoptions WHERE tenant_id=? AND project_id=? AND knowledge_id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out = &KnowledgeIntelligenceAdoption{}
	err = json.Unmarshal([]byte(raw), out)
	return
}
