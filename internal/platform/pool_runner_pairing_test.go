package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pc "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"
	"github.com/ai-mada/suma/internal/store"
)

func poolPairingFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := pairingFixture(t)
	return f, seedCredentialPool(t, f)
}
func poolPairingCreate(t *testing.T, f *fixture, pool, name string) PoolRunnerPairing {
	t.Helper()
	p, e := f.s.CreatePoolRunnerPairing(context.Background(), f.actor, PoolRunnerPairingCreateRequest{PoolID: pool, Name: name, MaxSlots: 2, TTLSeconds: 3600, IdempotencyKey: newID()}, pairingTestControl)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func poolPairingProposed(t *testing.T, f *fixture, pool string) (PoolRunnerPairing, string, string) {
	t.Helper()
	p := poolPairingCreate(t, f, pool, "Reviewed node")
	token, key := strings.Repeat("a", 32)+newID(), newID()
	p, e := f.s.ProposePoolRunnerPairing(context.Background(), p.ID, PoolRunnerPairingProposalRequest{PoolID: pool, DeviceName: "Owned remote node", CredentialSHA256: digest(token), IdempotencyKey: key}, token, pairingTestControl)
	if e != nil {
		t.Fatal(e)
	}
	return p, token, key
}
func poolPairingWantStatus(t *testing.T, e error, status int) {
	t.Helper()
	var api *APIError
	if !errors.As(e, &api) || api.Status != status {
		t.Fatalf("status %d expected, got %v", status, e)
	}
}
func poolPairingMoveTime(t *testing.T, f *fixture, p PoolRunnerPairing, created time.Time) {
	t.Helper()
	p.CreatedAt = poolPairingStamp(created)
	p.ExpiresAt = poolPairingStamp(created.Add(10 * time.Minute))
	if _, e := f.s.DB.Exec(`UPDATE suma_pool_runner_pairings SET created_at=?,expires_at=?,payload=? WHERE id=?`, p.CreatedAt, p.ExpiresAt, stringify(p), p.ID); e != nil {
		t.Fatal(e)
	}
}

func TestPoolRunnerPairingHTTPOriginalDigestAndApproval(t *testing.T) {
	f := newFixture(t)
	pool := seedCredentialPool(t, f)
	f.handler = (&Server{Service: f.s, Engine: f.e, PublicURL: pairingTestControl}).Handler()
	request := pc.PoolRunnerPairingCreateRequest{PoolID: pool, Name: "Owned Pool machine", MaxSlots: 2, TTLSeconds: 3600, IdempotencyKey: newID()}
	route := "/execution-pools/" + pool + "/runner-pairings"
	created := f.call("POST", route, request, 201)
	id, _ := created["id"].(string)
	if id == "" || created["purpose"] != "pool_runner" || created["owner_tenant_id"] != f.user.TenantID || created["state"] != "waiting" {
		t.Fatal(created)
	}
	replay := f.call("POST", route, request, 201)
	if replay["id"] != id {
		t.Fatal("lost create ACK made another pairing")
	}
	token := "suma_pool_" + newID() + newID()
	proposal := pc.PoolRunnerPairingProposalRequest{PoolID: pool, DeviceName: "Owned remote machine", CredentialSHA256: digest(token), IdempotencyKey: newID()}
	proposed := f.bearer("POST", "/pool-runner-pairings/"+id+"/proposal", token, proposal, 200)
	if proposed["state"] != "proposed" || proposed["can_approve"] != false {
		t.Fatal(proposed)
	}
	approval := pc.PoolRunnerPairingCommandRequest{PoolID: pool, Action: "approve", ExpectedVersion: int(proposed["version"].(float64)), IdempotencyKey: newID()}
	approved := f.call("POST", route+"/"+id+"/commands", approval, 200)
	if approved["state"] != "approved" || approved["runner_id"] == "" || approved["worker_id"] != "pool-"+approved["runner_id"].(string) {
		t.Fatal(approved)
	}
	duplicate := f.call("POST", route+"/"+id+"/commands", approval, 200)
	if duplicate["runner_id"] != approved["runner_id"] {
		t.Fatal("lost approve ACK made another runner")
	}
	status := pc.PoolRunnerPairingStatusRequest{PoolID: pool, IdempotencyKey: proposal.IdempotencyKey}
	head := f.bearer("POST", "/pool-runner-pairings/"+id+"/status", token, status, 200)
	if head["runner_id"] != approved["runner_id"] || head["can_approve"] != false {
		t.Fatal(head)
	}
	if strings.Contains(stringify(approved), token) || strings.Contains(stringify(created), token) {
		t.Fatal("pairing exposed credential")
	}
	session, err := f.s.GetExecutionPoolRunnerSession(context.Background(), token, approved["runner_id"].(string))
	if err != nil || session.Runner.MaxSlots != 2 || len(session.Scopes) != 0 {
		t.Fatal("original Pool identity not registered independently", session, err)
	}
	var credentials, projectCredentials, grants, runs int
	for query, target := range map[string]*int{`SELECT COUNT(*) FROM suma_execution_pool_credentials WHERE token_hash=?`: &credentials, `SELECT COUNT(*) FROM suma_credential_authority WHERE token_hash=?`: &projectCredentials} {
		if err = f.s.DB.QueryRow(query, digest(token)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_execution_pool_grants`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if credentials != 1 || projectCredentials != 0 || grants != 0 || runs != 0 {
		t.Fatal("pairing expanded project/dispatch authority", credentials, projectCredentials, grants, runs)
	}
	raw, _ := json.Marshal(request)
	var invalid map[string]any
	_ = json.Unmarshal(raw, &invalid)
	invalid["max_slots"] = nil
	f.call("POST", route, invalid, 400)
}

func TestPoolRunnerPairingCurrentAuthorityAndImmutableBasis(t *testing.T) {
	f, pool := poolPairingFixture(t)
	ctx := context.Background()
	p, token, op := poolPairingProposed(t, f, pool)
	route := "/execution-pools/" + pool + "/runner-pairings/" + p.ID
	other := PoolRunnerPairingProposalRequest{PoolID: pool, DeviceName: "Other node", CredentialSHA256: digest(strings.Repeat("b", 64)), IdempotencyKey: newID()}
	_, e := f.s.ProposePoolRunnerPairing(ctx, p.ID, other, strings.Repeat("b", 64), pairingTestControl)
	poolPairingWantStatus(t, e, 409)
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, PoolRunnerPairingStatusRequest{PoolID: pool, IdempotencyKey: newID()}, token, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, PoolRunnerPairingStatusRequest{PoolID: newID(), IdempotencyKey: op}, token, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	approve := PoolRunnerPairingCommandRequest{PoolID: pool, Action: "approve", ExpectedVersion: 1, IdempotencyKey: newID()}
	_, e = f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, approve, pairingTestControl)
	poolPairingWantStatus(t, e, 409)
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pools SET version=version+1,status='paused' WHERE id=?`, pool); e != nil {
		t.Fatal(e)
	}
	view := f.call("GET", route, nil, 200)
	if view["can_approve"] != false || view["reason_code"] != "pool_inactive" {
		t.Fatal(view)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pools SET status='active' WHERE id=?`, pool); e != nil {
		t.Fatal(e)
	}
	view = f.call("GET", route, nil, 200)
	if view["reason_code"] != "pool_changed" {
		t.Fatal(view)
	}
	approve.ExpectedVersion = p.Version
	_, e = f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, approve, pairingTestControl)
	poolPairingWantStatus(t, e, 409)
	approve.Action = "reject"
	rejected, e := f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, approve, pairingTestControl+"/changed")
	if e != nil || rejected.State != "rejected" {
		t.Fatal(rejected, e)
	}
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, PoolRunnerPairingStatusRequest{PoolID: pool, IdempotencyKey: op}, token, pairingTestControl)
	poolPairingWantStatus(t, e, 409)
	api, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	f.bearer("GET", route, api.Token, nil, 403)
	if _, e = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, approve, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	f.call("GET", "/execution-pools/"+pool+"/runner-pairings?limit=0&extra=bad", nil, 403)
	if domainACount(t, f.s, "suma_execution_pool_runners") != 0 || domainACount(t, f.s, "suma_execution_pool_grants") != 0 {
		t.Fatal("rejected or unauthorized pairing created execution authority")
	}
}

func TestPoolRunnerPairingAtomicApprovalAndCurrentMachineCredential(t *testing.T) {
	f, pool := poolPairingFixture(t)
	ctx := context.Background()
	p, token, op := poolPairingProposed(t, f, pool)
	if _, e := f.s.DB.Exec(`CREATE TRIGGER pool_pairing_failure BEFORE INSERT ON suma_activity WHEN NEW.kind='pool_runner_pairing.approved' BEGIN SELECT RAISE(ABORT,'owned atomic failure'); END`); e != nil {
		t.Fatal(e)
	}
	command := PoolRunnerPairingCommandRequest{PoolID: pool, Action: "approve", ExpectedVersion: p.Version, IdempotencyKey: newID()}
	_, e := f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, command, pairingTestControl)
	if e == nil {
		t.Fatal("trigger failure committed")
	}
	if domainACount(t, f.s, "suma_execution_pool_runners") != 0 || domainACount(t, f.s, "suma_execution_pool_credentials") != 0 {
		t.Fatal("partial registration survived rollback")
	}
	head, e := f.s.GetPoolRunnerPairing(ctx, f.actor, pool, p.ID, pairingTestControl)
	if e != nil || head.State != "proposed" || head.Version != p.Version {
		t.Fatal(head, e)
	}
	if _, e = f.s.DB.Exec(`DROP TRIGGER pool_pairing_failure`); e != nil {
		t.Fatal(e)
	}
	approved, e := f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, command, pairingTestControl)
	if e != nil {
		t.Fatal(e)
	}
	poolPairingMoveTime(t, f, approved, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	status := PoolRunnerPairingStatusRequest{PoolID: pool, IdempotencyKey: op}
	if _, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, status, token, pairingTestControl); e != nil {
		t.Fatal("pairing TTL revoked approved identity", e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pools SET status='paused' WHERE id=?`, pool); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, status, token, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pools SET status='active' WHERE id=?`, pool); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET expires_at='2000-01-01T00:00:00Z' WHERE runner_id=?`, approved.RunnerID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, status, token, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET expires_at=? WHERE runner_id=?`, approved.CredentialExpiresAt, approved.RunnerID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_runners SET status='revoked' WHERE id=?`, approved.RunnerID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, status, token, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_runners SET status='active' WHERE id=?`, approved.RunnerID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET version=version+1 WHERE runner_id=?`, approved.RunnerID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.PoolRunnerPairingStatus(ctx, p.ID, status, token, pairingTestControl)
	poolPairingWantStatus(t, e, 403)
	history, e := f.s.GetPoolRunnerPairing(ctx, f.actor, pool, p.ID, pairingTestControl)
	if e != nil || history.State != "approved" || history.CanApprove || history.ReasonCode != "credential_unavailable" {
		t.Fatal(history, e)
	}
	replay, e := f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, command, pairingTestControl)
	if e != nil || replay.RunnerID != approved.RunnerID || replay.CanApprove {
		t.Fatal("original approval receipt was not recoverable", replay, e)
	}
	if e = logoutCredentialForTest(ctx, f.s, f.cookie.Value); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, command, pairingTestControl)
	poolPairingWantStatus(t, e, 401)
}

