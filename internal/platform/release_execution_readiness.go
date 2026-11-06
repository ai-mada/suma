package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	rc "github.com/ai-mada/suma/internal/releasecontract"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const (
	releaseRunnerClaimFreshness = time.Minute
	releaseRunnerClaimRefresh   = 15 * time.Second
)

// ReleaseExecutionReadiness summarizes current project registration and claim
// observations. Dispatch and provider execution each keep their own authority.
type ReleaseExecutionReadiness struct {
	State           string `json:"state"`
	RegisteredCount int    `json:"registered_count"`
	CheckedAt       string `json:"checked_at"`
	ObservedAt      string `json:"observed_at,omitempty"`
}

// The same authenticated claim must advertise the generic scheduler
// capability and the protected release protocol for this exact provider.
func releaseProviderClaimed(provider string, capabilities []string) bool {
	return provider != wbc.Plugin && rc.ValidProfileID(provider) &&
		slices.Contains(capabilities, "external_workflow:"+provider) &&
		slices.Contains(capabilities, rc.ReleaseProviderCapabilityPrefix+provider)
}

// recordReleaseRunnerClaimTx is called only for new authenticated private
// claims. Persist one replaceable observation per deployment registration,
// with the current version to invalidate earlier evidence on rotation.
func (s *Service) recordReleaseRunnerClaimTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, caps []string) error {
	if auth == nil || auth.actor.kind != actorProjectRunner {
		return forbidden()
	}
	if err := auth.assertTx(tx, auth.target, purposeRunnerWrite); err != nil {
		return err
	}
	var id string
	var version int
	var priorVersion sql.NullInt64
	var priorAt, priorProviders sql.NullString
	err := tx.QueryRowContext(ctx, s.q(`SELECT w.id,w.version,c.worker_version,c.observed_at,c.provider_ids
		FROM suma_release_workers w LEFT JOIN suma_release_runner_claims c ON c.worker_registration_id=w.id
		WHERE w.tenant_id=? AND w.project_id=? AND w.worker_id=? AND w.token_hash=? AND w.status='active' AND w.deployment=1`),
		auth.target.tenantID, auth.target.projectID, auth.target.resourceID, auth.actor.credential.digest).
		Scan(&id, &version, &priorVersion, &priorAt, &priorProviders)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	providers := []string{}
	for _, capability := range caps {
		provider, ok := strings.CutPrefix(capability, rc.ReleaseProviderCapabilityPrefix)
		if ok && releaseProviderClaimed(provider, caps) {
			providers = append(providers, provider)
		}
	}
	slices.Sort(providers)
	encoded := stringify(slices.Compact(providers))
	stamp := auth.checkedAt.Format(time.RFC3339Nano)
	if !priorVersion.Valid {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_runner_claims(worker_registration_id,worker_version,observed_at,provider_ids) VALUES(?,?,?,?)`), id, version, stamp, encoded)
		return err
	}
	priorTime, parseErr := time.Parse(time.RFC3339Nano, priorAt.String)
	if priorVersion.Int64 == int64(version) && priorProviders.String == encoded && parseErr == nil &&
		!priorTime.After(auth.checkedAt) && auth.checkedAt.Sub(priorTime) < releaseRunnerClaimRefresh {
		return nil
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_release_runner_claims SET worker_version=?,observed_at=?,provider_ids=? WHERE worker_registration_id=?`), version, stamp, encoded, id)
	return err
}

