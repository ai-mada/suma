package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func knowledgeIntelligenceProfile(p ToolProfile) bool {
	return p.State == "enabled" && p.Profile.Adapter == "suma" && p.Profile.ReadOnly && p.Profile.WorkspaceToolsVersion == "" && p.Profile.Planning == nil && p.Profile.KnowledgeExtraction != nil && p.Profile.KnowledgeExtraction.Protocol == toolcontract.KnowledgeExtractionProtocol
}
func (s *Service) knowledgeIntelligenceSourceTx(ctx context.Context, auth *authorizedCommand, project string, refs []ProjectKnowledgeSourceInput) (out KnowledgeIntelligenceSource, err error) {
	if len(refs) < 1 || len(refs) > 8 {
		return out, invalid(intelligenceInvalid)
	}
	out.Sources, err = s.resolveKnowledgeSourcesTx(ctx, auth, project, refs)
	if err != nil {
		return out, err
	}
	out.SHA256 = digest(stringify(out.Sources))
	return
}

// This pure formal-source check follows current project authority. It never
// constructs an Actor, opens another transaction or treats a snapshot as access.
func (s *Service) checkKnowledgeIntelligenceSourceTx(ctx context.Context, tx *sql.Tx, tenant, project string, source KnowledgeIntelligenceSource) error {
	if len(source.Sources) < 1 || len(source.Sources) > 8 || digest(stringify(source.Sources)) != source.SHA256 {
		return conflict(intelligenceSource)
	}
	var status string
	e := tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_projects WHERE tenant_id=? AND id=?`), tenant, project).Scan(&status)
	if errors.Is(e, sql.ErrNoRows) {
		return conflict(intelligenceSource)
	}
	if e != nil {
		return e
	}
	if status == "archived" || status == "deleted" {
		return conflict(intelligenceSource)
	}
	for _, ref := range source.Sources {
		if !knowledgeSourceKind(ref.Kind) {
			return conflict(intelligenceSource)
		}
		record, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+ref.Kind+` WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, ref.ID))
		if errors.Is(e, sql.ErrNoRows) {
			return conflict(intelligenceSource)
		}
		if e != nil {
			return e
		}
		if !knowledgeSourceCurrent(ref, map[string]Record{knowledgeSourceKey(ref.Kind, ref.ID): record}) {
			return conflict(intelligenceSource)
		}
	}
	return nil
}
func parseKnowledgeIntelligenceOutput(raw string, source KnowledgeIntelligenceSource) (out KnowledgeIntelligenceOutput, err error) {
	invalidOutput := func() (KnowledgeIntelligenceOutput, error) {
		return KnowledgeIntelligenceOutput{}, invalid(intelligenceOutput)
	}
	if len(raw) > 64<<10 || !utf8.ValidString(raw) || validateIngressJSON([]byte(raw)) != nil {
		return invalidOutput()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return invalidOutput()
	}
	for _, key := range []string{"schema_version", "title", "summary", "content", "source_indices", "rationale"} {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return invalidOutput()
		}
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil {
		return invalidOutput()
	}
	if _, e := d.Token(); e != io.EOF {
		return invalidOutput()
	}
	if out.SchemaVersion != 1 || !planningText(out.Title, 200) || !knowledgeText(out.Summary, 1000) || !planningText(out.Content, 16000) || !planningText(out.Rationale, 8000) || len(out.SourceIndices) < 1 || len(out.SourceIndices) > len(source.Sources) {
		return invalidOutput()
	}
	seen := map[int]bool{}
	for _, index := range out.SourceIndices {
		if index < 0 || index >= len(source.Sources) || seen[index] {
			return invalidOutput()
		}
		seen[index] = true
	}
	return out, nil
}
