package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestAutomationIngressCASRotationReceiptsAndGlobalSecretTombstones(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	oldSecret := strings.Repeat("a", 64)
	in := ingressTestEntry(t, f, a, "github", oldSecret)
	ctx := context.Background()
	newSecret := strings.Repeat("b", 64)
	req := AutomationIngressCommandRequest{ProjectID: f.project, Action: "rotate_key", ExpectedVersion: in.Version, IdempotencyKey: newID(), Secret: newSecret}
	rotated, err := f.s.CommandAutomationIngress(ctx, f.actor, f.project, in.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.State != "paused" || rotated.KeyGeneration != 2 || rotated.Revision != 1 {
		t.Fatal("rotation did not require explicit re-enable", rotated)
	}
	again, err := f.s.CommandAutomationIngress(ctx, f.actor, f.project, in.ID, req)
	if err != nil || again.Version != rotated.Version {
		t.Fatal("rotation receipt failed to recover", err)
	}
	altered := req
	altered.Secret = strings.Repeat("c", 64)
	if _, err = f.s.CommandAutomationIngress(ctx, f.actor, f.project, in.ID, altered); err == nil {
		t.Fatal("operation key accepted different secret")
	}
	public, _ := json.Marshal(again)
	if strings.Contains(string(public), newSecret) {
		t.Fatal("rotation returned secret")
	}
	if _, err = f.s.CommandAutomationIngress(ctx, f.actor, f.project, in.ID, AutomationIngressCommandRequest{Action: "enable", ExpectedVersion: rotated.Version, ExpectedRuleVersion: a.Version + 1, ExpectedRuleRevision: a.Revision, IdempotencyKey: newID()}); err == nil {
		t.Fatal("stale rule CAS accepted")
	}
	in = ingressTestCommand(t, f, rotated, AutomationIngressCommandRequest{Action: "enable", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision})
	raw := ingressPush(strings.Repeat("a", 40))
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, oldSecret, raw, "push", "delivery-old-key", 0), 401)
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, newSecret, raw, "push", "delivery-new-key", 0), 202)
	second, err := f.s.CreateAutomationIngress(ctx, f.actor, f.project, a.ID, AutomationIngressCreateRequest{ProjectID: f.project, ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, IdempotencyKey: newID(), Spec: in.AutomationIngressSpec})
	if err != nil {
		t.Fatal(err)
	}
	for _, used := range []string{oldSecret, newSecret} {
		if _, err = f.s.CommandAutomationIngress(ctx, f.actor, f.project, second.ID, AutomationIngressCommandRequest{Action: "rotate_key", ExpectedVersion: second.Version, IdempotencyKey: newID(), Secret: used}); err == nil {
			t.Fatal("used secret moved to another ingress")
		}
	}
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "revoke"})
	if in.KeyConfigured {
		t.Fatal("revocation did not erase ciphertext")
	}
	if _, err = f.s.CommandAutomationIngress(ctx, f.actor, f.project, second.ID, AutomationIngressCommandRequest{Action: "rotate_key", ExpectedVersion: second.Version, IdempotencyKey: newID(), Secret: newSecret}); err == nil {
		t.Fatal("revocation released secret tombstone")
	}
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, newSecret, raw, "push", "delivery-revoked-key", 0), 401)
}

func TestAutomationIngressPauseBlocksOnlyOwnQueuedAndRetryNeverRevives(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret1, secret2 := strings.Repeat("c", 64), strings.Repeat("d", 64)
	one := ingressTestEntry(t, f, a, "github", secret1)
	two := ingressTestEntry(t, f, a, "github", secret2)
	raw := ingressPush(strings.Repeat("a", 40))
	receipt := ingressTestPost(t, f, one, raw, ingressTestHeaders(one, secret1, raw, "push", "delivery-paused", 0), 202)
	ingressTestPost(t, f, two, raw, ingressTestHeaders(two, secret2, raw, "push", "delivery-other-entry", 0), 202)
	a = autoCommand(t, f, a, "trigger").Rule
	one = ingressTestCommand(t, f, one, AutomationIngressCommandRequest{Action: "pause"})
	autoTick(t, f)
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 2 {
		t.Fatal("pause blocked unrelated source or dispatched paused input")
	}
	d, err := f.s.GetAutomationIngressDelivery(context.Background(), f.actor, f.project, one.ID, receipt.ID)
	if err != nil || d.OccurrenceState != "blocked" || d.RunID != "" {
		t.Fatal("paused queued occurrence escaped", err, d)
	}
	one = ingressTestCommand(t, f, one, AutomationIngressCommandRequest{Action: "enable", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision})
	again := ingressTestPost(t, f, one, raw, ingressTestHeaders(one, secret1, raw, "push", "delivery-paused", 0), 200)
	if again.ID != receipt.ID {
		t.Fatal("old receipt changed after re-enable")
	}
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 2 {
		t.Fatal("retry resurrected paused delivery")
	}
}

