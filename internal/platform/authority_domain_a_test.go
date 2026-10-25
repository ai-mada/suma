package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

// The HTTP body is consumed after ingress authentication. This deterministic
// barrier leaves a second real SQLite connection free to revoke the credential
// before the application transaction starts.
type domainAAuthorityBody struct {
	*bytes.Reader
	entered, release chan struct{}
	once             sync.Once
	ctx              context.Context
}

func (*domainAAuthorityBody) Close() error { return nil }
func (b *domainAAuthorityBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	select {
	case <-b.release:
		return b.Reader.Read(p)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

func domainACount(t *testing.T, s *Service, table string) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func domainAActor(t *testing.T, s *Service, token string) Actor {
	t.Helper()
	actor, err := s.AuthenticateActor(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return actor
}

func authorityDomainAFamily(t *testing.T, route, table string, success int, request func(*fixture) map[string]any) {
	t.Helper()
	for _, invalidation := range []string{"logout", "expiry"} {
		for _, replay := range []bool{false, true} {
			label := "fresh"
			if replay {
				label = "replay"
			}
			t.Run(invalidation+"/"+label, func(t *testing.T) {
				f := newFixture(t)
				body := request(f)
				if replay {
					f.call("POST", route, body, success)
				}
				actor, err := f.s.AuthenticateActor(context.Background(), f.cookie.Value)
				if err != nil {
					t.Fatal(err)
				}
				var sequence int
				var schema, path string
				if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &schema, &path); err != nil {
					t.Fatal(err)
				}
				other, err := store.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
				second := &Service{DB: other, Dialect: "sqlite"}
				before := [3]int{domainACount(t, f.s, table), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				raw, _ := json.Marshal(body)
				gated := &domainAAuthorityBody{Reader: bytes.NewReader(raw), entered: make(chan struct{}), release: make(chan struct{}), ctx: ctx}
				var once sync.Once
				release := func() { once.Do(func() { close(gated.release) }) }
				defer release()
				req := httptest.NewRequest("POST", "http://suma.test/api/v1"+route, gated).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Suma-Expected-User", actor.Projection().ID)
				req.Header.Set("X-Suma-Expected-Organization", actor.Projection().TenantID)
				req.AddCookie(f.cookie)
				w := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { defer close(done); f.handler.ServeHTTP(w, req) }()
				select {
				case <-gated.entered:
				case <-ctx.Done():
					t.Fatal("authenticated body barrier not reached")
				}
				if invalidation == "logout" {
					err = logoutCredentialForTest(ctx, second, f.cookie.Value)
				} else {
					// Keep the new model internally consistent so this exercises DB
					// expiry, rather than merely rejecting mismatched metadata.
					expiryTx, e := other.BeginTx(ctx, nil)
					if e != nil {
						t.Fatal(e)
					}
					for _, change := range []struct{ query, key string }{
						{`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(f.cookie.Value)},
						{`UPDATE suma_session_families SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, actor.credential.familyID},
						{`UPDATE suma_credential_slots SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, actor.credential.slotID},
					} {
						if _, e = expiryTx.ExecContext(ctx, change.query, change.key); e != nil {
							break
						}
					}
					if e != nil {
						expiryTx.Rollback()
						err = e
					} else {
						err = expiryTx.Commit()
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				_, err = second.AuthenticateActor(ctx, f.cookie.Value)
				requireAuthorityStatus(t, err, 401)
				release()
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("application command did not finish")
				}
				if w.Code != 401 {
					t.Fatalf("invalidated credential returned %d: %s", w.Code, w.Body.String())
				}
				after := [3]int{domainACount(t, f.s, table), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
				if after != before {
					t.Fatalf("denial changed business/receipt/activity: before=%v after=%v", before, after)
				}
			})
		}
	}
}

func TestAuthorityGovernanceFamiliesFreshAndReplay(t *testing.T) {
	authorityDomainAFamily(t, "/teams", "suma_teams", 201, func(*fixture) map[string]any {
		return map[string]any{"name": "current credential team", "expected_version": 0, "idempotency_key": "authority-team-operation"}
	})
}

func TestAuthorityRecordFamiliesFreshAndReplay(t *testing.T) {
	authorityDomainAFamily(t, "/tasks", "suma_tasks", 201, func(f *fixture) map[string]any {
		return f.body(map[string]any{"title": "current credential task", "idempotency_key": "authority-task-operation"})
	})
}

func TestAuthorityPlanningFamiliesFreshAndReplay(t *testing.T) {
	t.Run("period", func(t *testing.T) {
		authorityDomainAFamily(t, "/iterations", "suma_planning_periods", 201, func(f *fixture) map[string]any {
			return f.body(map[string]any{"title": "current credential iteration", "start_date": "2026-10-19", "end_date": "2026-10-25", "idempotency_key": "authority-planning-operation"})
		})
	})
	t.Run("schedule", func(t *testing.T) {
		authorityDomainAFamily(t, "/planning/schedule", "suma_task_plans", 200, func(f *fixture) map[string]any {
			task := f.call("POST", "/tasks", f.body(map[string]any{"title": "schedule authority task"}), 201)
			return f.body(map[string]any{"changes": []any{map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "expected_version": 0, "iteration_id": "", "milestone_id": "", "start_date": "2026-10-19", "due_date": "2026-10-25", "estimated_hours": 8}}, "idempotency_key": "authority-schedule-operation"})
		})
	})
	t.Run("capacity", func(t *testing.T) {
		authorityDomainAFamily(t, "/planning/capacity", "suma_capacity", 200, func(f *fixture) map[string]any {
			member := f.call("POST", "/members", f.body(map[string]any{"title": "capacity authority member", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 201)
			return f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": 40, "expected_version": 0, "idempotency_key": "authority-capacity-operation"})
		})
	})
	t.Run("baseline", func(t *testing.T) {
		authorityDomainAFamily(t, "/planning/baselines", "suma_baselines", 201, func(f *fixture) map[string]any {
			return f.body(map[string]any{"title": "current credential baseline", "idempotency_key": "authority-baseline-operation"})
		})
	})
}

func TestAuthorityProjectAPITaskCreateAndReplay(t *testing.T) {
	f := newFixture(t)
	otherProject := f.call("POST", "/projects", map[string]any{"title": "Foreign API scope", "idempotency_key": "authority-other-project"}, 201)["id"].(string)
	ctx := context.Background()
	actor, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "browser", IdempotencyKey: "authority-" + scope + "-token"})
		if err != nil {
			t.Fatal(err)
		}
		body := f.body(map[string]any{"title": "API task", "idempotency_key": "authority-api-task-" + scope})
		var first string
		before := domainACount(t, f.s, "suma_tasks")
		for i := 0; i < 2; i++ {
			raw, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "http://suma.test/api/v1/tasks", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+issued.Token)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			if scope == "api" {
				if w.Code != 201 {
					t.Fatalf("API task returned %d: %s", w.Code, w.Body.String())
				}
				var out Record
				if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = out.ID
				} else if out.ID != first {
					t.Fatal("API replay changed task")
				}
			} else if w.Code != 403 {
				t.Fatalf("%s task write returned %d: %s", scope, w.Code, w.Body.String())
			}
		}
		want := before
		if scope == "api" {
			want++
		}
		if got := domainACount(t, f.s, "suma_tasks"); got != want {
			t.Fatalf("%s task rows=%d want=%d", scope, got, want)
		}
		if scope == "api" {
			body["project_id"] = otherProject
			raw, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "http://suma.test/api/v1/tasks", bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+issued.Token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			if w.Code != 403 || domainACount(t, f.s, "suma_tasks") != want {
				t.Fatalf("foreign-project API wrote task: status=%d body=%s", w.Code, w.Body.String())
			}
		}
	}
}

func TestAuthoritySystemFailureRequiresPersistedRunScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := dag.Workflow{ID: "trusted-failure", Name: "Persisted workflow", Nodes: []dag.Node{{ID: "gate", Type: "approval"}}}
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, w); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, "failure-authority-run")
	if err != nil {
		t.Fatal(err)
	}
	hint := dag.Run{ID: run.ID, State: "failed", WorkflowName: "forged projection"}
	err = f.s.FailureIssue(ctx, f.user.TenantID, f.project, hint)
	requireAuthorityStatus(t, err, 403)
	if got := domainACount(t, f.s, "suma_issues"); got != 0 {
		t.Fatal("unproven failure created issue")
	}
	if _, err := f.e.Command(ctx, f.user.TenantID, f.project, run.ID, "approve", "failure-authority-reject", run.Version, map[string]any{"node_id": "gate", "decision": "rejected"}); err != nil {
		t.Fatal(err)
	}
	for _, target := range [][2]string{{"foreign", f.project}, {f.user.TenantID, "foreign"}} {
		err := f.s.FailureIssue(ctx, target[0], target[1], hint)
		requireAuthorityStatus(t, err, 403)
	}
	if got := domainACount(t, f.s, "suma_issues"); got != 0 {
		t.Fatal("foreign failure scope created issue")
	}
	for i := 0; i < 2; i++ {
		if err := f.s.FailureIssue(ctx, f.user.TenantID, f.project, hint); err != nil {
			t.Fatal(err)
		}
	}
	if got := domainACount(t, f.s, "suma_issues"); got != 1 {
		t.Fatalf("failure projection rows=%d", got)
	}
	var title, operation string
	if err := f.s.DB.QueryRow(`SELECT title FROM suma_issues`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "执行失败：Persisted workflow" {
		t.Fatalf("caller projection replaced persisted workflow: %q", title)
	}
	if err := f.s.DB.QueryRow(`SELECT operation_id FROM suma_operations WHERE operation_id=?`, "failure-"+run.ID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorityReviewRechecksBothTransactions(t *testing.T) {
	for _, stage := range []string{"before_authority_tx", "receipt_released"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			_ = invitedReviewer(t, f)
			_, token, err := f.s.Login(context.Background(), "authority-reviewer@example.test", "invited-password-strong-42")
			if err != nil {
				t.Fatal(err)
			}
			actor := domainAActor(t, f.s, token)
			task, request := unreviewedTestTask(t, f)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			service, gate := reviewAuthorityConnection(t, f, ctx, stage)
			before := [3]int{domainACount(t, f.s, "suma_reviews"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
			finished := make(chan error, 1)
			go func() {
				_, err := service.Review(ctx, actor, f.project, task["id"].(string), request, f.e)
				finished <- err
			}()
			select {
			case <-gate.observed:
			case <-ctx.Done():
				t.Fatal("review authority boundary not reached")
			}
			if err := logoutCredentialForTest(ctx, f.s, token); err != nil {
				t.Fatal(err)
			}
			gate.release()
			select {
			case err = <-finished:
				requireAuthorityStatus(t, err, 401)
			case <-ctx.Done():
				t.Fatal("review did not finish")
			}
			after := [3]int{domainACount(t, f.s, "suma_reviews"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
			if after != before {
				t.Fatalf("denied review changed evidence/receipt/activity: before=%v after=%v", before, after)
			}
			var version int
			if err := f.s.DB.QueryRow(`SELECT version FROM suma_tasks WHERE id=?`, task["id"]).Scan(&version); err != nil || version != int(task["version"].(float64)) {
				t.Fatalf("denied review changed task: version=%d err=%v", version, err)
			}
		})
	}
}

func TestAuthorityDomainHelpersRequireOriginalProjectProof(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	actor := domainAActor(t, f.s, f.cookie.Value)
	ctx, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := f.s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{
		func() error { return f.s.BindRunTx(ctx, tx, nil, f.project, "", "", 0) },
		func() error { return f.s.BindHumanResourceTx(ctx, nil, auth, f.project, f.user.ID) },
		func() error { return f.s.InvalidateDependentsTx(ctx, tx, auth, "foreign", nil, false) },
		func() error { return f.s.InvalidateCompensationTx(ctx, tx, auth, "foreign", "") },
		func() error { return f.s.ReopenPlanningTx(ctx, tx, auth, "foreign") },
	} {
		requireAuthorityStatus(t, check(), 403)
	}
}
