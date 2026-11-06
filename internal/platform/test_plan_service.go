package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
)

func testPlanActor(a Actor) bool { return a.kind == actorBrowser || a.kind == actorProjectAPI }
func (s *Service) testPlanViewTx(ctx context.Context, auth *authorizedCommand, project string, current testPlanAggregate) (out TestPlanView, err error) {
	out = TestPlanView{ID: current.ID, ProjectID: project, TaskID: current.TaskID, Version: current.Version, State: current.State, LatestRevision: current.Latest, PublishedRevision: current.Published, AllowedActions: []string{}, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, Eligibility: "none"}
	latest, e := s.testPlanRevisionTx(ctx, auth.tx, auth.target.tenantID, project, current.ID, current.Latest)
	if e != nil {
		return out, e
	}
	out.Latest = &latest
	out.HasDraft = latest.State == "draft"
	out.BasisCurrent, e = s.testPlanSourceCurrentTx(ctx, auth.tx, auth.target.tenantID, project, latest.Source)
	if e != nil {
		return out, e
	}
	if current.Published > 0 {
		published, e := s.testPlanRevisionTx(ctx, auth.tx, auth.target.tenantID, project, current.ID, current.Published)
		if e != nil {
			return out, e
		}
		out.Published = &published
		valid, e := s.testPlanSourceCurrentTx(ctx, auth.tx, auth.target.tenantID, project, published.Source)
		if e != nil {
			return out, e
		}
		out.Eligibility = "source_changed"
		if valid {
			out.Eligibility = "current"
		}
	} else if current.State == "retired" {
		out.Eligibility = "retired"
	}
	if _, e = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false); e == nil {
		out.AllowedActions = append(out.AllowedActions, "save")
	} else {
		var api *APIError
		if !errors.As(e, &api) {
			return out, e
		}
	}
	if auth.actor.kind == actorBrowser {
		if _, e = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, true); e == nil {
			if out.HasDraft && out.BasisCurrent && validateTestPlanDraft(latest.Title, latest.Description, latest.Cases, latest.Source, true) == nil {
				out.AllowedActions = append(out.AllowedActions, "publish")
			}
			if current.Published > 0 {
				out.AllowedActions = append(out.AllowedActions, "retire")
			}
		} else {
			var api *APIError
			if !errors.As(e, &api) {
				return out, e
			}
		}
	}
	return out, nil
}