func TestAutomationIngressRevisionDoesNotReevaluateOldDeliveryAndRuleCASMustRebind(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("e", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	raw := ingressPush(strings.Repeat("a", 40))
	r := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-old-revision", 0), 202)
	spec := a.AutomationSpec
	spec.Name = "New rule revision"
	out, err := f.s.CommandAutomation(context.Background(), f.actor, f.project, a.ID, AutomationCommandRequest{Action: "revise", ExpectedVersion: a.Version, IdempotencyKey: newID(), Spec: &spec})
	if err != nil {
		t.Fatal(err)
	}
	a = autoCommand(t, f, out.Rule, "enable").Rule
	if _, err = f.s.CommandAutomationIngress(context.Background(), f.actor, f.project, in.ID, AutomationIngressCommandRequest{Action: "enable", ExpectedVersion: in.Version, ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, IdempotencyKey: newID()}); err == nil {
		t.Fatal("entry silently rebound to rule revision")
	}
	entrySpec := in.AutomationIngressSpec
	entrySpec.Name = "Rebound input"
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "revise", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, Spec: &entrySpec})
	if in.State != "paused" || in.Revision != 2 || in.RuleRevision != 2 {
		t.Fatal("revision did not pause and bind", in)
	}
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "enable", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision})
	again := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-old-revision", 0), 200)
	if again.ID != r.ID {
		t.Fatal("spec revision changed receipt identity")
	}
	autoTick(t, f)
	if autoCount(t, f, "dag_runs") != 0 {
		t.Fatal("old revision delivery was re-evaluated")
	}
	history, err := f.s.AutomationIngressRevisions(context.Background(), f.actor, f.project, in.ID, 1, "")
	if err != nil || history.Total != 2 || history.Items[0].Revision != 2 || history.NextCursor == "" {
		t.Fatal("revision page unavailable", err)
	}
	older, err := f.s.AutomationIngressRevisions(context.Background(), f.actor, f.project, in.ID, 1, history.NextCursor)
	if err != nil || older.Items[0].Spec.Name != "Signed input" || older.Items[0].RuleRevision != 1 {
		t.Fatal("immutable history mutated", err)
	}
	if _, err = f.s.AutomationIngressDeliveries(context.Background(), f.actor, f.project, in.ID, 1, history.NextCursor); err == nil {
		t.Fatal("history cursor crossed collections")
	}
}

