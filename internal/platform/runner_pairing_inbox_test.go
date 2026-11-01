package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Controlled durable row fixtures exercise collection semantics, not credential
// issuance, a real machine or current qualification of historical approvals.
func seedPairingInbox(t *testing.T, f *fixture, count int) []RunnerPairing {
	t.Helper()
	items := make([]RunnerPairing, 0, count)
	for i := 0; i < count; i++ {
		p := RunnerPairing{ID: fmt.Sprintf("%032x", i+1), ProjectID: f.project, Version: 1, State: "rejected", ControlURL: pairingTestControl, CreatedAt: time.Date(2026, 1, 1, 0, i/2, 0, 0, time.UTC).Format(time.RFC3339), ExpiresAt: "2099-01-01T00:00:00Z", DeviceName: fmt.Sprintf("Device %d", i)}
		switch i {
		case 0:
			p.State = "waiting"
		case 1:
			p.State = "proposed"
			p.Version = 2
		case 2:
			p.State = "waiting"
			p.ExpiresAt = "2000-01-01T00:00:00Z"
		case 3:
			p.State = "approved"
			p.Version = 3
			p.WorkerID = "remote-" + p.ID
			p.CredentialExpiresAt = "2099-01-02T00:00:00Z"
		case 4:
			p.DeviceName = "Literal %_! needle"
		}
		if _, err := f.s.DB.Exec(`INSERT INTO suma_runner_pairings(id,tenant_id,project_id,version,state,expires_at,proposal_operation_id,proposal_sha256,payload)VALUES(?,?,?,?,?,?,'','',?)`, p.ID, f.user.TenantID, p.ProjectID, p.Version, p.State, p.ExpiresAt, stringify(p)); err != nil {
			t.Fatal(err)
		}
		items = append(items, p)
	}
	return items
}

func pairingInbox(t *testing.T, f *fixture, values url.Values, want int) map[string]any {
	t.Helper()
	if values == nil {
		values = url.Values{}
	}
	values.Set("project_id", f.project)
	return f.call(http.MethodGet, "/runner-pairings?"+values.Encode(), nil, want)
}

func TestRunnerPairingInboxLivePaginationAndLiteralFilters(t *testing.T) {
	f := pairingFixture(t)
	items := seedPairingInbox(t, f, 25)
	first := pairingInbox(t, f, url.Values{"status": {"all"}}, 200)
	if first["total"] != float64(25) || first["limit"] != float64(20) || len(first["items"].([]any)) != 20 || first["as_of"] == "" {
		t.Fatal("collection confused full total with page length", first)
	}
	seen := map[string]bool{}
	for i, row := range first["items"].([]any) {
		p := row.(map[string]any)
		if p["id"] != items[24-i].ID || p["reason_code"] == nil {
			t.Fatal("created/id order or browser projection lost", p)
		}
		seen[p["id"].(string)] = true
	}
	second := pairingInbox(t, f, url.Values{"status": {"all"}, "cursor": {first["next_cursor"].(string)}}, 200)
	if second["total"] != float64(25) || len(second["items"].([]any)) != 5 || second["next_cursor"] != nil {
		t.Fatal("continuation changed full count or end marker", second)
	}
	for _, row := range second["items"].([]any) {
		id := row.(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatal("duplicate seek result", id)
		}
		seen[id] = true
	}
	for _, test := range []struct {
		status string
		total  int
	}{{"active", 2}, {"waiting", 1}, {"proposed", 1}, {"approved", 1}, {"rejected", 21}, {"expired", 1}, {"all", 25}} {
		page := pairingInbox(t, f, url.Values{"status": {test.status}, "limit": {"100"}}, 200)
		if page["total"] != float64(test.total) || len(page["items"].([]any)) != test.total {
			t.Fatal("effective-state filter mismatch", test.status, page)
		}
	}
	for _, test := range []struct{ q, wantID string }{{"  %_!  ", items[4].ID}, {items[8].ID, items[8].ID}, {items[3].WorkerID, items[3].ID}} {
		page := pairingInbox(t, f, url.Values{"status": {"all"}, "q": {test.q}}, 200)
		if page["q"] != strings.TrimSpace(test.q) || page["total"] != float64(1) || page["items"].([]any)[0].(map[string]any)["id"] != test.wantID {
			t.Fatal("search treated literal text as patterns or lost scope", page)
		}
	}
	// Each read re-evaluates the database deadline instead of fixing cursor time.
	if _, err := f.s.DB.Exec(`UPDATE suma_runner_pairings SET expires_at='2000-01-01T00:00:00Z',payload=json_set(payload,'$.expires_at','2000-01-01T00:00:00Z') WHERE id=?`, items[1].ID); err != nil {
		t.Fatal(err)
	}
	if current := pairingInbox(t, f, nil, 200); current["status"] != "active" || current["total"] != float64(1) {
		t.Fatal("live page retained expired state", current)
	}
}

