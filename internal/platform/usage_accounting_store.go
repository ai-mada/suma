package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/toolcontract"
)

const (
	usageInvalid             = "Invalid usage accounting request"
	usageVersion             = "Usage accounting has changed; reload before deciding"
	usageRateProfile         = "Pricing requires a current enabled fixed Suma model configuration"
	usageConfirmRequired     = "Confirm the usage and provide its evidence source before settling"
	usageCannotReduce        = "Confirmed usage cannot reduce an already reported counter"
	usageEnforceConfirmation = "Confirm that enforcement can prevent new tool attempts"
	usageUnavailable         = "Model token or cost allowance is unavailable"
	usageOutputReservation   = "The output token reservation must cover this attempt's output budget"
)
const usageSafe = toolcontract.MaxSafeUsage

func usageFlag(value bool) int {
	if value {
		return 1
	}
	return 0
}

type usageRecord struct {
	Attempt                   UsageAttempt `json:"attempt"`
	Period                    string       `json:"period"`
	OrganizationPolicyVersion int          `json:"organization_policy_version"`
	ProjectPolicyVersion      int          `json:"project_policy_version"`
	ChargedTokens             string       `json:"charged_tokens"`
	ChargedCost               string       `json:"charged_cost"`
	HeldTokens                string       `json:"held_tokens"`
	HeldCost                  string       `json:"held_cost"`
	Unresolved                bool         `json:"unresolved"`
}
type usageAccount struct {
	scope, project, period, resource string
	spent, reserved                  *big.Int
	unresolved, version              int
}