// Every mutation shares the same current authority, project guard and receipt
// transaction as task dispatch and formal Review. No external action occurs.
func (s *Service) testPlanCommand(ctx context.Context, actor Actor, project, id, key string, purpose commandPurpose, request any, change func(context.Context, *authorizedCommand) (TestPlanView, error)) (out TestPlanView, err error) {
	if !testPlanActor(actor) || !handoffID(project, false) || !handoffID(id, true) || !planningKey(key) {
		if !testPlanActor(actor) {
			return out, forbidden()
		}
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purpose)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, key, request)
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		current, e := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, out.ID)
		if e != nil {
			return out, e
		}
		if e = s.testPlanReceiptEligibilityTx(ctx, auth, project, current, &out); e != nil {
			return TestPlanView{}, e
		}
		return out, nil
	}
	out, err = change(ctx, auth)
	if err != nil {
		return TestPlanView{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, key, hash, out); err != nil {
		return TestPlanView{}, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) CreateTestPlan(ctx context.Context, actor Actor, project string, request TestPlanCreateRequest) (TestPlanView, error) {
	if request.ProjectID != project || !handoffID(request.TaskID, false) || !designVersion(request.ExpectedTaskVersion) || request.ExpectedTaskVersion < 1 || !designVersion(request.ExpectedRequirementVersion) {
		return TestPlanView{}, invalid(testPlanInvalid)
	}
	return s.testPlanCommand(ctx, actor, project, "", request.IdempotencyKey, purposeProjectWrite, map[string]any{"test_plan_create": request}, func(ctx context.Context, auth *authorizedCommand) (TestPlanView, error) {
		source, err := s.testPlanSourceTx(ctx, auth.tx, auth.target.tenantID, project, request.TaskID)
		if err != nil {
			return TestPlanView{}, err
		}
		if !testPlanExpectedSource(source, request.ExpectedTaskVersion, request.ExpectedRequirementVersion) {
			return TestPlanView{}, conflict(testPlanVersion)
		}
		if err = validateTestPlanDraft(request.Title, request.Description, request.Cases, source, false); err != nil {
			return TestPlanView{}, err
		}
		if _, found, err := s.testPlanByTaskTx(ctx, auth.tx, auth.target.tenantID, project, request.TaskID); err != nil {
			return TestPlanView{}, err
		} else if found {
			return TestPlanView{}, conflict(testPlanVersion)
		}
		stamp := now()
		current := testPlanAggregate{ID: newID(), TaskID: request.TaskID, Version: 1, Latest: 1, State: "draft", CreatedAt: stamp, UpdatedAt: stamp}
		revision := TestPlanRevision{Revision: 1, State: "draft", Title: request.Title, Description: request.Description, Source: source, Cases: request.Cases, CreatedBy: auth.currentPrincipal.ID, CreatedAt: stamp}
		revision.SHA256 = testPlanRevisionHash(revision)
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plans(id,tenant_id,project_id,task_id,version,state,latest_revision,published_revision,governance_revision,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), current.ID, auth.target.tenantID, project, current.TaskID, 1, "draft", 1, 0, 0, stamp, stamp); err != nil {
			return TestPlanView{}, err
		}
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plan_revisions(tenant_id,project_id,plan_id,revision,snapshot)VALUES(?,?,?,?,?)`), auth.target.tenantID, project, current.ID, 1, stringify(revision)); err != nil {
			return TestPlanView{}, err
		}
		if err = s.commandActivity(ctx, auth, "test_plan.created", current.ID, request.Title); err != nil {
			return TestPlanView{}, err
		}
		return s.testPlanViewTx(ctx, auth, project, current)
	})
}
func testPlanExpectedSource(source TestPlanSource, task, requirement int) bool {
	version := 0
	if source.Requirement != nil {
		version = source.Requirement.Version
	}
	return source.Task.Version == task && version == requirement
}
func (s *Service) SaveTestPlan(ctx context.Context, actor Actor, project, id string, request TestPlanSaveRequest) (TestPlanView, error) {
	if request.ProjectID != project || !handoffID(id, false) || !designVersion(request.ExpectedVersion) || request.ExpectedVersion < 1 || !designVersion(request.ExpectedTaskVersion) || request.ExpectedTaskVersion < 1 || !designVersion(request.ExpectedRequirementVersion) {
		return TestPlanView{}, invalid(testPlanInvalid)
	}
	return s.testPlanCommand(ctx, actor, project, id, request.IdempotencyKey, purposeProjectWrite, map[string]any{"test_plan_save": id, "request": request}, func(ctx context.Context, auth *authorizedCommand) (TestPlanView, error) {
		current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if err != nil {
			return TestPlanView{}, err
		}
		if current.Version != request.ExpectedVersion {
			return TestPlanView{}, conflict(testPlanVersion)
		}
		source, err := s.testPlanSourceTx(ctx, auth.tx, auth.target.tenantID, project, current.TaskID)
		if err != nil {
			return TestPlanView{}, err
		}
		if !testPlanExpectedSource(source, request.ExpectedTaskVersion, request.ExpectedRequirementVersion) {
			return TestPlanView{}, conflict(testPlanVersion)
		}
		if err = validateTestPlanDraft(request.Title, request.Description, request.Cases, source, false); err != nil {
			return TestPlanView{}, err
		}
		revision := TestPlanRevision{Revision: current.Latest + 1, State: "draft", Title: request.Title, Description: request.Description, Source: source, Cases: request.Cases, CreatedBy: auth.currentPrincipal.ID, CreatedAt: now()}
		state := "draft"
		if current.Published > 0 {
			state = "published"
		}
		current, err = s.saveTestPlanRevisionTx(ctx, auth, project, current, revision, state, current.Published)
		if err != nil {
			return TestPlanView{}, err
		}
		if err = s.commandActivity(ctx, auth, "test_plan.saved", id, request.Title); err != nil {
			return TestPlanView{}, err
		}
		return s.testPlanViewTx(ctx, auth, project, current)
	})
}
func (s *Service) CommandTestPlan(ctx context.Context, actor Actor, project, id string, request TestPlanCommandRequest) (TestPlanView, error) {
	if request.ProjectID != project || !handoffID(id, false) || !designVersion(request.ExpectedVersion) || request.ExpectedVersion < 1 || request.Action != "publish" && request.Action != "retire" || !testPlanText(request.Reason, 2000, true) {
		return TestPlanView{}, invalid(testPlanInvalid)
	}
	return s.testPlanCommand(ctx, actor, project, id, request.IdempotencyKey, purposeBrowserProjectApprove, map[string]any{"test_plan_command": id, "request": request}, func(ctx context.Context, auth *authorizedCommand) (TestPlanView, error) {
		current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if err != nil {
			return TestPlanView{}, err
		}
		if current.Version != request.ExpectedVersion {
			return TestPlanView{}, conflict(testPlanVersion)
		}
		revision, err := s.testPlanRevisionTx(ctx, auth.tx, auth.target.tenantID, project, id, current.Latest)
		if err != nil {
			return TestPlanView{}, err
		}
		state, published := "retired", 0
		if request.Action == "publish" {
			if revision.State != "draft" {
				return TestPlanView{}, invalid(testPlanIncomplete)
			}
			valid, e := s.testPlanSourceCurrentTx(ctx, auth.tx, auth.target.tenantID, project, revision.Source)
			if e != nil {
				return TestPlanView{}, e
			}
			if !valid {
				return TestPlanView{}, conflict(testPlanSource)
			}
			if e = validateTestPlanDraft(revision.Title, revision.Description, revision.Cases, revision.Source, true); e != nil {
				return TestPlanView{}, e
			}
			state, published = "published", current.Latest+1
		} else if current.Published == 0 {
			return TestPlanView{}, invalid(testPlanInvalid)
		}
		revision.Revision = current.Latest + 1
		revision.State = state
		revision.CreatedAt = now()
		revision.CreatedBy = auth.currentPrincipal.ID
		// Only reviewed governance changes advance the policy epoch. Draft
		// saves preserve it, including when no plan is currently published.
		current.Governance = revision.Revision
		if err = s.InvalidateDependentsTx(ctx, auth.tx, auth, project, []string{current.TaskID}, true); err != nil {
			return TestPlanView{}, err
		}
		current, err = s.saveTestPlanRevisionTx(ctx, auth, project, current, revision, state, published)
		if err != nil {
			return TestPlanView{}, err
		}
		if err = s.commandActivity(ctx, auth, "test_plan."+request.Action, id, request.Reason); err != nil {
			return TestPlanView{}, err
		}
		return s.testPlanViewTx(ctx, auth, project, current)
	})
}
func (s *Service) GetTestPlan(ctx context.Context, actor Actor, project, id string) (out TestPlanView, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.testPlanViewTx(ctx, auth, project, current)
}

const testPlanStateCursorLimit = 1024

type testPlanListCursor struct{ Tenant, Project, Task, State, Created, ID string }

func encodeTestPlanListCursor(cursor testPlanListCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(stringify(cursor)))
}

func (s *Service) ListTestPlans(ctx context.Context, actor Actor, project, task, state string, limit int, before string) (out TestPlanList, err error) {
	out.Items = []TestPlanView{}
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	switch state {
	case "", "draft", "published", "retired":
	default:
		return out, invalid(testPlanInvalid)
	}
	if !handoffID(project, false) || !handoffID(task, true) || limit < 1 || limit > 100 ||
		state == "" && !handoffID(before, true) || state != "" && len(before) > testPlanStateCursorLimit {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	tenant := auth.target.tenantID
	from := ` FROM suma_test_plans WHERE tenant_id=? AND project_id=?`
	args := []any{tenant, project}
	if task != "" {
		// A task-scoped selector proves its current project relation before
		// exposing its plan. It never scans another task's client-side page.
		if _, err = s.getRecordTx(ctx, auth, project, "tasks", task); err != nil {
			return out, err
		}
		from += ` AND task_id=?`
		args = append(args, task)
	}
	var cursor testPlanListCursor
	if state != "" {
		from += ` AND state=?`
		args = append(args, state)
	}
	if before != "" {
		if state == "" {
			anchor, e := s.testPlanTx(ctx, auth.tx, tenant, project, before)
			if e != nil {
				return out, e
			}
			if task != "" && anchor.TaskID != task {
				return out, invalid(testPlanInvalid)
			}
			cursor.Created, cursor.ID = anchor.CreatedAt, anchor.ID
		} else {
			// A plan can leave the filtered state between requests. Use the
			// original sort position without consulting the plan's new state.
			raw, e := base64.RawURLEncoding.Strict().DecodeString(before)
			if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Tenant != tenant ||
				cursor.Project != project || cursor.Task != task || cursor.State != state ||
				!validReleaseTime(cursor.Created) || !handoffID(cursor.ID, false) || encodeTestPlanListCursor(cursor) != before {
				return out, invalid(testPlanInvalid)
			}
		}
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+from), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT id` + from
	if before != "" {
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, cursor.Created, cursor.Created, cursor.ID)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, e := auth.tx.QueryContext(ctx, s.q(query), args...)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	for _, id := range ids {
		current, e := s.testPlanTx(ctx, auth.tx, tenant, project, id)
		if e != nil {
			return out, e
		}
		view, e := s.testPlanViewTx(ctx, auth, project, current)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, view)
	}
	if more {
		last := out.Items[len(out.Items)-1]
		out.NextCursor = last.ID
		if state != "" {
			out.NextCursor = encodeTestPlanListCursor(testPlanListCursor{Tenant: tenant, Project: project, Task: task, State: state, Created: last.CreatedAt, ID: last.ID})
		}
	}
	return out, nil
}

// Dispatch uses the protected current publication, never task.Data or a caller
// supplied revision. A draft cannot silently replace the fixed test contract.
func (s *Service) taskTestPlanContextTx(ctx context.Context, tx *sql.Tx, tenant, project, task string) (*TestPlanRevision, error) {
	current, found, err := s.testPlanByTaskTx(ctx, tx, tenant, project, task)
	if err != nil || !found {
		return nil, err
	}
	if current.Published == 0 {
		return nil, nil
	}
	revision, err := s.testPlanRevisionTx(ctx, tx, tenant, project, current.ID, current.Published)
	if err != nil {
		return nil, err
	}
	valid, err := s.testPlanSourceCurrentTx(ctx, tx, tenant, project, revision.Source)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, conflict(testPlanSource)
	}
	return &revision, nil
}

