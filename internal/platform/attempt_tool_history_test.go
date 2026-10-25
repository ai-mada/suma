package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

// Missing routing must fail visibly, rather than silently returning an empty history.
func TestAttemptToolHistoryHTTPReadsDurableIdentity(t *testing.T) {
	f := newFixture(t)
	run := streamRun(t, f)
	_, err := f.s.DB.ExecContext(context.Background(), `INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result)VALUES(?,?,1,7,'saved-worker','failed','','','')`, run.ID, "one")
	if err != nil {
		t.Fatal(err)
	}
	out := f.call("GET", "/runs/"+run.ID+"/nodes/one/attempts/1/tool-events?project_id="+f.project, nil, 200)
	if out["schema_version"] != float64(1) || out["fence"] != float64(7) || out["worker_id"] != "saved-worker" {
		t.Fatalf("wrong saved identity: %#v", out)
	}
}

func historyHTTPAttempt(t *testing.T, f *fixture) (dag.Run, string) {
	t.Helper()
	run := streamRun(t, f)
	if _, err := f.s.DB.Exec(`INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result)VALUES(?,?,1,7,'saved-worker','failed','','','')`, run.ID, "one"); err != nil {
		t.Fatal(err)
	}
	return run, "/runs/" + run.ID + "/nodes/one/attempts/1/tool-events?project_id=" + f.project
}

func TestAttemptToolHistoryHTTPStrictQueryAndScope(t *testing.T) {
	f := newFixture(t)
	run, base := historyHTTPAttempt(t, f)
	for _, suffix := range []string{"&extra=x", "&project_id=" + f.project, "&limit=1&limit=1", "&cursor=abc&cursor=abc", "&limit=", "&cursor=", "&limit=0", "&limit=101", "&limit=01", "&limit=1e0", "&limit=1.0", "&cursor=" + strings.Repeat("x", 4097), "&x=%ZZ"} {
		f.call("GET", base+suffix, nil, 400)
	}
	for _, raw := range []string{"0", "01", "-1", "1e0", "1.0", "9007199254740992"} {
		f.call("GET", "/runs/"+run.ID+"/nodes/one/attempts/"+raw+"/tool-events?project_id="+f.project, nil, 400)
	}
	f.call("GET", strings.Replace(base, "/attempts/1/", "/attempts/2/", 1), nil, 404)
	f.call("GET", strings.Replace(base, "/nodes/one/", "/nodes/missing/", 1), nil, 404)
	other := f.call("POST", "/projects", map[string]any{"title": "other", "idempotency_key": newID()}, 201)["id"].(string)
	for _, scope := range []string{"api", "runner", "agent"} {
		token := f.call("POST", "/auth/tokens", map[string]any{"project_id": f.project, "scope": scope, "idempotency_key": newID()}, 201)["token"].(string)
		want := 403
		if scope == "api" {
			want = 200
		}
		f.bearer("GET", base, token, nil, want)
		f.bearer("GET", strings.Replace(base, f.project, other, 1), token, nil, 403)
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("GET", base, nil, 401)
}

func TestAttemptToolHistoryHTTPProjectQueryByteBoundaryBeforeProjectLookup(t *testing.T) {
	f := newFixture(t)
	_, base := historyHTTPAttempt(t, f)
	for _, tc := range []struct {
		project string
		status  int
	}{
		{strings.Repeat("x", 512), 404}, {strings.Repeat("x", 513), 400},
		{strings.Repeat("é", 256), 404}, {strings.Repeat("界", 171), 400},
	} {
		t.Run(fmt.Sprintf("bytes=%d", len(tc.project)), func(t *testing.T) {
			f.call("GET", strings.Replace(base, f.project, url.QueryEscape(tc.project), 1), nil, tc.status)
		})
	}
	// Syntax validation follows authentication and credential boundaries.
	token := f.call("POST", "/auth/tokens", map[string]any{"project_id": f.project, "scope": "runner", "idempotency_key": newID()}, 201)["token"].(string)
	f.bearer("GET", strings.Replace(base, f.project, strings.Repeat("x", 513), 1), token, nil, 403)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("GET", strings.Replace(base, f.project, strings.Repeat("x", 513), 1), nil, 401)
}

// Snapshot every real table except the explicitly allowed coordination counter.
func historyDatabaseHash(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT IN ('suma_governance_locks','suma_authority_sqlite_guard','suma_identity_authority_locks','suma_authority_credential_locks') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	rows.Close()
	out := []string{}
	for _, table := range tables {
		rows, err = db.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		lines := []string{}
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			if err = rows.Scan(p...); err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, string(b))
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		sort.Strings(lines)
		out = append(out, table, strings.Join(lines, "\n"))
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(out, "\n"))))
}

