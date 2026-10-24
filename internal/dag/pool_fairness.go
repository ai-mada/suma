package dag

import (
	"fmt"
	"sort"
)

const poolFairQuantum int64 = 1000000
const poolFairRebase int64 = 1 << 50

type poolFairState struct {
	pass, remainder, floor, epoch, last int64
	weight                              int
}
type poolFairCandidate struct {
	scope                             PoolScope
	tenant, project                   poolFairState
	tenantEffective, projectEffective int64
}

func maxPoolInt(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func (t *transaction) fairState(pool, tenant, project string) (poolFairState, error) {
	var s poolFairState
	if err := t.insertIgnore(`INSERT INTO dag_execution_pool_fairness(pool_id,tenant_id,project_id,pass,remainder,weight,floor,epoch,last_sequence)VALUES(?,?,?,0,0,1,0,0,0)`, "pool_id,tenant_id,project_id", pool, tenant, project); err != nil {
		return s, err
	}
	err := t.row(`SELECT pass,remainder,weight,floor,epoch,last_sequence FROM dag_execution_pool_fairness WHERE pool_id=? AND tenant_id=? AND project_id=?`+t.e.currentReadSuffix(), pool, tenant, project).Scan(&s.pass, &s.remainder, &s.weight, &s.floor, &s.epoch, &s.last)
	return s, err
}

// Fixed-point virtual service measures successful admissions only. Clamping to
// a monotonic parent floor prevents inactive/blocked tenants banking old credit.
// Remainders make 1:3 and other integer weights exact across repeated quanta.
func (t *transaction) rankPoolScopes(pool string, scopes []PoolScope) ([]poolFairCandidate, error) {
	root, err := t.fairState(pool, "", "")
	if err != nil {
		return nil, err
	}
	if root.floor >= poolFairRebase {
		if err = t.exec(`UPDATE dag_execution_pool_fairness SET pass=CASE WHEN pass>=? THEN pass-? ELSE 0 END,epoch=epoch+1 WHERE pool_id=? AND tenant_id<>'' AND project_id=''`, root.floor, root.floor, pool); err != nil {
			return nil, err
		}
		if err = t.exec(`UPDATE dag_execution_pool_fairness SET floor=0,epoch=epoch+1 WHERE pool_id=? AND tenant_id='' AND project_id=''`, pool); err != nil {
			return nil, err
		}
		root.floor = 0
	}
	items := []poolFairCandidate{}
	parents := map[string]poolFairState{}
	for _, scope := range scopes {
		parent, ok := parents[scope.TenantID]
		if !ok {
			parent, err = t.fairState(pool, scope.TenantID, "")
			if err != nil {
				return nil, err
			}
			if parent.floor >= poolFairRebase {
				if err = t.exec(`UPDATE dag_execution_pool_fairness SET pass=CASE WHEN pass>=? THEN pass-? ELSE 0 END,epoch=epoch+1 WHERE pool_id=? AND tenant_id=? AND project_id<>''`, parent.floor, parent.floor, pool, scope.TenantID); err != nil {
					return nil, err
				}
				if err = t.exec(`UPDATE dag_execution_pool_fairness SET floor=0,epoch=epoch+1 WHERE pool_id=? AND tenant_id=? AND project_id=''`, pool, scope.TenantID); err != nil {
					return nil, err
				}
				parent.floor = 0
				parent.epoch++
			}
			parents[scope.TenantID] = parent
		}
		child, err := t.fairState(pool, scope.TenantID, scope.ProjectID)
		if err != nil {
			return nil, err
		}
		items = append(items, poolFairCandidate{scope: scope, tenant: parent, project: child, tenantEffective: maxPoolInt(parent.pass, root.floor), projectEffective: maxPoolInt(child.pass, parent.floor)})
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.tenantEffective != b.tenantEffective {
			return a.tenantEffective < b.tenantEffective
		}
		if a.tenant.last != b.tenant.last {
			return a.tenant.last < b.tenant.last
		}
		if a.scope.TenantID != b.scope.TenantID {
			return a.scope.TenantID < b.scope.TenantID
		}
		if a.projectEffective != b.projectEffective {
			return a.projectEffective < b.projectEffective
		}
		if a.project.last != b.project.last {
			return a.project.last < b.project.last
		}
		return a.scope.ProjectID < b.scope.ProjectID
	})
	return items, nil
}
func fairAdvance(s poolFairState, effective int64, weight int) (int64, int64, error) {
	if weight < 1 || weight > 100 {
		return 0, 0, ErrInvalid
	}
	rem := s.remainder
	if s.weight != weight {
		rem = 0
	}
	delta := poolFairQuantum / int64(weight)
	rem += poolFairQuantum % int64(weight)
	delta += rem / int64(weight)
	rem %= int64(weight)
	if effective > maxSchedulingInteger-delta {
		return 0, 0, dagDiagnostic(fmt.Errorf("%w: fair epoch requires normalization", ErrConflict), "dag.pool.fair_epoch", nil)
	}
	return effective + delta, rem, nil
}
func (t *transaction) chargePoolFairness(pool string, c poolFairCandidate, tenantWeight int) error {
	root, err := t.fairState(pool, "", "")
	if err != nil {
		return err
	}
	seq := root.last + 1
	tp, tr, err := fairAdvance(c.tenant, c.tenantEffective, tenantWeight)
	if err != nil {
		return err
	}
	pp, pr, err := fairAdvance(c.project, c.projectEffective, c.scope.ProjectWeight)
	if err != nil {
		return err
	}
	project := c.project
	project.pass, project.remainder, project.weight, project.last = pp, pr, c.scope.ProjectWeight, seq
	if err = t.writeFairState(pool, c.scope.TenantID, c.scope.ProjectID, c.project, project); err != nil {
		return err
	}
	tenant := c.tenant
	tenant.pass, tenant.remainder, tenant.weight, tenant.floor, tenant.last = tp, tr, tenantWeight, maxPoolInt(c.tenant.floor, c.projectEffective), seq
	if err = t.writeFairState(pool, c.scope.TenantID, "", c.tenant, tenant); err != nil {
		return err
	}
	next := root
	next.floor, next.last = maxPoolInt(root.floor, c.tenantEffective), seq
	return t.writeFairState(pool, "", "", root, next)
}

// A missing or changed fair entity must roll back resource charges and attempt,
// even while the coarse pool guard normally serializes these updates.
func (t *transaction) writeFairState(pool, tenant, project string, old, next poolFairState) error {
	res, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_execution_pool_fairness SET pass=?,remainder=?,weight=?,floor=?,epoch=?,last_sequence=? WHERE pool_id=? AND tenant_id=? AND project_id=? AND pass=? AND remainder=? AND weight=? AND floor=? AND epoch=? AND last_sequence=?`), next.pass, next.remainder, next.weight, next.floor, next.epoch, next.last, pool, tenant, project, old.pass, old.remainder, old.weight, old.floor, old.epoch, old.last)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return dagDiagnostic(fmt.Errorf("%w: fair service entity changed", ErrConflict), "dag.pool.fair_entity", nil)
	}
	return nil
}