// Recovery returns the original operation's content and version, with freshly
// derived eligibility. Task/requirement changes do not necessarily increment
// the plan, so checking only plan.Version would leave stale approval controls.
func (s *Service) testPlanReceiptEligibilityTx(ctx context.Context, auth *authorizedCommand, project string, current testPlanAggregate, out *TestPlanView) error {
	if out.Latest == nil {
		return conflict(testPlanVersion)
	}
	fresh, err := s.testPlanViewTx(ctx, auth, project, current)
	if err != nil {
		return err
	}
	out.BasisCurrent, err = s.testPlanSourceCurrentTx(ctx, auth.tx, auth.target.tenantID, project, out.Latest.Source)
	if err != nil {
		return err
	}
	out.AllowedActions = []string{}
	if current.Version == out.Version {
		out.AllowedActions = append(out.AllowedActions, fresh.AllowedActions...)
	}
	out.Eligibility = "none"
	if out.Published != nil {
		switch {
		case current.Published == 0:
			out.Eligibility = "retired"
		case fresh.Published == nil || current.Published != out.PublishedRevision || fresh.Published.SHA256 != out.Published.SHA256:
			out.Eligibility = "source_changed"
		default:
			valid, e := s.testPlanSourceCurrentTx(ctx, auth.tx, auth.target.tenantID, project, out.Published.Source)
			if e != nil {
				return e
			}
			out.Eligibility = "source_changed"
			if valid {
				out.Eligibility = "current"
			}
		}
	} else if out.State == "retired" {
		out.Eligibility = "retired"
	}
	return nil
}