func (s *Service) releaseExecutionReadinessTx(ctx context.Context, auth *authorizedCommand, provider string) (*ReleaseExecutionReadiness, error) {
	// Only project-scoped, current credential metadata contributes to the
	// observation. The query does not impersonate a Runner or reauthorize a
	// command. Membership is checked below using the ordinary project rules.
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT w.expires_at,src.source_kind,m.role,COALESCE(p.role,''),c.observed_at,c.provider_ids
		FROM suma_release_workers w
		JOIN suma_sessions x ON x.token_hash=w.token_hash AND x.tenant_id=w.tenant_id AND x.expires_at=w.expires_at
		JOIN suma_token_scopes t ON t.token_hash=x.token_hash AND t.kind='runner' AND t.project_id=w.project_id
		JOIN suma_credential_authority a ON a.token_hash=x.token_hash AND a.credential_kind=? AND a.family_id='' AND a.revoked=0
		JOIN suma_auth_sources src ON src.id=a.source_id AND src.user_id=x.user_id AND src.generation=a.source_generation AND src.revoked=0
			AND src.authenticated_at<>''
		JOIN suma_credential_grants g ON g.id=a.grant_id AND g.user_id=x.user_id AND g.approved_by=x.user_id
			AND g.tenant_id=w.tenant_id AND g.project_id=w.project_id AND g.credential_kind=a.credential_kind AND g.scope_kind=t.kind
			AND g.source_id=src.id AND g.source_generation=src.generation AND g.expires_at=x.expires_at AND g.revoked=0
		JOIN suma_credential_slots sl ON sl.id=a.slot_id AND sl.current_digest=x.token_hash AND sl.user_id=x.user_id
			AND sl.credential_kind=a.credential_kind AND sl.project_id=w.project_id AND sl.source_id=src.id
			AND sl.family_id=a.family_id AND sl.grant_id=g.id AND sl.expires_at=x.expires_at AND sl.revoked=0
		JOIN suma_credential_issuance i ON i.token_hash=x.token_hash AND i.slot_id=sl.id AND i.family_id=a.family_id AND i.grant_id=g.id
		JOIN suma_users u ON u.id=x.user_id
		JOIN suma_org_members m ON m.tenant_id=w.tenant_id AND m.user_id=u.id
		LEFT JOIN suma_project_members p ON p.project_id=w.project_id AND p.user_id=u.id
		LEFT JOIN suma_release_runner_claims c ON c.worker_registration_id=w.id AND c.worker_version=w.version
		WHERE w.tenant_id=? AND w.project_id=? AND w.status='active' AND w.deployment=1`), kindName(actorProjectRunner), auth.target.tenantID, auth.target.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checked := auth.checkedAt
	out := &ReleaseExecutionReadiness{CheckedAt: checked.Format(time.RFC3339Nano)}
	var unobserved bool
	var latestMatch, latestFresh, latestStale time.Time
	for rows.Next() {
		var expiresAt, sourceKind, orgRole, projectRole string
		var at, raw sql.NullString
		if err = rows.Scan(&expiresAt, &sourceKind, &orgRole, &projectRole, &at, &raw); err != nil {
			return nil, err
		}
		expiry, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
		if parseErr != nil || expiry.Unix() <= checked.Unix() || !validCredentialSourceKind(sourceKind) ||
			!orgAdmin(Principal{Role: orgRole}) && (!validProjectRole(projectRole) || projectRole == "viewer") {
			continue
		}
		out.RegisteredCount++
		observed, parseErr := time.Parse(time.RFC3339Nano, at.String)
		var providers []string
		if parseErr != nil || observed.After(checked) || json.Unmarshal([]byte(raw.String), &providers) != nil {
			unobserved = true
			continue
		}
		if checked.Sub(observed) >= releaseRunnerClaimFreshness {
			if observed.After(latestStale) {
				latestStale = observed
			}
			continue
		}
		if observed.After(latestFresh) {
			latestFresh = observed
		}
		if slices.Contains(providers, provider) && observed.After(latestMatch) {
			latestMatch = observed
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	switch {
	case out.RegisteredCount == 0:
		out.State = "not_registered"
	case !latestMatch.IsZero():
		out.State, out.ObservedAt = "capability_observed", latestMatch.Format(time.RFC3339Nano)
	case unobserved:
		out.State = "unobserved"
	case !latestStale.IsZero():
		out.State, out.ObservedAt = "stale", latestStale.Format(time.RFC3339Nano)
	default:
		out.State, out.ObservedAt = "capability_missing", latestFresh.Format(time.RFC3339Nano)
	}
	return out, nil
}
