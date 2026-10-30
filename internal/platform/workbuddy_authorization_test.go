package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/workbuddy"
)

func workBuddyCommandFixture(t *testing.T, handler http.HandlerFunc) (*fixture, *WorkBuddyConnections) {
	t.Helper()
	f := newFixture(t)
	if err := f.s.InitializeWorkBuddyConnections(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	w, err := NewWorkBuddyConnections(f.s, workbuddy.OAuthConfig{BaseURL: server.URL, ClientID: "app-test", ClientSecret: "private-app-secret", RedirectURI: "https://suma.example/oauth", Scopes: []string{"user.task.readable", "user.task.invokable"}, HTTPClient: server.Client()}, []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return f, w
}
func workBuddyCommandResponse(w http.ResponseWriter, id, scope, refresh string) {
	w.Header().Set("Content-Type", "application/json")
	extra := ""
	if refresh != "" {
		extra = fmt.Sprintf(`,"refresh_token":%q`, refresh)
	}
	fmt.Fprintf(w, `{"access_token":"private-access","token_type":"Bearer","expires_in":3600,"open_id":%q,"scope":%q%s}`, id, scope, extra)
}
func workBuddyCommandStart(t *testing.T, f *fixture, w *WorkBuddyConnections) (WorkBuddyAuthorization, WorkBuddyAuthorizationRequest) {
	t.Helper()
	req := WorkBuddyAuthorizationRequest{ProjectID: f.project, Scopes: []string{"user.task.readable", "user.task.invokable"}, IdempotencyKey: newID()}
	auth, err := w.StartAuthorization(context.Background(), f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	return auth, req
}
func workBuddyCommandComplete(auth WorkBuddyAuthorization) WorkBuddyAuthorizationComplete {
	u, _ := url.Parse(auth.AuthorizationURL)
	return WorkBuddyAuthorizationComplete{ProjectID: auth.Connection.ProjectID, ExpectedVersion: auth.Version, State: u.Query().Get("state"), Code: "private-code", IdempotencyKey: newID()}
}
func TestWorkBuddyConnectionsAuthorizationLifecycleReceipts(t *testing.T) {
	var calls atomic.Int32
	f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		workBuddyCommandResponse(out, "open-one", "user.task.readable user.task.invokable", "private-refresh")
	})
	auth, start := workBuddyCommandStart(t, f, w)
	again, err := w.StartAuthorization(context.Background(), f.actor, f.project, start)
	if err != nil || again.ID != auth.ID || again.AuthorizationURL != auth.AuthorizationURL || calls.Load() != 0 {
		t.Fatal("start replay changed authorization", again, err)
	}
	req := workBuddyCommandComplete(auth)
	connected, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
	if err != nil || connected.State != "connected" || connected.OpenID != "open-one" || connected.GrantGeneration != 1 || connected.TokenGeneration != 1 {
		t.Fatal("authorization not connected", connected, err)
	}
	replay, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
	if err != nil || replay.Version != connected.Version || calls.Load() != 1 {
		t.Fatal("exchange replayed", replay, err, calls.Load())
	}
	changed := req
	changed.Code = "different-code"
	_, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, changed)
	requireAuthorityStatus(t, err, 409)
	changed = req
	changed.IdempotencyKey = newID()
	_, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, changed)
	requireAuthorityStatus(t, err, 409)
	if calls.Load() != 1 {
		t.Fatal("conflict caused external exchange")
	}
	rows, err := f.s.DB.Query(`SELECT response FROM suma_operations WHERE project_id=?`, f.project)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{req.Code, req.State, "private-access", "private-refresh", "private-app-secret"} {
			if strings.Contains(raw, secret) {
				t.Fatal("secret persisted in operation receipt")
			}
		}
	}
}