func TestAttemptToolHistoryHTTPAuthorizationAndSelectionShareGuardedTransaction(t *testing.T) {
	f := newFixture(t)
	_, base := historyHTTPAttempt(t, f)
	var path string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	trace := &schedulingSQLTrace{}
	name := "attempt_history_trace_" + newID()
	sql.Register(name, &schedulingTraceDriver{trace: trace})
	db, err := sql.Open(name, path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f.s.DB = db
	f.e = engine
	f.handler = (&Server{Service: f.s, Engine: engine}).Handler()
	before := historyDatabaseHash(t, db)
	trace.mu.Lock()
	trace.transactions = nil
	trace.mu.Unlock()
	f.call("GET", base, nil, 200)
	after := historyDatabaseHash(t, db)
	if before != after {
		t.Fatal("GET changed business, credential, execution or receipt tables")
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	selected := 0
	for _, tx := range trace.transactions {
		lock, session, scope, org, attempt := -1, -1, -1, -1, -1
		for i, q := range tx.Statements {
			switch {
			case strings.HasPrefix(q, "UPDATE suma_governance_locks"):
				lock = i
			case strings.Contains(q, "FROM suma_sessions"):
				session = i
			case strings.Contains(q, "FROM suma_token_scopes"):
				scope = i
			case strings.Contains(q, "FROM suma_users u JOIN suma_org_members"):
				org = i
			case strings.Contains(q, "FROM dag_attempts a JOIN dag_runs"):
				attempt = i
			}
			if strings.HasPrefix(q, "UPDATE dag_lock") || strings.Contains(q, "INSERT INTO dag_") {
				t.Fatal("read acquired scheduler lock or wrote execution", q)
			}
			if strings.Contains(q, "FROM dag_events") && strings.Contains(q, "LIMIT 1001") {
				selected++
				if !tx.Committed || lock < 0 || session < lock || scope < session || org < lock || attempt < scope || i < attempt {
					t.Fatalf("selection before current authority: %+v", tx)
				}
			}
		}
	}
	if selected != 1 {
		t.Fatalf("expected exactly one bounded selection in authorized transaction, got %d", selected)
	}
}

func TestAttemptToolHistoryHTTPRechecksCurrentAuthorityAfterSecondConnectionChanges(t *testing.T) {
	for _, change := range []string{"session_revoked", "session_expired", "project_grant", "organization_member", "token_scope"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			run, base := historyHTTPAttempt(t, f)
			member := invitedReviewer(t, f)
			_, token, err := f.s.Login(context.Background(), "authority-reviewer@example.test", "invited-password-strong-42")
			if err != nil {
				t.Fatal(err)
			}
			if change == "token_scope" {
				issued, e := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "history", IdempotencyKey: newID()})
				if e != nil {
					t.Fatal(e)
				}
				token = issued.Token
			}
			expected, err := f.s.AuthenticateActor(context.Background(), token)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.withCurrentRead(context.Background(), expected, commandTarget{tenantID: expected.Projection().TenantID, projectID: f.project}, purposeProjectRead, func(*authorizedCommand) error { return nil }); err != nil {
				t.Fatal(err)
			}
			var path string
			if err = f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
				t.Fatal(err)
			}
			other, err := store.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			want := 403
			switch change {
			case "session_revoked":
				err = logoutCredentialForTest(context.Background(), &Service{DB: other, Dialect: "sqlite"}, token)
				want = 401
			case "session_expired":
				_, err = other.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(token))
				want = 401
			case "project_grant":
				_, err = (&Service{DB: other, Dialect: "sqlite"}).SaveProjectAccess(context.Background(), f.actor, map[string]any{"project_id": f.project, "user_id": member.Projection().ID, "action": "remove", "expected_version": float64(1), "idempotency_key": newID()})
			case "organization_member":
				_, err = other.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, member.Projection().TenantID, member.Projection().ID)
			case "token_scope":
				_, err = other.Exec(`UPDATE suma_token_scopes SET project_id='foreign' WHERE token_hash=?`, digest(token))
				want = 401
			}
			if err != nil {
				t.Fatal(err)
			}
			// Resume the actual route body with the authenticator's earlier principal.
			req := httptest.NewRequest("GET", "/api/v1"+base, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			server := &Server{Service: f.s, Engine: f.e}
			server.attemptToolHistoryHTTP(w, req, expected, []string{"runs", run.ID, "nodes", "one", "attempts", "1", "tool-events"}, f.project)
			if w.Code != want || strings.Contains(w.Body.String(), `"items"`) {
				t.Fatalf("stale authority leaked history: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAttemptToolHistoryHTTPInfrastructureFailureIsVisible(t *testing.T) {
	f := newFixture(t)
	_, base := historyHTTPAttempt(t, f)
	if _, err := f.s.DB.Exec(`DROP TABLE dag_events`); err != nil {
		t.Fatal(err)
	}
	f.call("GET", base, nil, 500)
}