func ingressReviewer(t *testing.T, f *fixture) Actor {
	t.Helper()
	ctx := context.Background()
	email := newID() + "@example.test"
	in, err := f.s.CreateInvitation(ctx, f.actor, map[string]any{"email": email, "role": "member", "grants": []any{map[string]any{"project_id": f.project, "role": "reviewer"}}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.s.AcceptInvitation(ctx, map[string]any{"email": email, "name": "Reviewer", "password": "real-ingress-reviewer-password-42", "token": in["token"], "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	return actor
}

func TestAutomationIngressDispatchRechecksDistinctEntryActivatorAndRevokedReplay(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("f", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	reviewer := ingressReviewer(t, f)
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "pause"})
	req := AutomationIngressCommandRequest{Action: "enable", ExpectedVersion: in.Version, ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, IdempotencyKey: newID()}
	var err error
	in, err = f.s.CommandAutomationIngress(context.Background(), reviewer, f.project, in.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	raw := ingressPush(strings.Repeat("a", 40))
	r := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-authorized-before-revoke", 0), 202)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, reviewer.Projection().ID); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	d, err := f.s.GetAutomationIngressDelivery(context.Background(), f.actor, f.project, in.ID, r.ID)
	if err != nil || d.OccurrenceState != "blocked" || autoCount(t, f, "dag_runs") != 0 {
		t.Fatal("entry activator revocation bypassed dispatch", err, d)
	}
	changed := ingressPush(strings.Repeat("b", 40))
	blocked := ingressTestPost(t, f, in, changed, ingressTestHeaders(in, secret, changed, "push", "delivery-after-revoke", 0), 202)
	if blocked.State != "blocked" || blocked.OccurrenceID != "" {
		t.Fatal("revoked entry activator admitted new input")
	}
	if _, err = f.s.CommandAutomationIngress(context.Background(), reviewer, f.project, in.ID, req); err == nil {
		t.Fatal("revoked caller replayed privileged receipt")
	}
}

func TestAutomationIngressDeliveryAndOccurrenceRollbackTogether(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("a", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	if _, err := f.s.DB.Exec(`CREATE TRIGGER fail_ingress_delivery BEFORE INSERT ON suma_automation_ingress_deliveries BEGIN SELECT RAISE(ABORT,'forced delivery failure'); END`); err != nil {
		t.Fatal(err)
	}
	raw := ingressPush(strings.Repeat("a", 40))
	h := ingressTestHeaders(in, secret, raw, "push", "delivery-atomic", 0)
	ingressTestPost(t, f, in, raw, h, 500)
	for _, table := range []string{"suma_automation_occurrences", "suma_automation_ingress_deliveries", "suma_automation_ingress_delivery_ids", "suma_automation_ingress_github_bodies"} {
		if autoCount(t, f, table) != 0 {
			t.Fatal("failed transaction left partial intake", table)
		}
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER fail_ingress_delivery`); err != nil {
		t.Fatal(err)
	}
	ingressTestPost(t, f, in, raw, h, 202)
}

func TestAutomationIngressTwoConnectionsAndReopenPreserveSingleReceipt(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("b", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	ctx := context.Background()
	var seq int
	var name, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := &Service{DB: db, Dialect: "sqlite", AutomationIngressKey: append([]byte(nil), f.s.AutomationIngressKey...)}
	raw := ingressPush(strings.Repeat("a", 40))
	headers := ingressTestHeaders(in, secret, raw, "push", "delivery-competing", 0)
	var wg sync.WaitGroup
	receipts := make(chan AutomationIngressReceipt, 2)
	failures := make(chan error, 2)
	for _, s := range []*Service{f.s, other} {
		wg.Add(1)
		go func(s *Service) {
			defer wg.Done()
			r, err := s.ReceiveAutomationIngress(ctx, in.ID, headers, raw)
			receipts <- r
			failures <- err
		}(s)
	}
	wg.Wait()
	close(receipts)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	duplicates := 0
	for r := range receipts {
		if id != "" && id != r.ID {
			t.Fatal("connections created different receipts")
		}
		id = r.ID
		if r.Duplicate {
			duplicates++
		}
	}
	if duplicates != 1 || autoCount(t, f, "suma_automation_occurrences") != 1 {
		t.Fatal("concurrent intake duplicated occurrence")
	}
	reopened, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restart := &Service{DB: reopened, Dialect: "sqlite", AutomationIngressKey: append([]byte(nil), f.s.AutomationIngressKey...)}
	r, err := restart.ReceiveAutomationIngress(ctx, in.ID, headers, raw)
	if err != nil || !r.Duplicate || r.ID != id {
		t.Fatal("restart forgot replay tombstone", err)
	}
}

func TestAutomationIngressHTTPStrictFieldsScopesAndProjectIsolation(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	in := ingressTestEntry(t, f, a, "github", strings.Repeat("c", 64))
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "scope test", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		token := issued.Token
		r := httptest.NewRequest("GET", "http://suma.test/api/v1/automation-ingresses/"+in.ID+"?project_id="+f.project, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("scoped token accessed entry metadata", scope, w.Code)
		}
	}
	f.call("GET", "/automation-ingresses/"+in.ID+"?project_id=wrong-project", nil, 404)
	f.call("POST", "/automation-ingresses/"+in.ID+"/commands", f.body(map[string]any{"action": "pause", "expected_version": in.Version, "unknown": true}), 400)
	raw := []byte(`{"project_id":"` + f.project + `","action":"pause","action":"revoke","expected_version":` + strconv.Itoa(in.Version) + `,"idempotency_key":"duplicate-field-operation"}`)
	r := httptest.NewRequest("POST", "http://suma.test/api/v1/automation-ingresses/"+in.ID+"/commands", bytes.NewReader(raw))
	r.AddCookie(f.cookie)
	r.Header.Set("X-Suma-Expected-User", f.view.ID)
	r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("duplicate management field accepted", w.Code)
	}
}
