package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type commandTarget struct{ tenantID, projectID, resourceID, subjectID string }
type commandPurpose uint16

const (
	purposeInvalid commandPurpose = iota
	purposeOrganizationRead
	purposeOrganizationWrite
	purposeProjectRead
	purposeProjectWrite
	purposeProjectApprove
	purposeBrowserProjectRead
	purposeBrowserProjectWrite
	purposeBrowserProjectApprove
	purposeRunnerRead
	purposeRunnerWrite
	purposeAgentRead
	purposeSwitchOrganization
	purposeIssueCredential
	purposeAccountPreferences
	purposeOrganizationPreferences
	purposeAutomationEvaluate
	purposeAutomationMaterialize
	purposeAutomationAdmit
	purposeToolAdmission
	purposeIngressDelivery
)

type authorizedCommand struct {
	tx               *sql.Tx
	actor            Actor
	target           commandTarget
	purpose          commandPurpose
	currentPrincipal Principal
	checkedAt        time.Time
	deadline         time.Time
	activationDenied bool            // evaluation-only: cannot authorize a new execution
	ctx              context.Context // immutable first transaction budget; no raw secret/context auth flag
}

func (a *authorizedCommand) assertTx(tx *sql.Tx, target commandTarget, purpose commandPurpose) error {
	if a == nil || a.tx == nil || a.tx != tx || a.target != target || a.purpose != purpose || a.actor.kind == actorInvalid {
		return forbidden()
	}
	if !time.Now().Before(a.deadline) {
		tx.Rollback()
		return context.DeadlineExceeded
	}
	if a.ctx != nil && a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	return nil
}

func purposePolicy(p commandPurpose) (browserOnly, write, approve bool, valid bool) {
	switch p {
	case purposeOrganizationRead, purposeOrganizationWrite, purposeSwitchOrganization, purposeAccountPreferences, purposeOrganizationPreferences:
		return true, false, false, true
	case purposeProjectRead:
		return false, false, false, true
	case purposeProjectWrite:
		return false, true, false, true
	case purposeProjectApprove:
		return false, true, true, true
	case purposeBrowserProjectRead:
		return true, false, false, true
	case purposeBrowserProjectWrite, purposeIssueCredential:
		return true, true, false, true
	case purposeBrowserProjectApprove:
		return true, true, true, true
	case purposeRunnerRead, purposeAgentRead:
		return false, false, false, true
	case purposeRunnerWrite:
		return false, true, false, true
	}
	return false, false, false, false
}

func (s *Service) authorizeTx(ctx context.Context, tx *sql.Tx, actor Actor, target commandTarget, purpose commandPurpose) (*authorizedCommand, error) {
	return s.authorizeTxWithParticipants(ctx, tx, actor, target, purpose, nil)
}

