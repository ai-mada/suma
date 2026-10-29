package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// ToolDiscovery separates report freshness from individual tool observations.
// Only fresh reports contain items; this read never approves or executes tools.
type ToolDiscovery struct {
	ProjectID  string                     `json:"project_id"`
	WorkerID   string                     `json:"worker_id"`
	State      string                     `json:"state"`
	AsOf       string                     `json:"as_of"`
	ObservedAt string                     `json:"observed_at,omitempty"`
	ExpiresAt  string                     `json:"expires_at,omitempty"`
	Items      []toolcontract.Observation `json:"items"`
}

func (s *Service) DiscoverTools(ctx context.Context, actor Actor, project, worker string) (ToolDiscovery, error) {
	if project == "" || !toolcontract.ValidCommandID(worker) {
		return ToolDiscovery{}, invalid("项目或执行节点标识无效")
	}
	var out ToolDiscovery
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		out, err = s.discoverToolsTx(ctx, actor, project, worker)
		if !authoritySetRetry(err) {
			break
		}
	}
	if err != nil {
		return ToolDiscovery{}, err
	}
	return out, nil
}

func (s *Service) discoverToolsTx(ctx context.Context, actor Actor, project, worker string) (ToolDiscovery, error) {
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ToolDiscovery{}, err
	}
	defer cleanup()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return ToolDiscovery{}, err
	}
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	poolWorker := strings.HasPrefix(strings.ToLower(worker), "pool-")
	var owner string
	if poolWorker {
		// This preliminary lookup discovers only a real coordination participant.
		// It grants no access and is rechecked under ordered organization locks.
		err = tx.QueryRowContext(ctx, s.q(`SELECT p.owner_tenant_id FROM dag_project_execution_routes d JOIN suma_execution_pools p ON p.id=d.pool_id JOIN suma_execution_pool_runners r ON r.pool_id=p.id WHERE d.tenant_id=? AND d.project_id=? AND r.worker_id=?`), target.tenantID, project, worker).Scan(&owner)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ToolDiscovery{}, err
		}
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, target, purposeBrowserProjectRead, []string{owner})
	if err != nil {
		return ToolDiscovery{}, err
	}
	var projectState string
	if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status FROM suma_projects WHERE tenant_id=? AND id=?`)), target.tenantID, project).Scan(&projectState); err != nil {
		return ToolDiscovery{}, err
	}
	if projectState != "active" {
		return ToolDiscovery{}, forbidden()
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return ToolDiscovery{}, err
	}
	if poolWorker {
		if err = s.toolDiscoveryPoolWorkerTx(ctx, tx, target.tenantID, project, worker, owner, clock); err != nil {
			return ToolDiscovery{}, err
		}
	} else {
		reason, e := s.toolWorkerReasonTx(ctx, tx, auth.currentPrincipal, project, worker, clock)
		if e != nil {
			return ToolDiscovery{}, e
		}
		if reason.source != "" {
			return ToolDiscovery{}, forbidden()
		}
	}
	out := ToolDiscovery{ProjectID: project, WorkerID: worker, State: "unreported", AsOf: automationStamp(clock), Items: []toolcontract.Observation{}}
	var observedAt, raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT observed_at,items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), target.tenantID, project, worker).Scan(&observedAt, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ToolDiscovery{}, err
	}
	if err == nil {
		out.State = "stale"
		observed, parseErr := time.Parse(time.RFC3339, observedAt)
		if parseErr == nil {
			out.ObservedAt = automationStamp(observed)
			out.ExpiresAt = automationStamp(observed.Add(60 * time.Second))
			if !observed.After(clock) && clock.Before(observed.Add(60*time.Second)) {
				out.Items, err = decodeToolDiscoveryItems(raw)
				if err != nil {
					return ToolDiscovery{}, err
				}
				out.State = "fresh"
			}
		}
	}
	if err = auth.assertTx(tx, target, purposeBrowserProjectRead); err != nil {
		return ToolDiscovery{}, err
	}
	return out, tx.Commit()
}

func (s *Service) toolDiscoveryPoolWorkerTx(ctx context.Context, tx *sql.Tx, tenant, project, worker, owner string, clock time.Time) error {
	// Same exact route/grant/identity conditions as project Runner choices.
	// Historical approved grants cannot authorize a different current route.
	var currentOwner, expires string
	err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT p.owner_tenant_id,c.expires_at FROM dag_project_execution_routes d JOIN suma_execution_pool_grants g ON g.id=d.grant_id AND g.version=d.grant_version AND g.pool_id=d.pool_id AND g.tenant_id=d.tenant_id AND g.project_id=d.project_id JOIN suma_execution_pools p ON p.id=d.pool_id JOIN suma_execution_pool_runners r ON r.pool_id=d.pool_id JOIN suma_execution_pool_credentials c ON c.runner_id=r.id AND c.version=r.credential_version WHERE d.tenant_id=? AND d.project_id=? AND r.worker_id=? AND g.status='approved' AND p.status='active' AND r.status='active' AND c.status='active'`)), tenant, project, worker).Scan(&currentOwner, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return forbidden()
	}
	if err != nil {
		return err
	}
	if owner == "" || currentOwner != owner {
		return &APIError{409, "authority_retry", "资源池授权依据已变化，请重试", nil}
	}
	until, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || until.Unix() <= clock.Unix() {
		return forbidden()
	}
	return nil
}

func decodeToolDiscoveryItems(raw string) ([]toolcontract.Observation, error) {
	// The existing Runner HTTP envelope is bounded to 2 MiB. Revalidate the
	// persisted projection without returning raw or malformed storage content.
	if len(raw) > 2<<20 {
		return nil, fmt.Errorf("stored tool inventory exceeds its read bound")
	}
	var items []toolcontract.Observation
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&items); err != nil || items == nil || len(items) > 128 {
		return nil, fmt.Errorf("stored tool inventory is invalid")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, fmt.Errorf("stored tool inventory is invalid")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if toolcontract.ValidateObservation(item) != nil || seen[item.Profile.CommandID] {
			return nil, fmt.Errorf("stored tool inventory is invalid")
		}
		seen[item.Profile.CommandID] = true
	}
	return items, nil
}