func TestPoolRunnerPairingConcurrentApprovalAndInboxCap(t *testing.T) {
	f, pool := poolPairingFixture(t)
	ctx := context.Background()
	p, _, _ := poolPairingProposed(t, f, pool)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, e := f.s.CommandPoolRunnerPairing(ctx, f.actor, p.ID, PoolRunnerPairingCommandRequest{PoolID: pool, Action: "approve", ExpectedVersion: p.Version, IdempotencyKey: newID()}, pairingTestControl)
			results <- e
		}()
	}
	first, second := <-results, <-results
	if first == nil && second == nil || first != nil && second != nil {
		t.Fatal("concurrent approval must have one winner", first, second)
	}
	if first != nil {
		poolPairingWantStatus(t, first, 409)
	} else {
		poolPairingWantStatus(t, second, 409)
	}
	if domainACount(t, f.s, "suma_execution_pool_runners") != 1 || domainACount(t, f.s, "suma_execution_pool_credentials") != 1 {
		t.Fatal("concurrent approval duplicated identity")
	}
	for i := 0; i < 7; i++ {
		poolPairingCreate(t, f, pool, "Pending machine")
	}
	creates := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, e := f.s.CreatePoolRunnerPairing(ctx, f.actor, PoolRunnerPairingCreateRequest{PoolID: pool, Name: "Last slot", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()}, pairingTestControl)
			creates <- e
		}()
	}
	first, second = <-creates, <-creates
	if first == nil && second == nil || first != nil && second != nil {
		t.Fatal("active pairing cap race", first, second)
	}
	if first != nil {
		poolPairingWantStatus(t, first, 409)
	} else {
		poolPairingWantStatus(t, second, 409)
	}
	if domainACount(t, f.s, "suma_pool_runner_pairings") != 9 {
		t.Fatal("active inbox cap was not eight alongside approved history")
	}
}