func TestWorkBuddyConnectionsAuthorizationStateSessionProjectAndExpiry(t *testing.T) {
	for _, kind := range []string{"state", "session", "project", "expired"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				workBuddyCommandResponse(out, "open-one", "user.task.readable", "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			req := workBuddyCommandComplete(auth)
			actor, project := f.actor, f.project
			switch kind {
			case "state":
				req.State += "wrong"
			case "session":
				f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
				var err error
				actor, err = f.s.AuthenticateActor(context.Background(), f.cookie.Value)
				if err != nil {
					t.Fatal(err)
				}
			case "project":
				project = newID()
				req.ProjectID = project
			case "expired":
				op, err := w.opTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.ID, false)
				if err != nil {
					t.Fatal(err)
				}
				op.ExpiresAt = "2000-01-01T00:00:00Z"
				req.State = w.stateToken(op)
				if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_oauth_operations SET expires_at=?,state_hash=? WHERE id=?`, op.ExpiresAt, digest(req.State), op.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := w.CompleteAuthorization(context.Background(), actor, project, auth.ID, req); err == nil {
				t.Fatal("invalid callback accepted")
			}
			if calls.Load() != 0 {
				t.Fatal("invalid callback sent code")
			}
		})
	}
}

func TestWorkBuddyConnectionsAuthorizationUnknownAndIdentityChange(t *testing.T) {
	for _, kind := range []string{"unknown", "identity"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if kind == "unknown" {
					out.WriteHeader(503)
					return
				}
				id := "open-one"
				if n > 1 {
					id = "open-two"
				}
				workBuddyCommandResponse(out, id, "user.task.readable user.task.invokable", "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			req := workBuddyCommandComplete(auth)
			result, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "unknown" {
				if result.State != "authorization_unknown" {
					t.Fatal("unknown exchange treated as reusable", result)
				}
				_, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
				if err != nil || calls.Load() != 1 {
					t.Fatal("unknown exchange repeated", err, calls.Load())
				}
			} else {
				next, err := w.StartAuthorization(context.Background(), f.actor, f.project, WorkBuddyAuthorizationRequest{ProjectID: f.project, ConnectionID: result.ID, ExpectedVersion: result.Version, Scopes: []string{"user.task.readable", "user.task.invokable"}, IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
				result, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, next.ID, workBuddyCommandComplete(next))
				if err != nil || result.State != "reauthorization_required" || result.OpenID != "open-one" {
					t.Fatal("reauthorization silently replaced account", result, err)
				}
			}
		})
	}
}

func TestWorkBuddyConnectionsAuthorizationRevokedDuringExchange(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		workBuddyCommandResponse(out, "open-one", "user.task.readable", "private-refresh")
	})
	auth, _ := workBuddyCommandStart(t, f, w)
	req := workBuddyCommandComplete(auth)
	done := make(chan error, 1)
	go func() {
		_, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
		done <- err
	}()
	<-entered
	if _, err := f.s.DB.Exec(`DELETE FROM suma_sessions WHERE token_hash=?`, f.actor.credential.digest); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("revoked callback stored grant")
	}
	row, err := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.Connection.ID, false)
	if err != nil || row.State == "connected" || row.Ciphertext != "" {
		t.Fatal("grant resurrected after revoke", err)
	}
	if _, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req); err == nil {
		t.Fatal("revoked receipt read succeeded")
	}
}

func TestWorkBuddyConnectionsAuthorizationConcurrentPermitAndSaveFailure(t *testing.T) {
	for _, kind := range []string{"concurrent", "save_failure"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				close(entered)
				<-release
				workBuddyCommandResponse(out, "open-one", "user.task.readable", "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			req := workBuddyCommandComplete(auth)
			if kind == "save_failure" {
				if _, err := f.s.DB.Exec(`CREATE TRIGGER fail_workbuddy_grant BEFORE UPDATE ON suma_workbuddy_connections WHEN NEW.state='connected' BEGIN SELECT RAISE(ABORT,'test grant save rejected'); END`); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				_, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
				done <- err
			}()
			<-entered
			if kind == "concurrent" {
				if _, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req); err != nil {
					close(release)
					t.Fatal(err)
				}
			}
			close(release)
			err := <-done
			if kind == "concurrent" && err != nil {
				t.Fatal(err)
			}
			if kind == "save_failure" && err == nil {
				t.Fatal("save failure hidden")
			}
			replay, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
			if err != nil || calls.Load() != 1 {
				t.Fatal("prepared exchange repeated after outcome", err, calls.Load())
			}
			if kind == "save_failure" && replay.State != "authorization_unknown" {
				t.Fatal("save failure not retained as unknown", replay.State)
			}
		})
	}
}

func TestWorkBuddyConnectionsAuthorizationCancelWithoutExchange(t *testing.T) {
	var calls atomic.Int32
	f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) { calls.Add(1) })
	auth, _ := workBuddyCommandStart(t, f, w)
	complete := workBuddyCommandComplete(auth)
	req := WorkBuddyAuthorizationCancel{ProjectID: f.project, ExpectedVersion: auth.Version, State: complete.State, IdempotencyKey: newID()}
	row, err := w.CancelAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
	if err != nil || row.State != "disconnected" {
		t.Fatal(row, err)
	}
	replay, err := w.CancelAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
	if err != nil || replay.Version != row.Version || calls.Load() != 0 {
		t.Fatal("cancel replay changed facts", replay, err)
	}
	if _, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, complete); err == nil || calls.Load() != 0 {
		t.Fatal("cancelled authorization exchanged")
	}
}

func TestWorkBuddyConnectionsAuthorizationVersionRequestBoundary(t *testing.T) {
	var calls atomic.Int32
	f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) { calls.Add(1) })
	auth, _ := workBuddyCommandStart(t, f, w)
	complete := workBuddyCommandComplete(auth)
	for _, version := range []int{2147483647, 2147483646} {
		want := 400
		if version == 2147483646 {
			want = 409
		}
		_, err := w.StartAuthorization(context.Background(), f.actor, f.project, WorkBuddyAuthorizationRequest{ProjectID: f.project, ConnectionID: auth.Connection.ID, ExpectedVersion: version, Scopes: []string{"user.task.readable"}, IdempotencyKey: newID()})
		requireAuthorityStatus(t, err, want)
		complete.ExpectedVersion = version
		complete.IdempotencyKey = newID()
		_, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, complete)
		requireAuthorityStatus(t, err, want)
		_, err = w.CancelAuthorization(context.Background(), f.actor, f.project, auth.ID, WorkBuddyAuthorizationCancel{ProjectID: f.project, ExpectedVersion: version, State: complete.State, IdempotencyKey: newID()})
		requireAuthorityStatus(t, err, want)
		_, err = w.Refresh(context.Background(), f.actor, f.project, auth.Connection.ID, WorkBuddyRefreshRequest{ProjectID: f.project, ExpectedVersion: version, IdempotencyKey: newID()})
		requireAuthorityStatus(t, err, want)
		_, err = w.Disconnect(context.Background(), f.actor, f.project, auth.Connection.ID, WorkBuddyDisconnectRequest{ProjectID: f.project, ExpectedVersion: version, Reason: "Boundary validation", IdempotencyKey: newID()})
		requireAuthorityStatus(t, err, want)
	}
	row, err := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.Connection.ID, false)
	if err != nil || row.Version != auth.Connection.Version || row.State != "authorizing" || calls.Load() != 0 {
		t.Fatal("invalid version changed facts or sent request", err, calls.Load())
	}
}

func TestWorkBuddyConnectionsAuthorizationExternalCASCapacity(t *testing.T) {
	for _, boundary := range []string{"complete_row", "complete_operation", "token_generation", "grant_generation", "refresh_row"} {
		t.Run(boundary, func(t *testing.T) {
			var calls atomic.Int32
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				workBuddyCommandResponse(out, "open-one", "user.task.readable user.task.invokable", "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			req := workBuddyCommandComplete(auth)
			if boundary == "refresh_row" {
				if _, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req); err != nil {
					t.Fatal(err)
				}
			}
			switch boundary {
			case "complete_row", "refresh_row":
				if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET version=2147483646 WHERE id=?`, auth.Connection.ID); err != nil {
					t.Fatal(err)
				}
				if boundary == "complete_row" {
					if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_oauth_operations SET connection_version=2147483646 WHERE id=?`, auth.ID); err != nil {
						t.Fatal(err)
					}
				}
			case "complete_operation":
				if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_oauth_operations SET version=2147483646 WHERE id=?`, auth.ID); err != nil {
					t.Fatal(err)
				}
				req.ExpectedVersion = 2147483646
			case "token_generation":
				if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET token_generation=2147483647 WHERE id=?`, auth.Connection.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_oauth_operations SET token_generation=2147483647 WHERE id=?`, auth.ID); err != nil {
					t.Fatal(err)
				}
			case "grant_generation":
				if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET grant_generation=2147483647 WHERE id=?`, auth.Connection.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_oauth_operations SET grant_generation=2147483647 WHERE id=?`, auth.ID); err != nil {
					t.Fatal(err)
				}
			}
			before, err := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.Connection.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			opBefore, err := w.opTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			beforeCalls := calls.Load()
			if boundary == "refresh_row" {
				_, err = w.Refresh(context.Background(), f.actor, f.project, before.ID, WorkBuddyRefreshRequest{ProjectID: f.project, ExpectedVersion: before.Version, IdempotencyKey: newID()})
			} else {
				_, err = w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, req)
			}
			requireAuthorityStatus(t, err, 409)
			after, e := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, before.ID, false)
			if e != nil {
				t.Fatal(e)
			}
			opAfter, e := w.opTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.ID, false)
			if e != nil {
				t.Fatal(e)
			}
			if stringify(before) != stringify(after) || opBefore != opAfter || calls.Load() != beforeCalls {
				t.Fatal("exhausted CAS capacity consumed permit or sent token request")
			}
		})
	}
}

