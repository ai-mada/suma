package platform

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/ai-mada/suma/internal/dag"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
)

func sortedSteeringIDs(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for id := range values {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Creator references are located before opening the authority transaction.
// After canonical organization/user/credential locks and the execution guard,
// every instruction is read again; a newly inserted creator causes retry.
func (s *Service) steeringLockCreatorsTx(ctx context.Context, tx *sql.Tx, actor Actor, r steeringSessionRecord, rows []steeringInstructionRecord) error {
	if err := s.authorityWriter(ctx, tx); err != nil {
		return err
	}
	users := []string{}
	organizations := []string{r.Assignment.TenantID}
	keys := []string{}
	if actor.credential.subjectID != "" {
		users = append(users, actor.credential.subjectID)
		keys = append(keys, credentialLockKeys(actor.credential)...)
	}
	if r.Assignment.PoolID != "" {
		var owner string
		if err := tx.QueryRowContext(ctx, s.q(`SELECT owner_tenant_id FROM suma_execution_pools WHERE id=?`), r.Assignment.PoolID).Scan(&owner); err != nil {
			return err
		}
		organizations = append(organizations, owner)
	}
	if p, err := s.toolProfileTx(ctx, tx, Principal{TenantID: r.Assignment.TenantID}, r.Session.ProjectID, r.ProfileID); err == nil && p.EnabledBy != "" {
		users = append(users, p.EnabledBy)
	} else if err != nil {
		return err
	}
	for _, row := range rows {
		users = append(users, row.Caller.SubjectID)
		keys = append(keys, credentialLockKeys(row.Caller.ref())...)
	}
	for _, user := range sortedAuthorityIDs(users) {
		orgs, err := s.authorityOrganizations(ctx, tx, user)
		if err != nil {
			return err
		}
		organizations = append(organizations, orgs...)
	}
	return s.lockHumanAuthorities(ctx, tx, users, organizations, keys)
}
func steeringCreatorKnown(rows []steeringInstructionRecord, r steeringInstructionRecord) bool {
	for _, known := range rows {
		if known.Caller == r.Caller {
			return true
		}
	}
	return false
}

func (s *Service) PollSteering(ctx context.Context, actor Actor, project string, a dag.Assignment, id string) (sc.SteeringPoll, error) {
	return s.pollSteering(ctx, actor, "", project, a, id)
}
func (s *Service) PollPoolSteering(ctx context.Context, token, project string, a dag.Assignment, id string) (sc.SteeringPoll, error) {
	return s.pollSteering(ctx, Actor{}, token, project, a, id)
}
func (s *Service) pollSteering(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id string) (out sc.SteeringPoll, err error) {
	out.Instructions = []sc.SteeringInstruction{}
	if !steeringAssignmentValid(project, a) || !handoffID(id, false) {
		return out, invalid(steeringInvalid)
	}
	located, err := s.steeringSessionTx(ctx, s.DB, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if !steeringSameAssignment(located.Session, a) {
		return out, conflict(steeringChanged)
	}
	callers, err := s.steeringInstructionsTx(ctx, s.DB, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.steeringLockCreatorsTx(ctx, tx, actor, located, callers); err != nil {
		return out, err
	}
	_, b, err := s.steeringMachineBasisTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return out, err
	}
	r, err := s.steeringSessionTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if !steeringSameAssignment(r.Session, a) || !steeringSourceMatches(r.Session, b) {
		return out, conflict(steeringChanged)
	}
	rows, err := s.steeringInstructionsTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	out.Session = r.Session
	out.Session.AllowedActions = []string{}
	if r.Session.State == "closed" {
		for _, row := range rows {
			out.Instructions = append(out.Instructions, row.Instruction)
		}
		return out, nil
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	fresh := s.steeringFreshTx(ctx, tx, a.TenantID, project, r, b, now)
	if fresh != nil {
		if !automationAuthorityFailure(fresh) {
			return out, fresh
		}
		out.Session.State = "closed"
	}
	for _, row := range rows {
		if !steeringCreatorKnown(callers, row) {
			return sc.SteeringPoll{}, conflict(steeringChanged)
		}
		if row.Instruction.Observation != nil && row.Instruction.Observation.WriteStarted {
			out.Instructions = append(out.Instructions, row.Instruction)
			continue
		}
		if fresh != nil {
			continue
		}
		if e := s.steeringInstructionFreshTx(ctx, tx, a.TenantID, project, r, b, row, now); e != nil {
			if !automationAuthorityFailure(e) {
				return out, e
			}
			continue
		}
		out.Instructions = append(out.Instructions, row.Instruction)
	}
	return out, nil
}

func (s *Service) GetTaskSteeringContext(ctx context.Context, actor Actor, project, task, message string) (out sc.SteeringContext, err error) {
	out.Targets = []sc.SteeringSession{}
	out.Instructions = []sc.SteeringInstruction{}
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(task, false) || !handoffID(message, false) {
		return out, invalid(steeringInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	t, err := s.getRecordTx(ctx, auth, project, "tasks", task)
	if err != nil {
		return out, err
	}
	out.TaskID, out.TaskVersion = t.ID, int64(t.Version)
	out.Message, _, err = s.steeringMessageTx(ctx, auth.tx, auth, project, task, message)
	if err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id FROM suma_steering_instructions WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id=? ORDER BY id LIMIT 10001`), auth.target.tenantID, project, task, message)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(ids) > 10000 {
		return out, conflict(steeringChanged)
	}
	sessionIDs := map[string]bool{}
	for _, id := range ids {
		row, e := s.steeringInstructionTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		out.Instructions = append(out.Instructions, row.Instruction)
		sessionIDs[row.Instruction.SessionID] = true
	}
	rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT id FROM suma_steering_sessions WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND state='active' ORDER BY id LIMIT 10001`), auth.target.tenantID, project, task, t.RunID)
	if err != nil {
		return out, err
	}
	count := 0
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		sessionIDs[id] = true
		count++
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if count > 10000 || len(sessionIDs) > 10000 {
		return out, conflict(steeringChanged)
	}
	_, canWrite := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false)
	if canWrite != nil && !automationAuthorityFailure(canWrite) {
		return out, canWrite
	}
	now, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	active := 0
	for _, id := range sortedSteeringIDs(sessionIDs) {
		r, e := s.steeringSessionTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		v := r.Session
		v.AllowedActions = []string{}
		b, e := s.toolPermissionBasisTx(ctx, auth.tx, auth.target.tenantID, project, r.Assignment)
		if e == nil {
			e = s.steeringFreshTx(ctx, auth.tx, auth.target.tenantID, project, r, b, now)
		}
		if e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return out, e
			}
			v.State = "closed"
		} else {
			active++
			if canWrite == nil && out.Message.AuthorID == auth.currentPrincipal.ID && t.Status != "accepted" {
				used := false
				for _, i := range out.Instructions {
					used = used || i.SessionID == v.ID
				}
				var n int
				if e = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_steering_instructions WHERE session_id=?`), v.ID).Scan(&n); e != nil {
					return out, e
				}
				if !used && n < 32 {
					v.AllowedActions = []string{"send"}
					out.CanSend = true
				}
			}
		}
		out.Targets = append(out.Targets, v)
	}
	if !out.CanSend {
		switch {
		case out.Message.AuthorID != auth.currentPrincipal.ID:
			out.Reason = "not_own_message"
		case t.Status == "accepted":
			out.Reason = "task_accepted"
		case active == 0:
			out.Reason = "no_live_turn"
		default:
			out.Reason = "current_execution_unavailable"
		}
	}
	return out, nil
}
