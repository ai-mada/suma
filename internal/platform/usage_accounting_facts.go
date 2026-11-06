package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

type UsageAttemptFact struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	CreatedAt      string              `json:"created_at"`
	IdentitySHA256 string              `json:"identity_sha256"`
	OriginalResult bool                `json:"original_result"`
	Usage          *toolcontract.Usage `json:"usage,omitempty" wire:"nonnull"`
	UsageState     string              `json:"usage_state,omitempty"`
}

type UsageAttemptFactPage struct {
	ProjectID  string             `json:"project_id"`
	AttemptID  string             `json:"attempt_id"`
	Items      []UsageAttemptFact `json:"items"`
	NextCursor string             `json:"next_cursor"`
}

var errInvalidUsageFact = errors.New("invalid stored usage fact")

const usageFactSecondLayout = "2006-01-02T15:04:05"

func usageFactStamp(value string) (time.Time, error) {
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || len(value) < 20 || len(value) > 30 || value[len(value)-1] != 'Z' ||
		value[:19] != stamp.Format(usageFactSecondLayout) || len(value) > 20 && value[19] != '.' {
		return time.Time{}, errInvalidUsageFact
	}
	return stamp, nil
}

func (s *Service) ListUsageAttemptFacts(ctx context.Context, actor Actor, project, id string, limit int, before string) (page UsageAttemptFactPage, err error) {
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(id, false) || !handoffID(before, true) || limit < 1 || limit > 100 {
		return page, invalid(usageInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectRead)
	if err != nil {
		return page, err
	}
	defer close()

	tenant := auth.target.tenantID
	var storedID, rawAttempt string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,payload FROM suma_usage_attempts WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&storedID, &rawAttempt)
	if errors.Is(err, sql.ErrNoRows) || err == nil && storedID != id {
		return page, missing()
	}
	if err != nil {
		return page, err
	}
	var source struct {
		Attempt *struct {
			SourceResultSHA256 string `json:"source_result_sha256"`
		} `json:"attempt"`
	}
	if json.Unmarshal([]byte(rawAttempt), &source) != nil || source.Attempt == nil ||
		source.Attempt.SourceResultSHA256 != "" && !toolcontract.ValidDigest(source.Attempt.SourceResultSHA256) {
		return page, errInvalidUsageFact
	}

	const scope = ` FROM suma_usage_facts WHERE tenant_id=? AND project_id=? AND attempt_id=?`
	// RFC3339Nano can omit trailing zeros and the entire fraction. Pad the
	// fraction to nine digits so equivalent UTC instants share the same key.
	const sortKey = `SUBSTR(REPLACE(REPLACE(created_at,'.',''),'Z','000000000'),1,28)`
	pageQuery := `SELECT id,` + sortKey + ` AS sort_key` + scope
	args := []any{tenant, project, id}
	if before != "" {
		var cursorID, rawStamp string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,created_at`+scope+` AND id=?`), tenant, project, id, before).Scan(&cursorID, &rawStamp)
		if errors.Is(err, sql.ErrNoRows) || err == nil && cursorID != before {
			return page, invalid(usageInvalid)
		}
		if err != nil {
			return page, err
		}
		stamp, err := usageFactStamp(rawStamp)
		if err != nil {
			return page, err
		}
		// The raw upper bound keeps later seconds outside the indexed range.
		// The padded key and ID then locate the exact boundary within it.
		upper := stamp.Add(time.Second).Format(usageFactSecondLayout)
		key := stamp.Format(usageFactSecondLayout) + fmt.Sprintf("%09d", stamp.Nanosecond())
		pageQuery += ` AND created_at<? AND (` + sortKey + `<? OR (` + sortKey + `=? AND id<?))`
		args = append(args, upper, key, key, before)
	}
	// The index covers candidate keys. Read private payloads only for the
	// bounded page selected by the normalized ordering.
	query := `SELECT f.id,f.kind,f.created_at,f.identity_sha256,f.payload FROM (` + pageQuery +
		` ORDER BY sort_key DESC,id DESC LIMIT ?) fact_page JOIN suma_usage_facts f ON f.id=fact_page.id` +
		` ORDER BY fact_page.sort_key DESC,fact_page.id DESC`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), append(args, limit+1)...)
	if err != nil {
		return page, err
	}
	defer rows.Close()

	items := make([]UsageAttemptFact, 0, limit+1)
	for rows.Next() {
		var fact UsageAttemptFact
		var raw string
		if err := rows.Scan(&fact.ID, &fact.Kind, &fact.CreatedAt, &fact.IdentitySHA256, &raw); err != nil {
			return page, err
		}
		if err := fact.projectUsage(raw, source.Attempt.SourceResultSHA256); err != nil {
			return page, err
		}
		items = append(items, fact)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	page = UsageAttemptFactPage{ProjectID: project, AttemptID: id, Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = items[limit-1].ID
	}
	return page, nil
}

func (f *UsageAttemptFact) projectUsage(raw, originalResultSHA256 string) error {
	if _, err := usageFactStamp(f.CreatedAt); err != nil ||
		!handoffID(f.ID, false) || !toolcontract.ValidDigest(f.IdentitySHA256) {
		return errInvalidUsageFact
	}
	// Decode only the fields this endpoint can publish. Usage's JSON decoder
	// checks the safe integer range and preserves an explicit measured zero.
	type usageEvidence struct {
		Usage      *toolcontract.Usage `json:"usage"`
		UsageState string              `json:"usage_state"`
	}
	switch f.Kind {
	case "result":
		var result *struct {
			ToolEvidence *usageEvidence `json:"tool_evidence"`
		}
		if json.Unmarshal([]byte(raw), &result) != nil || result == nil {
			return errInvalidUsageFact
		}
		f.OriginalResult = f.IdentitySHA256 == originalResultSHA256
		f.UsageState = toolcontract.UsageUnreported
		if result.ToolEvidence != nil {
			f.Usage, f.UsageState = result.ToolEvidence.Usage, result.ToolEvidence.UsageState
		}
	case "confirmation":
		var confirmation struct {
			Usage *toolcontract.Usage `json:"usage"`
		}
		if json.Unmarshal([]byte(raw), &confirmation) != nil {
			return errInvalidUsageFact
		}
		f.Usage, f.UsageState = confirmation.Usage, toolcontract.UsageComplete
	case "resolution":
		var resolution *struct{}
		if json.Unmarshal([]byte(raw), &resolution) != nil || resolution == nil {
			return errInvalidUsageFact
		}
		return nil
	default:
		return errInvalidUsageFact
	}
	switch f.UsageState {
	case toolcontract.UsageComplete:
		if f.Usage != nil && f.Usage.InputTokens != nil && f.Usage.OutputTokens != nil {
			return nil
		}
	case toolcontract.UsagePartial:
		if f.Usage != nil {
			return nil
		}
	case toolcontract.UsageUnreported:
		if f.Usage == nil {
			return nil
		}
	}
	return errInvalidUsageFact
}