func TestWorkBuddyConnectionsAuthorizationStartReservesCompletionCapacity(t *testing.T) {
	for _, boundary := range []string{"max_minus_one", "max_minus_two", "last_valid", "grant_generation", "token_generation"} {
		t.Run(boundary, func(t *testing.T) {
			var calls atomic.Int32
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				workBuddyCommandResponse(out, "open-one", "user.task.readable user.task.invokable", "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			connected, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, workBuddyCommandComplete(auth))
			if err != nil {
				t.Fatal(err)
			}
			originalRow, e := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, connected.ID, false)
			if e != nil {
				t.Fatal(e)
			}
			originalGrant, e := w.open(originalRow)
			if e != nil {
				t.Fatal(e)
			}
			version := 2147483644
			switch boundary {
			case "max_minus_one":
				version = 2147483646
			case "max_minus_two":
				version = 2147483645
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET version=? WHERE id=?`, version, connected.ID); err != nil {
				t.Fatal(err)
			}
			if boundary == "grant_generation" {
				if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET grant_generation=2147483646 WHERE id=?`, connected.ID); err != nil {
					t.Fatal(err)
				}
			}
			if boundary == "token_generation" {
				if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET token_generation=2147483647 WHERE id=?`, connected.ID); err != nil {
					t.Fatal(err)
				}
			}
			before, err := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, connected.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "grant_generation" || boundary == "token_generation" {
				before.Ciphertext, err = w.seal(before, originalGrant)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET ciphertext=? WHERE id=?`, before.Ciphertext, before.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = w.open(before); err != nil {
				t.Fatal("boundary fixture grant is not usable", err)
			}
			originalOp, err := w.opTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			var opCount int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workbuddy_oauth_operations WHERE connection_id=?`, connected.ID).Scan(&opCount); err != nil {
				t.Fatal(err)
			}
			next, err := w.StartAuthorization(context.Background(), f.actor, f.project, WorkBuddyAuthorizationRequest{ProjectID: f.project, ConnectionID: connected.ID, ExpectedVersion: version, Scopes: []string{"user.task.readable", "user.task.invokable"}, IdempotencyKey: newID()})
			if boundary == "last_valid" {
				if err != nil {
					t.Fatal("last valid start rejected", err)
				}
				final, e := w.CompleteAuthorization(context.Background(), f.actor, f.project, next.ID, workBuddyCommandComplete(next))
				if e != nil || final.Version != 2147483647 || final.State != "connected" || final.OpenID != connected.OpenID || calls.Load() != 2 {
					t.Fatal("reserved final transition did not complete", final, e, calls.Load())
				}
				return
			}
			requireAuthorityStatus(t, err, 409)
			after, e := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, connected.ID, false)
			if e != nil {
				t.Fatal(e)
			}
			opAfter, e := w.opTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.ID, false)
			if e != nil {
				t.Fatal(e)
			}
			var count int
			if e = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workbuddy_oauth_operations WHERE connection_id=?`, connected.ID).Scan(&count); e != nil {
				t.Fatal(e)
			}
			if stringify(before) != stringify(after) || originalOp != opAfter || opCount != count || calls.Load() != 1 || after.Ciphertext == "" {
				t.Fatal("failed start discarded usable grant or created new authority")
			}
		})
	}
}