func TestRunnerPairingInboxCursorAndAuthorityStayInCurrentScope(t *testing.T) {
	f := pairingFixture(t)
	seedPairingInbox(t, f, 3)
	page := pairingInbox(t, f, url.Values{"status": {"all"}, "limit": {"1"}}, 200)
	cursor := page["next_cursor"].(string)
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	padded := base64.RawURLEncoding.EncodeToString(append(decoded, bytes.Repeat([]byte(" "), 3072-len(decoded))...))
	if len(padded) != 4096 {
		t.Fatal("invalid maximum cursor fixture", len(padded))
	}
	pairingInbox(t, f, url.Values{"status": {"all"}, "limit": {"1"}, "cursor": {padded}}, 200)
	for _, values := range []url.Values{{"status": {"active"}, "limit": {"1"}, "cursor": {cursor}}, {"status": {"all"}, "limit": {"2"}, "cursor": {cursor}}, {"status": {"all"}, "limit": {"1"}, "q": {"different"}, "cursor": {cursor}}, {"cursor": {"not-base64"}}, {"cursor": {strings.Repeat("x", 4097)}}, {"status": {"unknown"}}, {"limit": {"101"}}, {"limit": {"0"}}, {"q": {strings.Repeat("界", 67)}}} {
		pairingInbox(t, f, values, 400)
	}
	f.call("GET", "/runner-pairings?project_id="+f.project+"&project_id="+f.project, nil, 400)
	f.call("GET", "/runner-pairings?project_id="+f.project+"&unknown=x", nil, 400)
	other := f.call("POST", "/projects", f.body(map[string]any{"title": "Other pairing inbox"}), 201)
	f.call("GET", "/runner-pairings?"+url.Values{"project_id": {other["id"].(string)}, "status": {"all"}, "limit": {"1"}, "cursor": {cursor}}.Encode(), nil, 400)
	api, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("GET", "/runner-pairings?project_id="+f.project, api.Token, nil, 403)
	invite := governanceInvite(f, "pairing-inbox-admin@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "admin"}})
	governanceAccept(f, invite, "pairing-inbox-admin@example.test")
	pairingInbox(t, f, url.Values{"status": {"all"}, "limit": {"1"}, "cursor": {cursor}}, 400)
	var current Principal
	session := f.call("GET", "/auth/session", nil, 200)
	raw, _ := json.Marshal(session["user"])
	if err = json.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, current.ID); err != nil {
		t.Fatal(err)
	}
	// Permission rejection wins over interpreting the malformed cursor.
	pairingInbox(t, f, url.Values{"cursor": {"not-base64"}}, 403)
}

func TestRunnerPairingBrowserReasonsPermitOnlyOriginalPendingRejectionAfterControlDrift(t *testing.T) {
	f, token, key := pairingFixture(t), strings.Repeat("b", 64), newID()
	p := pairingCreate(t, f)
	if p["reason_code"] != nil {
		t.Fatal("create ACK carried a current reason", p)
	}
	waiting := f.call("GET", "/runner-pairings/"+p["id"].(string)+queryProject(f.project), nil, 200)
	if waiting["reason_code"] != "awaiting_machine" {
		t.Fatal(waiting)
	}
	proposal := pairingPropose(t, f, p, token, key, 200)
	if proposal["reason_code"] != nil {
		t.Fatal("machine proposal received browser reason", proposal)
	}
	if status := pairingStatus(t, f, p, token, key, 200); status["reason_code"] != nil {
		t.Fatal("machine status received browser reason", status)
	}
	proposed := f.call("GET", "/runner-pairings/"+p["id"].(string)+queryProject(f.project), nil, 200)
	if proposed["reason_code"] != "approval_required" || proposed["can_approve"] != true {
		t.Fatal(proposed)
	}
	f.handler = (&Server{Service: f.s, Engine: f.e, PublicURL: pairingTestControl + "/changed"}).Handler()
	view := f.call("GET", "/runner-pairings/"+p["id"].(string)+queryProject(f.project), nil, 200)
	if view["reason_code"] != "control_changed" || view["can_approve"] != false {
		t.Fatal(view)
	}
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", f.body(map[string]any{"expected_version": 2, "action": "approve"}), 409)
	f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", f.body(map[string]any{"expected_version": 1, "action": "reject"}), 409)
	reject := f.body(map[string]any{"expected_version": 2, "action": "reject"})
	ack := f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", reject, 200)
	if ack["state"] != "rejected" || ack["reason_code"] != nil {
		t.Fatal("rejection changed identity or saved a view reason", ack)
	}
	if again := f.call("POST", "/runner-pairings/"+p["id"].(string)+"/commands", reject, 200); stringify(again) != stringify(ack) {
		t.Fatal("rejection recovery changed its historical ACK", again)
	}
	var payload string
	if err := f.s.DB.QueryRow(`SELECT payload FROM suma_runner_pairings WHERE id=?`, p["id"]).Scan(&payload); err != nil || strings.Contains(payload, "reason_code") {
		t.Fatal("current reason was stored", payload, err)
	}
	if domainACount(t, f.s, "suma_release_workers") != 0 || domainACount(t, f.s, "suma_credential_grants") != 0 {
		t.Fatal("closing intent issued a Runner")
	}
}

func TestRunnerPairingInboxRejectsMalformedSourceAndPreservesIndexOnRestart(t *testing.T) {
	f := pairingFixture(t)
	seedPairingInbox(t, f, 1)
	if err := f.s.InitializeRunnerPairings(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.DB.Query(`SELECT name FROM pragma_index_info('suma_runner_pairing_scope_state') ORDER BY seqno`)
	if err != nil {
		t.Fatal(err)
	}
	var columns []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	rows.Close()
	if strings.Join(columns, ",") != "tenant_id,project_id,state,expires_at" {
		t.Fatal("initializer did not retain exact scoped-cap index", columns)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_runner_pairings SET payload='not-json'`); err != nil {
		t.Fatal(err)
	}
	pairingInbox(t, f, url.Values{"status": {"all"}}, 500)
}