func (s *Service) MigrateUsageAccounting(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS suma_usage_policies(tenant_id VARCHAR(64) NOT NULL,scope VARCHAR(20) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(tenant_id,scope,project_id))`,
		`CREATE TABLE IF NOT EXISTS suma_usage_rates(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,tool_profile_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,tool_profile_id,version))`,
		`CREATE TABLE IF NOT EXISTS suma_usage_account_guards(tenant_id VARCHAR(64) PRIMARY KEY,version INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_usage_accounts(tenant_id VARCHAR(64) NOT NULL,scope VARCHAR(20) NOT NULL,project_id VARCHAR(64) NOT NULL,period VARCHAR(7) NOT NULL,resource VARCHAR(12) NOT NULL,version INTEGER NOT NULL,spent TEXT NOT NULL,reserved TEXT NOT NULL,unresolved INTEGER NOT NULL,PRIMARY KEY(tenant_id,scope,project_id,period,resource))`,
		`CREATE TABLE IF NOT EXISTS suma_usage_attempts(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,period VARCHAR(7) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,unresolved INTEGER NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(run_id,node_id,attempt,fence))`,
		`CREATE TABLE IF NOT EXISTS suma_usage_facts(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,attempt_id VARCHAR(64) NOT NULL,kind VARCHAR(20) NOT NULL,identity_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(attempt_id,kind,identity_sha256))`,
	} {
		if _, e := s.DB.ExecContext(ctx, s.ddl(q)); e != nil {
			return e
		}
	}
	for _, index := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_usage_attempt_page", "suma_usage_attempts", []string{"tenant_id", "project_id", "period", "created_at", "id"}},
		{"suma_usage_attempt_state", "suma_usage_attempts", []string{"tenant_id", "project_id", "period", "state", "created_at", "id"}},
		{"suma_usage_rate_current", "suma_usage_rates", []string{"tenant_id", "project_id", "tool_profile_id", "version"}},
		{"suma_usage_fact_scope", "suma_usage_facts", []string{"tenant_id", "project_id", "attempt_id", "created_at", "id"}},
	} {
		if e := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, index.table, index.columns...); e != nil {
			return e
		}
	}
	return nil
}
func usageMonth(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	stamp, e := time.Parse("2006-01", value)
	if e != nil || stamp.Format("2006-01") != value {
		return "", invalid(usageInvalid)
	}
	return value, nil
}
func usageCurrency(c string) bool { return c == "CNY" || c == "USD" }
func usageInteger(n int64) bool   { return n >= 0 && n <= usageSafe }
func usageLimit(n *int64) bool    { return n == nil || usageInteger(*n) }
func usageInt(n int64) *int64     { return &n }
func usageBig(raw string) *big.Int {
	v, ok := new(big.Int).SetString(raw, 10)
	if !ok || v.Sign() < 0 {
		return new(big.Int)
	}
	return v
}
func usagePublic(n *big.Int) *int64 {
	if !n.IsInt64() || n.Sign() < 0 || n.Cmp(big.NewInt(usageSafe)) > 0 {
		return nil
	}
	return usageInt(n.Int64())
}
func usageCost(input, output int64, rate UsageRateRevision) *big.Int {
	total := new(big.Int).Mul(big.NewInt(input), big.NewInt(rate.InputRateMicrosPerMillion))
	total.Add(total, new(big.Int).Mul(big.NewInt(output), big.NewInt(rate.OutputRateMicrosPerMillion)))
	total.Add(total, big.NewInt(999999))
	return total.Quo(total, big.NewInt(1000000))
}
func usageID(ref dag.AccountingAttempt) string {
	return digest(stringify([]any{ref.TenantID, ref.ProjectID, ref.RunID, ref.NodeID, ref.Attempt, ref.Fence}))[:32]
}

// This row serializes both monthly scopes in one tenant. It follows the current
// runner's organization authority and existing scheduler guards, never reacquires
// human authority and uses the same order for claims, results and human writes.
func (s *Service) usageLockTx(ctx context.Context, tx *sql.Tx, tenant string) error {
	var version int
	e := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version FROM suma_usage_account_guards WHERE tenant_id=?`)), tenant).Scan(&version)
	if errors.Is(e, sql.ErrNoRows) {
		_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_usage_account_guards(tenant_id,version)VALUES(?,1)`), tenant)
	}
	return e
}
func (s *Service) usagePolicyTx(ctx context.Context, tx *sql.Tx, tenant, project, scope string) (out UsagePolicy, err error) {
	if scope == "organization" {
		project = ""
	}
	out = UsagePolicy{ID: digest(tenant + ":" + scope + ":" + project)[:32], Scope: scope, ProjectID: project, Mode: "observe", Currency: "CNY", AllowedActions: []string{}}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_usage_policies WHERE tenant_id=? AND scope=? AND project_id=?`), tenant, scope, project).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	out.AllowedActions = []string{}
	return
}
func (s *Service) usageRateTx(ctx context.Context, tx *sql.Tx, tenant, project, profile string) (out *UsageRateRevision, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_usage_rates WHERE tenant_id=? AND project_id=? AND tool_profile_id=? ORDER BY version DESC LIMIT 1`), tenant, project, profile).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out = &UsageRateRevision{}
	err = json.Unmarshal([]byte(raw), out)
	return
}
func (s *Service) usageRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out usageRecord, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_usage_attempts WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	return
}
func (s *Service) saveUsageTx(ctx context.Context, tx *sql.Tx, tenant string, r usageRecord, old int) error {
	a := r.Attempt
	result, e := tx.ExecContext(ctx, s.q(`UPDATE suma_usage_attempts SET version=?,state=?,unresolved=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, usageFlag(r.Unresolved), stringify(r), tenant, a.ProjectID, a.ID, old)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict(usageVersion)
	}
	return nil
}
func (s *Service) usageAccountTx(ctx context.Context, tx *sql.Tx, tenant, scope, project, period, resource string, create bool) (a usageAccount, err error) {
	a = usageAccount{scope: scope, project: project, period: period, resource: resource, spent: new(big.Int), reserved: new(big.Int)}
	var spent, reserved string
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,spent,reserved,unresolved FROM suma_usage_accounts WHERE tenant_id=? AND scope=? AND project_id=? AND period=? AND resource=?`), tenant, scope, project, period, resource).Scan(&a.version, &spent, &reserved, &a.unresolved)
	if errors.Is(err, sql.ErrNoRows) {
		if !create {
			return a, nil
		}
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_usage_accounts(tenant_id,scope,project_id,period,resource,version,spent,reserved,unresolved)VALUES(?,?,?,?,?,1,'0','0',0)`), tenant, scope, project, period, resource)
		a.version = 1
		return a, err
	}
	if err != nil {
		return a, err
	}
	a.spent = usageBig(spent)
	a.reserved = usageBig(reserved)
	return a, nil
}
func (s *Service) usageAccountDeltaTx(ctx context.Context, tx *sql.Tx, tenant, scope, project, period, resource string, spentDelta, heldDelta *big.Int, unresolvedDelta int) error {
	a, e := s.usageAccountTx(ctx, tx, tenant, scope, project, period, resource, true)
	if e != nil {
		return e
	}
	a.spent.Add(a.spent, spentDelta)
	a.reserved.Add(a.reserved, heldDelta)
	a.unresolved += unresolvedDelta
	if a.spent.Sign() < 0 || a.reserved.Sign() < 0 || a.unresolved < 0 {
		return conflict(usageVersion)
	}
	result, e := tx.ExecContext(ctx, s.q(`UPDATE suma_usage_accounts SET version=version+1,spent=?,reserved=?,unresolved=? WHERE tenant_id=? AND scope=? AND project_id=? AND period=? AND resource=? AND version=?`), a.spent.String(), a.reserved.String(), a.unresolved, tenant, scope, project, period, resource, a.version)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict(usageVersion)
	}
	return nil
}
