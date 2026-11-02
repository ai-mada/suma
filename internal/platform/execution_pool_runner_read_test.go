package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func exactPoolRunnerStatus(t *testing.T, err error, status int) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("exact runner read status: %v, want %d", err, status)
	}
}

func TestExecutionPoolRunnerExactReadAuthorityAndPageBoundary(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := seedCredentialPool(t, f)
	registered := make([]ExecutionPoolCredentialResponse, 0, 51)
	for index := 0; index < 51; index++ {
		value, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool, ExecutionPoolRunnerCreateRequest{
			Name: fmt.Sprintf("exact-node-%02d", index), MaxSlots: 2, TTLSeconds: 3600, IdempotencyKey: newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
		registered = append(registered, value)
	}
	page, err := f.s.ListExecutionPoolRunners(ctx, f.actor, pool, 50, "")
	if err != nil || len(page.Items) != 50 || page.Total != 51 || page.NextCursor == "" {
		t.Fatalf("bounded first page: count=%d total=%d err=%v", len(page.Items), page.Total, err)
	}
	firstPage := map[string]bool{}
	for _, runner := range page.Items {
		firstPage[runner.ID] = true
	}
	var target ExecutionPoolCredentialResponse
	for _, runner := range registered {
		if !firstPage[runner.Runner.ID] {
			target = runner
			break
		}
	}
	if target.Runner.ID == "" {
		t.Fatal("fixture did not retain a registered runner beyond the first page")
	}
	got, err := f.s.GetExecutionPoolRunner(ctx, f.actor, pool, target.Runner.ID)
	if err != nil || got.ID != target.Runner.ID || got.PoolID != pool || got.WorkerID != target.Runner.WorkerID || got.Version != 1 || got.CredentialVersion != 1 || got.LastSeenAt != target.Runner.LastSeenAt {
		t.Fatalf("exact off-page registration was not read: %+v %v", got, err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{target.Token, digest(target.Token), "token_hash", `"token"`} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("read-only registration response disclosed a credential")
		}
	}
	otherPool := seedCredentialPool(t, f)
	_, err = f.s.GetExecutionPoolRunner(ctx, f.actor, otherPool, target.Runner.ID)
	exactPoolRunnerStatus(t, err, 404)
	_, err = f.s.GetExecutionPoolRunner(ctx, f.actor, pool, newID())
	exactPoolRunnerStatus(t, err, 404)
	_, err = f.s.GetExecutionPoolRunner(ctx, f.actor, newID(), target.Runner.ID)
	exactPoolRunnerStatus(t, err, 404)

	// The original actor still projects owner, but current organization
	// membership is authoritative for every exact read, including missing IDs.
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetExecutionPoolRunner(ctx, f.actor, pool, target.Runner.ID)
	exactPoolRunnerStatus(t, err, 403)
	_, err = f.s.GetExecutionPoolRunner(ctx, f.actor, newID(), newID())
	exactPoolRunnerStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='admin' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetExecutionPoolRunner(ctx, f.actor, pool, target.Runner.ID); err != nil {
		t.Fatal("current organization admin could not read exact owned runner", err)
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetExecutionPoolRunner(ctx, f.actor, pool, target.Runner.ID)
	exactPoolRunnerStatus(t, err, 401)
}

func TestExecutionPoolRunnerExactHTTPClosedQueryAndNoSecrets(t *testing.T) {
	f := newFixture(t)
	pool := seedCredentialPool(t, f)
	registered, err := f.s.RegisterExecutionPoolRunner(context.Background(), f.actor, pool, ExecutionPoolRunnerCreateRequest{Name: "exact-http-node", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	path := "/execution-pools/" + pool + "/runners/" + registered.Runner.ID
	read := func(path string, status int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://suma.test/api/v1"+path, nil)
		request.AddCookie(f.cookie)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("GET %s: %d cache=%q, want %d no-store: %s", path, response.Code, response.Header().Get("Cache-Control"), status, response.Body.String())
		}
		return response
	}
	response := read(path, http.StatusOK)
	var out ExecutionPoolRunner
	if err = json.Unmarshal(response.Body.Bytes(), &out); err != nil || out.ID != registered.Runner.ID || out.PoolID != pool || out.WorkerID != registered.Runner.WorkerID {
		t.Fatal("HTTP response lost exact registration", err)
	}
	if strings.Contains(response.Body.String(), registered.Token) || strings.Contains(response.Body.String(), "token_hash") {
		t.Fatal("HTTP response disclosed a machine secret")
	}
	for _, query := range []string{"?", "?project_id=" + f.project, "?limit=1", "?cursor=next", "?worker_id=other", "?x=1&x=2", "?%zz=1"} {
		read(path+query, http.StatusBadRequest)
	}
	read("/execution-pools/"+pool+"/runners/"+newID(), http.StatusNotFound)
	apiToken := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	f.bearer(http.MethodGet, path, apiToken, nil, http.StatusForbidden)
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	// Current pool authority wins over malformed input and nonexistent object
	// IDs, so the closed query parser does not become an existence oracle.
	read(path+"?project_id=foreign", http.StatusForbidden)
	read("/execution-pools/"+newID()+"/runners/"+newID()+"?bad=1", http.StatusForbidden)
}
