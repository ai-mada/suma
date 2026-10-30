package platform

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWorkBuddyConnectionsRefreshRotationAndAbsentToken(t *testing.T) {
	var calls atomic.Int32
	f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		r.ParseForm()
		if n > 1 && r.Form.Get("refresh_token") != "private-refresh" {
			t.Error("original refresh missing")
		}
		refresh := "private-refresh"
		if n > 1 {
			refresh = ""
		}
		workBuddyCommandResponse(out, "open-one", "user.task.readable user.task.invokable", refresh)
	})
	auth, _ := workBuddyCommandStart(t, f, w)
	connected, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, workBuddyCommandComplete(auth))
	if err != nil {
		t.Fatal(err)
	}
	req := WorkBuddyRefreshRequest{ProjectID: f.project, ExpectedVersion: connected.Version, IdempotencyKey: newID()}
	next, err := w.Refresh(context.Background(), f.actor, f.project, connected.ID, req)
	if err != nil || next.State != "connected" || next.GrantGeneration != connected.GrantGeneration || next.TokenGeneration != 2 {
		t.Fatal("refresh changed grant identity", next, err)
	}
	replay, err := w.Refresh(context.Background(), f.actor, f.project, connected.ID, req)
	if err != nil || replay.Version != next.Version || calls.Load() != 2 {
		t.Fatal("refresh replay repeated wire", replay, err)
	}
	req.ExpectedVersion = next.Version
	req.IdempotencyKey = newID()
	next, err = w.Refresh(context.Background(), f.actor, f.project, connected.ID, req)
	if err != nil || next.TokenGeneration != 3 || calls.Load() != 3 {
		t.Fatal("absent refresh token was discarded", next, err)
	}
}

func TestWorkBuddyConnectionsRefreshUnknownAndIdentityScopeChange(t *testing.T) {
	for _, kind := range []string{"unknown", "identity", "scope", "scope_config"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				id, scope := "open-one", "user.task.readable user.task.invokable"
				if n > 1 {
					switch kind {
					case "unknown":
						out.WriteHeader(502)
						return
					case "identity":
						id = "open-two"
					case "scope":
						scope = "user.task.readable"
					}
				}
				workBuddyCommandResponse(out, id, scope, "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			connected, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, workBuddyCommandComplete(auth))
			if err != nil {
				t.Fatal(err)
			}
			req := WorkBuddyRefreshRequest{ProjectID: f.project, ExpectedVersion: connected.Version, IdempotencyKey: newID()}
			if kind == "scope_config" {
				w.app.Scopes = []string{"user.task.readable"}
			}
			row, err := w.Refresh(context.Background(), f.actor, f.project, connected.ID, req)
			if kind == "scope_config" {
				if err == nil || calls.Load() != 1 {
					t.Fatal("config scope ceiling bypassed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := "reauthorization_required"
			if kind == "unknown" {
				expected = "refresh_unknown"
			}
			if row.State != expected || row.OpenID != "open-one" || row.TokenGeneration != 1 {
				t.Fatal("changed grant accepted", row)
			}
			_, err = w.Refresh(context.Background(), f.actor, f.project, row.ID, req)
			if err != nil || calls.Load() != 2 {
				t.Fatal("failed refresh retried", err, calls.Load())
			}
		})
	}
}

func TestWorkBuddyConnectionsRefreshConcurrentAndDisconnectFence(t *testing.T) {
	for _, kind := range []string{"concurrent", "disconnect"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) {
				if calls.Add(1) > 1 {
					close(entered)
					<-release
				}
				workBuddyCommandResponse(out, "open-one", "user.task.readable user.task.invokable", "private-refresh")
			})
			auth, _ := workBuddyCommandStart(t, f, w)
			connected, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, workBuddyCommandComplete(auth))
			if err != nil {
				t.Fatal(err)
			}
			req := WorkBuddyRefreshRequest{ProjectID: f.project, ExpectedVersion: connected.Version, IdempotencyKey: newID()}
			done := make(chan error, 1)
			go func() { _, err := w.Refresh(context.Background(), f.actor, f.project, connected.ID, req); done <- err }()
			<-entered
			preparing, err := w.Refresh(context.Background(), f.actor, f.project, connected.ID, req)
			if err != nil || preparing.State != "refreshing" {
				close(release)
				t.Fatal("second refresh not projected", preparing, err)
			}
			if kind == "disconnect" {
				// Local withdrawal must remain possible when application config/key is gone.
				disabled := &WorkBuddyConnections{s: f.s}
				_, err = disabled.Disconnect(context.Background(), f.actor, f.project, connected.ID, WorkBuddyDisconnectRequest{ProjectID: f.project, ExpectedVersion: preparing.Version, Reason: "Stop local authorization", IdempotencyKey: newID()})
				if err != nil {
					close(release)
					t.Fatal(err)
				}
			}
			close(release)
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			replay, err := w.Refresh(context.Background(), f.actor, f.project, connected.ID, req)
			if err != nil || calls.Load() != 2 {
				t.Fatal("refresh repeated", err, calls.Load())
			}
			if kind == "disconnect" && replay.State != "disconnected" {
				t.Fatal("late refresh restored disconnected grant", replay)
			}
		})
	}
}

func TestWorkBuddyConnectionsRefreshDisconnectReasonBoundary(t *testing.T) {
	var calls atomic.Int32
	f, w := workBuddyCommandFixture(t, func(out http.ResponseWriter, r *http.Request) { calls.Add(1) })
	auth, _ := workBuddyCommandStart(t, f, w)
	for _, reason := range []string{strings.Repeat("界", 2001), strings.Repeat("😀", 2001), "reason\x00tail", string([]byte{0xff}), "   "} {
		_, err := w.Disconnect(context.Background(), f.actor, f.project, auth.Connection.ID, WorkBuddyDisconnectRequest{ProjectID: f.project, ExpectedVersion: auth.Connection.Version, Reason: reason, IdempotencyKey: newID()})
		requireAuthorityStatus(t, err, 400)
		row, e := w.rowTx(context.Background(), f.s.DB, f.user.TenantID, f.project, auth.Connection.ID, false)
		if e != nil || row.Version != auth.Connection.Version || row.State != "authorizing" {
			t.Fatal("invalid reason changed connection", row.State, e)
		}
	}
	row, err := w.Disconnect(context.Background(), f.actor, f.project, auth.Connection.ID, WorkBuddyDisconnectRequest{ProjectID: f.project, ExpectedVersion: auth.Connection.Version, Reason: strings.Repeat("停止本地授权", 200), IdempotencyKey: newID()})
	if err != nil || row.State != "disconnected" {
		t.Fatal("valid long Chinese reason rejected", err)
	}
	second, _ := workBuddyCommandStart(t, f, w)
	row, err = w.Disconnect(context.Background(), f.actor, f.project, second.Connection.ID, WorkBuddyDisconnectRequest{ProjectID: f.project, ExpectedVersion: second.Connection.Version, Reason: strings.Repeat("😀", 2000), IdempotencyKey: newID()})
	if err != nil || row.State != "disconnected" || calls.Load() != 0 {
		t.Fatal("exact Unicode byte boundary failed", err, calls.Load())
	}
}