// Additional organization IDs coordinate persisted bilateral references. They
// only enlarge the lock set; current authority still comes from Actor/target.
// Callers resolve the real references before entry and recheck them under locks.
func (s *Service) authorizeTxWithParticipants(ctx context.Context, tx *sql.Tx, actor Actor, target commandTarget, purpose commandPurpose, organizations []string, subjects ...string) (*authorizedCommand, error) {
	if tx == nil || actor.kind == actorInvalid {
		return nil, unauthenticated()
	}
	if actor.kind == actorDurableAutomation || actor.kind == actorDurableToolActivation || actor.kind == actorSignedIngress {
		if len(subjects) > 0 {
			return nil, forbidden()
		}
		return s.authorizeDelegationTx(ctx, tx, actor, target, purpose, organizations)
	}
	if actor.credential.digest == "" || actor.credential.subjectID == "" {
		return nil, unauthenticated()
	}
	browserOnly, write, approve, valid := purposePolicy(purpose)
	if !valid {
		return nil, forbidden()
	}
	deadline := s.txAuthorityBudget(ctx, tx).deadline
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := s.authorityWriter(bounded, tx); err != nil {
		return nil, err
	}
	if target.tenantID == "" || target.tenantID != actor.credential.tenantID {
		return nil, forbidden()
	}
	users := sortedAuthorityIDs(append([]string{actor.credential.subjectID, target.subjectID}, subjects...))
	orgs := append([]string{target.tenantID}, organizations...)
	for _, user := range users {
		candidates, e := s.authorityOrganizations(bounded, tx, user)
		if e != nil {
			return nil, e
		}
		orgs = append(orgs, candidates...)
	}
	if err := s.lockHumanAuthorities(bounded, tx, users, orgs, credentialLockKeys(actor.credential)); err != nil {
		return nil, err
	}
	c, err := s.readCredential(bounded, tx, actor.credential.digest, true)
	if err != nil {
		return nil, err
	}
	if c.kind != actor.kind || c.ref != actor.credential {
		return nil, unauthenticated()
	}
	at, err := s.authorityNow(bounded, tx)
	if err != nil {
		return nil, err
	}
	if c.expires.Unix() <= at.Unix() {
		return nil, unauthenticated()
	}
	if browserOnly && c.kind != actorBrowser {
		return nil, forbidden()
	}
	switch c.kind {
	case actorBrowser:
		if purpose == purposeRunnerRead || purpose == purposeRunnerWrite {
			return nil, forbidden()
		}
	case actorProjectAPI:
		if browserOnly || purpose == purposeRunnerRead || purpose == purposeRunnerWrite {
			return nil, forbidden()
		}
	case actorProjectRunner:
		if purpose != purposeRunnerRead && purpose != purposeRunnerWrite {
			return nil, forbidden()
		}
	case actorAgentRead:
		if purpose != purposeAgentRead {
			return nil, forbidden()
		}
	default:
		return nil, forbidden()
	}
	if c.kind != actorBrowser && (target.projectID == "" || target.projectID != c.ref.projectID) {
		return nil, forbidden()
	}
	var p Principal
	err = tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT u.id,u.name,u.email,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE u.id=? AND m.tenant_id=?`)), c.ref.subjectID, target.tenantID).Scan(&p.ID, &p.Name, &p.Email, &p.TenantID, &p.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, forbidden()
	}
	if err != nil {
		return nil, err
	}
	if target.projectID != "" && target.projectID != organizationScope {
		var tenant string
		err = tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT tenant_id FROM suma_projects WHERE id=?`)), target.projectID).Scan(&tenant)
		if errors.Is(err, sql.ErrNoRows) || tenant != target.tenantID {
			return nil, missing()
		}
		if err != nil {
			return nil, err
		}
		if !orgAdmin(p) {
			var role string
			err = tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`)), target.projectID, p.ID).Scan(&role)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, forbidden()
			}
			if err != nil {
				return nil, err
			}
			if !validProjectRole(role) || approve && role != "admin" && role != "reviewer" || write && role == "viewer" {
				return nil, forbidden()
			}
		}
	}
	return &authorizedCommand{tx: tx, actor: actor, target: target, purpose: purpose, currentPrincipal: p, checkedAt: at, deadline: deadline, ctx: s.txAuthorityBudget(ctx, tx).ctx}, nil
}

func (s *Service) commandReceipt(ctx context.Context, a *authorizedCommand, key string, body any) (string, bool, error) {
	if a == nil {
		return "", false, forbidden()
	}
	if delegationOnlyPurpose(a.purpose) {
		return "", false, forbidden()
	}
	if err := a.assertTx(a.tx, a.target, a.purpose); err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithDeadline(ctx, a.deadline)
	defer cancel()
	if len(key) < 8 || len(key) > 128 {
		return "", false, invalid("请提供 8–128 字符的 idempotency_key")
	}
	hash := digest(a.currentPrincipal.ID + stringify(body))
	var stored, response string
	err := a.tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), a.target.tenantID, a.target.projectID, key).Scan(&stored, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return hash, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if stored != hash {
		return "", false, &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
	}
	return response, true, nil
}

func (s *Service) saveCommandReceipt(ctx context.Context, a *authorizedCommand, key, hash string, response any) error {
	if a == nil {
		return forbidden()
	}
	if delegationOnlyPurpose(a.purpose) {
		return forbidden()
	}
	if err := a.assertTx(a.tx, a.target, a.purpose); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, a.deadline)
	defer cancel()
	_, err := a.tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response) VALUES(?,?,?,?,?)`), a.target.tenantID, a.target.projectID, key, hash, stringify(response))
	return err
}

func (s *Service) commandActivity(ctx context.Context, a *authorizedCommand, kind, id, title string) error {
	if a == nil {
		return forbidden()
	}
	if delegationOnlyPurpose(a.purpose) {
		return forbidden()
	}
	if err := a.assertTx(a.tx, a.target, a.purpose); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, a.deadline)
	defer cancel()
	_, err := a.tx.ExecContext(ctx, s.q(`INSERT INTO suma_activity(id,tenant_id,project_id,actor_id,kind,object_id,title,created_at) VALUES(?,?,?,?,?,?,?,?)`), newID(), a.target.tenantID, a.target.projectID, a.currentPrincipal.ID, kind, id, title, a.checkedAt.Format(time.RFC3339Nano))
	return err
}
