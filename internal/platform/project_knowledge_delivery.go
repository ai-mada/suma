package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Only these sources are backed by ordinary record tables. Accepted delivery
// provenance has its own resolver and must never enter a table-name expression.
func knowledgeRecordSourceKind(kind string) bool {
	return kind == "documents" || kind == "requirements" || kind == "tasks" || kind == "issues"
}

func knowledgeDeliverySourceCurrent(ref, actual ProjectKnowledgeSource) bool {
	return ref.Kind == "accepted_delivery" && actual.Kind == ref.Kind && ref.ID == actual.ID && ref.Version == actual.Version &&
		ref.Delivery != nil && actual.Delivery != nil && stringify(ref.Delivery) == stringify(actual.Delivery) &&
		ref.Title == actual.Title && ref.SHA256 == actual.SHA256 && ref.Quote != "" && knowledgeText(ref.Quote, 16000) && strings.Contains(actual.Quote, ref.Quote)
}

// Authorization and its ordered locks belong to the caller. A batch resolves
// each accepted task once in this transaction; no cached fact survives it.
// current=false preserves the existing cross-project catalog read boundary.
func (s *Service) knowledgeSourcesCurrentTx(ctx context.Context, tx *sql.Tx, tenant, project string, refs []ProjectKnowledgeSource, records map[string]Record, current bool) ([]bool, error) {
	states := make([]bool, len(refs))
	deliveries := map[string]ProjectKnowledgeSource{}
	deliveryRead := map[string]bool{}
	batchedRecords := records != nil
	if records == nil {
		records = map[string]Record{}
	}
	recordRead := map[string]bool{}
	for key := range records {
		recordRead[key] = true
	}
	if batchedRecords {
		// A supplied map is the completed ordinary-record batch. An absent row
		// is an unavailable source, not a reason to issue an individual retry.
		for _, ref := range refs {
			if knowledgeRecordSourceKind(ref.Kind) {
				recordRead[knowledgeSourceKey(ref.Kind, ref.ID)] = true
			}
		}
	}
	for i, ref := range refs {
		if ref.Kind == "accepted_delivery" {
			if ref.Delivery == nil {
				continue
			}
			if !deliveryRead[ref.ID] {
				deliveryRead[ref.ID] = true
				actual, err := s.resolveDeliveryKnowledgeSourceTx(ctx, tx, tenant, project, ref.ID, current)
				if err != nil {
					var api *APIError
					if !errors.As(err, &api) || api.Status != 400 && api.Status != 403 && api.Status != 404 && api.Status != 409 && api.Status != 422 {
						return nil, err
					}
				} else {
					deliveries[ref.ID] = actual
				}
			}
			states[i] = knowledgeDeliverySourceCurrent(ref, deliveries[ref.ID])
			continue
		}
		if !knowledgeRecordSourceKind(ref.Kind) || ref.Delivery != nil {
			continue
		}
		key := knowledgeSourceKey(ref.Kind, ref.ID)
		if !recordRead[key] {
			recordRead[key] = true
			record, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+ref.Kind+` WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, ref.ID))
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if err == nil {
				records[key] = record
			}
		}
		states[i] = knowledgeSourceCurrent(ref, records)
	}
	return states, nil
}
