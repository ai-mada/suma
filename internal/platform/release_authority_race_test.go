package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReleaseEnvironmentWorkerCredentialsCannotReadBusinessRoutes(t *testing.T) {
	f, _ := releaseTestSetup(t)
	environment := releaseTestEnvironment(t, f)
	// Omitting scope at the public credential endpoint is a real supported
	// request. It must still issue a runner scope rather than an unscoped key.
	ordinary := f.call("POST", "/auth/tokens", map[string]any{"project_id": f.project, "label": "scope omitted worker", "idempotency_key": newID()}, 201)["token"].(string)
	registered, err := f.s.RegisterReleaseWorker(context.Background(), f.actor, f.project, map[string]any{"worker_id": "read-isolation-worker", "profiles": []any{}, "deployment": true, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{ordinary, registered["token"].(string)} {
		kind, project := f.s.TokenScope(context.Background(), secret)
		if kind != "runner" || project != f.project {
			t.Fatal("public worker credential did not bind a runner project scope")
		}
		for _, path := range []string{"environments", "environments/" + environment.ID, "release-providers", "release-plans", "deployments"} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/"+path+"?project_id="+f.project, nil)
			request.Header.Set("Authorization", "Bearer "+secret)
			response := httptest.NewRecorder()
			f.handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("worker read %s returned HTTP %d", path, response.Code)
			}
		}
	}
}

// An actual initial authorization read is paused before its transaction begins.
// The owner removes the project grant on a second SQLite connection before the
// read resumes. Returning empty actions must not hide a current read denial.
func TestReleaseEnvironmentReadsRecheckAuthorityAfterConcurrentGrantRemoval(t *testing.T) {
	for _, target := range []string{"detail", "actions", "list"} {
		t.Run(target, func(t *testing.T) {
			f, _ := releaseTestSetup(t)
			environment := releaseTestEnvironment(t, f)
			ownerCookie := f.cookie
			invitation := governanceInvite(f, "release-read-race@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
			session := governanceAccept(f, invitation, "release-read-race@example.test")
			readerActor := domainCActor(t, f.s, f.cookie.Value)
			f.cookie = ownerCookie
			f.call("GET", "/auth/session", nil, 200)
			var reader Principal
			raw, err := json.Marshal(session["user"])
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &reader); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			service, gate := reviewAuthorityConnection(t, f, ctx, "before_authority_tx")
			finished := make(chan error, 1)
			go func() {
				var readErr error
				switch target {
				case "detail":
					_, readErr = service.ReleaseEnvironment(ctx, readerActor, f.project, environment.ID)
				case "actions":
					_, readErr = service.ReleaseEnvironmentActions(ctx, readerActor, f.project)
				case "list":
					_, readErr = service.ReleaseEnvironments(ctx, readerActor, f.project)
				}
				finished <- readErr
			}()
			select {
			case <-gate.observed:
			case <-ctx.Done():
				t.Fatal("release read did not reach its real before-authority transaction barrier")
			}
			_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": reader.ID, "action": "remove", "expected_version": float64(1), "idempotency_key": newID()})
			gate.release()
			if err != nil {
				t.Fatal("owner could not commit concurrent project grant removal:", err)
			}
			select {
			case err = <-finished:
				releaseWantStatus(t, err, 403)
			case <-ctx.Done():
				t.Fatal("release read hung after project grant removal")
			}
			var remaining int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, reader.ID).Scan(&remaining); err != nil || remaining != 0 {
				t.Fatalf("project access removal did not persist: count=%d err=%v", remaining, err)
			}
		})
	}
}