func TestPoolRunnerPairingInboxCursorExpiryAndStrictWire(t *testing.T) {
	f, pool := poolPairingFixture(t)
	a := poolPairingCreate(t, f, pool, "Alpha machine")
	poolPairingCreate(t, f, pool, "Beta machine")
	poolPairingCreate(t, f, pool, "中文机器")
	route := "/execution-pools/" + pool + "/runner-pairings"
	page := f.call("GET", route+"?status=all&limit=1", nil, 200)
	cursor, _ := page["next_cursor"].(string)
	if cursor == "" || page["total"] != float64(3) {
		t.Fatal(page)
	}
	next := f.call("GET", route+"?status=all&limit=1&cursor="+url.QueryEscape(cursor), nil, 200)
	if next["as_of"] != page["as_of"] || next["items"].([]any)[0].(map[string]any)["id"] == page["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("cursor did not retain first watermark or seek", next)
	}
	f.call("GET", route+"?status=active&limit=1&cursor="+url.QueryEscape(cursor), nil, 400)
	otherPool := seedCredentialPool(t, f)
	f.call("GET", "/execution-pools/"+otherPool+"/runner-pairings?status=all&limit=1&cursor="+url.QueryEscape(cursor), nil, 400)
	found := f.call("GET", route+"?q="+url.QueryEscape("中文"), nil, 200)
	if found["total"] != float64(1) {
		t.Fatal(found)
	}
	clock, e := f.s.authorityNow(context.Background(), f.s.DB)
	if e != nil {
		t.Fatal(e)
	}
	poolPairingMoveTime(t, f, a, clock.Add(-11*time.Minute))
	expired := f.call("GET", route+"?status=expired", nil, 200)
	if expired["total"] != float64(1) || expired["items"].([]any)[0].(map[string]any)["state"] != "expired" {
		t.Fatal(expired)
	}
	for _, raw := range []string{`{"pool_id":"` + pool + `","name":"x","max_slots":1,"ttl_seconds":3600}`, `{"pool_id":"` + pool + `","name":"x","max_slots":null,"ttl_seconds":3600,"idempotency_key":"strict-operation"}`, `{"pool_id":"` + pool + `","pool_id":"` + pool + `","name":"x","max_slots":1,"ttl_seconds":3600,"idempotency_key":"strict-operation"}`, `{"pool_id":"` + pool + `","name":"x","max_slots":1,"ttl_seconds":3600,"idempotency_key":"strict-operation","token":"injected"}`} {
		req := httptest.NewRequest("POST", "http://suma.test/api/v1"+route, bytes.NewBufferString(raw))
		req.AddCookie(f.cookie)
		req.Header.Set("X-Suma-Expected-User", f.user.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		req.Header.Set("Accept-Language", "en-US")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "pool.runner_pairing.invalid") || strings.Contains(w.Body.String(), "资源池") {
			t.Fatal("strict multilingual wire rejected incorrectly", w.Code, w.Body.String())
		}
	}
	machine := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/pool-runner-pairings/"+a.ID+"/status", strings.NewReader(`{"pool_id":"`+pool+`","idempotency_key":"strict-operation"}`))
	machine.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, machine)
	if w.Code != 401 {
		t.Fatal("browser cookie replaced narrow machine proof", w.Code)
	}
	f.call("GET", route+"/"+a.ID+"?pool_id="+pool, nil, 400)
	f.call("GET", route+"?limit=0", nil, 400)
}

func TestPoolRunnerPairingRejectsUnknownInitializationLayout(t *testing.T) {
	db, e := store.Open("sqlite", filepath.Join(t.TempDir(), "unknown-pool-pairing.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(`CREATE TABLE suma_pool_runner_pairings(id TEXT PRIMARY KEY,payload TEXT)`); e != nil {
		t.Fatal(e)
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if e = s.Initialize(context.Background()); !errors.Is(e, store.ErrInitialSchema) {
		t.Fatal("unregistered existing schema was accepted", e)
	}
	var n int
	if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='suma_projects'`).Scan(&n); e != nil || n != 0 {
		t.Fatal("schema validation wrote baseline tables first", n, e)
	}
}
