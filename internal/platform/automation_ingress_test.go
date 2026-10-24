package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestAutomationIngressHTTPCreateRequiresCurrentRule(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	body := f.body(map[string]any{"expected_rule_version": a.Version, "expected_rule_revision": a.Revision, "spec": map[string]any{"name": "Git input", "provider": "github", "events": []string{"push"}, "repository": "acme/repo", "repository_id": "42", "ref": "refs/heads/main", "conclusions": []string{}}})
	created := f.call("POST", "/automations/"+a.ID+"/ingresses", body, 201)
	if created["state"] != "paused" || created["key_configured"] != false || created["rule_revision"] != float64(a.Revision) {
		t.Fatal("entry was created without paused fixed binding", created)
	}
}

func ingressTestEntry(t *testing.T, f *fixture, a AutomationRule, provider, secret string) AutomationIngress {
	t.Helper()
	f.s.AutomationIngressKey = bytes.Repeat([]byte{0x42}, 32)
	spec := AutomationIngressSpec{Name: "Signed input", Provider: provider, Events: []string{"push"}, Repository: "acme/repo", RepositoryID: "42", Ref: "refs/heads/main", Conclusions: []string{}}
	if provider == "suma" {
		spec.Events = []string{"git.push", "ci.completed", "monitor.firing", "monitor.resolved"}
	}
	in, err := f.s.CreateAutomationIngress(context.Background(), f.user, f.project, a.ID, AutomationIngressCreateRequest{ProjectID: f.project, ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, IdempotencyKey: newID(), Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "rotate_key", Secret: secret})
	return ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "enable", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision})
}
func ingressTestCommand(t *testing.T, f *fixture, in AutomationIngress, req AutomationIngressCommandRequest) AutomationIngress {
	t.Helper()
	req.ProjectID = f.project
	req.ExpectedVersion = in.Version
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = newID()
	}
	out, err := f.s.CommandAutomationIngress(context.Background(), f.user, f.project, in.ID, req)
	if err != nil {
		t.Fatal(req.Action, err)
	}
	return out
}
func ingressTestHeaders(in AutomationIngress, secret string, raw []byte, event, delivery string, stamp int64) http.Header {
	h := http.Header{"Content-Type": []string{"application/json"}}
	message := raw
	if in.Provider == "github" {
		h.Set("X-GitHub-Delivery", delivery)
		h.Set("X-GitHub-Event", event)
	} else {
		h.Set("X-Suma-Delivery", delivery)
		h.Set("X-Suma-Timestamp", strconv.FormatInt(stamp, 10))
		message = append([]byte("suma-v1\n"+in.ID+"\n"+strconv.FormatInt(stamp, 10)+"\n"+delivery+"\n"), raw...)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if in.Provider == "github" {
		h.Set("X-Hub-Signature-256", signature)
	} else {
		h.Set("X-Suma-Signature-256", signature)
	}
	return h
}
func ingressTestPost(t *testing.T, f *fixture, in AutomationIngress, raw []byte, h http.Header, status int) AutomationIngressReceipt {
	t.Helper()
	req := httptest.NewRequest("POST", "http://suma.test"+in.EndpointPath, bytes.NewReader(raw))
	req.Header = h.Clone()
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != status {
		t.Fatalf("signed delivery status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var out AutomationIngressReceipt
	if status < 300 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}
func ingressPush(commit string) []byte {
	return []byte(`{"repository":{"id":42,"full_name":"Acme/Repo"},"ref":"refs/heads/main","after":"` + commit + `","deleted":false,"sender":{"private":"never-persist-payload"}}`)
}
func ingressSumaEvent(id string) []byte {
	return []byte(`{"schema_version":1,"event":"ci.completed","repository":"acme/repo","repository_id":"42","ref":"refs/heads/main","commit":"` + strings.Repeat("a", 40) + `","conclusion":"success","external_id":"` + id + `"}`)
}

func TestAutomationIngressGithubDeliveryAliasCannotChangeBody(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("a", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	raw := ingressPush(strings.Repeat("a", 40))
	first := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-original", 0), 202)
	alias := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-alias", 0), 200)
	if alias.ID != first.ID || !alias.Duplicate {
		t.Fatal("unsigned delivery header replay created a new receipt")
	}
	changed := ingressPush(strings.Repeat("b", 40))
	ingressTestPost(t, f, in, changed, ingressTestHeaders(in, secret, changed, "push", "delivery-alias", 0), 409)
}

func TestAutomationIngressQueuesFixedWorkflowAndStoresOnlyRestrictedFacts(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("b", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	if _, err := f.e.Publish(context.Background(), f.user.TenantID, f.project, dag.Workflow{ID: a.WorkflowID, Name: "Later workflow", Nodes: []dag.Node{{ID: "later", Type: "approval"}}}); err != nil {
		t.Fatal(err)
	}
	raw := ingressPush(strings.Repeat("a", 40))
	h := ingressTestHeaders(in, secret, raw, "push", "delivery-fixed-workflow", 0)
	receipt := ingressTestPost(t, f, in, raw, h, 202)
	if receipt.State != "accepted" || receipt.OccurrenceID == "" || autoCount(t, f, "dag_runs") != 0 {
		t.Fatal("input bypassed durable occurrence", receipt)
	}
	again := ingressTestPost(t, f, in, raw, h, 200)
	if !again.Duplicate || again.ID != receipt.ID {
		t.Fatal("retry changed receipt")
	}
	if autoCount(t, f, "suma_automation_occurrences") != 1 {
		t.Fatal("retry duplicated occurrence")
	}
	autoTick(t, f)
	delivery, err := f.s.GetAutomationIngressDelivery(context.Background(), f.user, f.project, in.ID, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.RunID == "" || delivery.OccurrenceState != "started" || delivery.Event.Commit != strings.Repeat("a", 40) {
		t.Fatal("delivery did not resolve real run", delivery)
	}
	run, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, delivery.RunID)
	if err != nil || run.WorkflowVersion != 1 || run.Nodes[0].ID != "gate" {
		t.Fatal("fixed workflow changed", err)
	}
	var ciphertext, spec, event, receiptRows string
	if err = f.s.DB.QueryRow(`SELECT key_ciphertext,spec FROM suma_automation_ingresses WHERE id=?`, in.ID).Scan(&ciphertext, &spec); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT event FROM suma_automation_ingress_deliveries WHERE id=?`, receipt.ID).Scan(&event); err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.DB.Query(`SELECT response FROM suma_operations`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r string
		if err = rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		receiptRows += r
	}
	rows.Close()
	public, _ := json.Marshal(delivery)
	entry, _ := json.Marshal(in)
	for _, value := range []string{ciphertext, spec, event, receiptRows, string(public), string(entry)} {
		if strings.Contains(value, secret) || strings.Contains(value, "never-persist-payload") {
			t.Fatal("secret or raw provider body escaped restricted storage")
		}
	}
	if ciphertext == "" || ciphertext == secret {
		t.Fatal("credential was not encrypted")
	}
}

func TestAutomationIngressRejectsBadSignaturesAndUnsafeHTTP(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("c", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	raw := ingressPush(strings.Repeat("a", 40))
	for _, tc := range []struct {
		name   string
		mutate func(http.Header)
		status int
	}{
		{"missing signature", func(h http.Header) { h.Del("X-Hub-Signature-256") }, 401},
		{"tampered signature", func(h http.Header) { h.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64)) }, 401},
		{"duplicate signature", func(h http.Header) { h.Add("X-Hub-Signature-256", h.Get("X-Hub-Signature-256")) }, 401},
		{"duplicate delivery", func(h http.Header) { h.Add("X-GitHub-Delivery", "another-delivery") }, 401},
		{"bearer", func(h http.Header) { h.Set("Authorization", "Bearer unused") }, 400},
		{"cookie", func(h http.Header) { h.Set("Cookie", "suma_session=unused") }, 400},
		{"origin", func(h http.Header) { h.Set("Origin", "http://suma.test") }, 400},
		{"encoding", func(h http.Header) { h.Set("Content-Encoding", "gzip") }, 400},
		{"not json", func(h http.Header) { h.Set("Content-Type", "text/plain") }, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := ingressTestHeaders(in, secret, raw, "push", "delivery-unsafe", 0)
			tc.mutate(h)
			ingressTestPost(t, f, in, raw, h, tc.status)
		})
	}
	if autoCount(t, f, "suma_automation_ingress_deliveries") != 0 {
		t.Fatal("invalid signature polluted delivery history")
	}
	h := ingressTestHeaders(in, secret, raw, "push", "delivery-tampered-body", 0)
	changed := append([]byte(nil), raw...)
	changed[len(changed)-3] ^= 1
	ingressTestPost(t, f, in, changed, h, 401)
	large := bytes.Repeat([]byte(" "), (2<<20)+1)
	ingressTestPost(t, f, in, large, ingressTestHeaders(in, secret, large, "push", "delivery-oversize", 0), 413)
	query := in
	query.EndpointPath += "?project_id=" + f.project
	ingressTestPost(t, f, query, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-query", 0), 400)
}

func TestAutomationIngressSumaTimestampBindingAndIndependentEvents(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("d", 64)
	in := ingressTestEntry(t, f, a, "suma", secret)
	raw := ingressSumaEvent("external-1")
	clock, err := f.s.automationClock(context.Background(), f.s.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int64{-600, 600} {
		ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "", "delivery-expired", clock.Unix()+offset), 401)
	}
	first := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "", "delivery-suma-1", clock.Unix()), 202)
	second := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "", "delivery-suma-2", clock.Unix()), 202)
	if first.ID == second.ID {
		t.Fatal("Suma independent same-body events were collapsed")
	}
	again := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "", "delivery-suma-1", clock.Unix()+1), 200)
	if again.ID != first.ID {
		t.Fatal("refreshed timestamp broke retry")
	}
	wrong := in
	wrong.ID = strings.Repeat("0", 32)
	ingressTestPost(t, f, in, raw, ingressTestHeaders(wrong, secret, raw, "", "delivery-cross-entry", clock.Unix()), 401)
	for _, bad := range []string{strings.Replace(string(raw), `"schema_version":1`, `"schema_version":2`, 1), strings.Replace(string(raw), `"schema_version":1,`, "", 1), strings.Replace(string(raw), `"schema_version":1`, `"schema_version":null`, 1), strings.Replace(string(raw), `"external_id":"external-1"`, `"external_id":""`, 1), strings.Replace(string(raw), `"event":"ci.completed"`, `"command":"danger","event":"ci.completed"`, 1), strings.Replace(string(raw), `"event":"ci.completed"`, `"event":"ci.completed","event":"git.push"`, 1)} {
		b := []byte(bad)
		ingressTestPost(t, f, in, b, ingressTestHeaders(in, secret, b, "", "delivery-invalid-suma", clock.Unix()), 400)
	}
}

func TestAutomationIngressConfiguredFiltersCannotMatchMissingEventFields(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("e", 64)
	in := ingressTestEntry(t, f, a, "suma", secret)
	clock, _ := f.s.automationClock(context.Background(), f.s.DB)
	for i, raw := range [][]byte{[]byte(`{"schema_version":1,"event":"monitor.firing","external_id":"alert-1"}`), []byte(strings.Replace(string(ingressSumaEvent("external-1")), "refs/heads/main", "refs/heads/other", 1))} {
		r := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "", fmt.Sprintf("delivery-filter-%d", i), clock.Unix()), 202)
		if r.State != "ignored" || r.OccurrenceID != "" {
			t.Fatal("configured filter bypassed by missing/mismatched field", r)
		}
	}
	if autoCount(t, f, "suma_automation_occurrences") != 0 {
		t.Fatal("filter mismatch created occurrence")
	}
}

func TestAutomationIngressOfficialGitHubMACVector(t *testing.T) {
	// Published GitHub vector, independently fixed rather than generated by our signer.
	if !verifyIngressMAC([]byte("It's a Secret to Everybody"), []byte("Hello, World!"), "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17") {
		t.Fatal("official GitHub signature vector rejected")
	}
}

func TestAutomationIngressUnsignedPingHeaderCannotPoisonPushReplay(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("f", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	raw := ingressPush(strings.Repeat("a", 40))
	h := ingressTestHeaders(in, secret, raw, "push", "delivery-header-poison", 0)
	h.Set("X-GitHub-Event", "ping")
	ingressTestPost(t, f, in, raw, h, 400)
	h.Set("X-GitHub-Event", "push")
	receipt := ingressTestPost(t, f, in, raw, h, 202)
	if receipt.State != "accepted" {
		t.Fatal("forged unsigned event header poisoned valid push")
	}
}

func TestAutomationIngressNewGithubAliasesConsumeRateBudgetButKnownRetryRecovers(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("a", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	raw := ingressPush(strings.Repeat("a", 40))
	for i := range 120 {
		status := 200
		if i == 0 {
			status = 202
		}
		ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", fmt.Sprintf("delivery-budget-%03d", i), 0), status)
	}
	// Keep this quota test independent of crossing a wall-clock minute while
	// issuing the 120 requests. The receiver still reads the real database clock.
	clock, err := f.s.automationClock(context.Background(), f.s.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_automation_ingress_delivery_ids SET received_at=? WHERE ingress_id=?`, automationStamp(clock), in.ID); err != nil {
		t.Fatal(err)
	}
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-budget-over-limit", 0), 429)
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-budget-000", 0), 200)
	if autoCount(t, f, "suma_automation_occurrences") != 1 {
		t.Fatal("alias flood duplicated occurrence")
	}
}

func TestAutomationIngressProviderFieldsHaveFiniteParsingBudget(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("b", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	raw := ingressPush(strings.Repeat("a", 40))
	var body strings.Builder
	body.Write(raw[:len(raw)-1])
	for i := range 17000 {
		fmt.Fprintf(&body, `,"field_%d":0`, i)
	}
	body.WriteByte('}')
	raw = []byte(body.String())
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-field-budget", 0), 400)
	if autoCount(t, f, "suma_automation_ingress_deliveries") != 0 {
		t.Fatal("over-budget parse created delivery")
	}
}

func TestAutomationIngressGithubCompletedPingAndDeleteSemantics(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("c", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	spec := in.AutomationIngressSpec
	spec.Events = []string{"push", "workflow_run.completed", "ping"}
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "revise", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, Spec: &spec})
	in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "enable", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision})
	for _, tc := range []struct{ name, event, body, want string }{
		{"workflow completed", "workflow_run", `{"action":"completed","repository":{"id":42,"full_name":"ACME/REPO"},"workflow_run":{"id":99,"head_branch":"main","head_sha":"` + strings.Repeat("a", 40) + `","conclusion":"success"}}`, "accepted"},
		{"workflow requested", "workflow_run", `{"action":"requested","repository":{"id":42,"full_name":"acme/repo"},"workflow_run":{"id":100,"head_branch":"main","head_sha":"` + strings.Repeat("a", 40) + `","conclusion":null}}`, "ignored"},
		{"ping", "ping", `{"zen":"Mind your words.","hook_id":100,"hook":{"id":100}}`, "ignored"},
		{"deleted branch", "push", strings.Replace(string(ingressPush(strings.Repeat("0", 40))), `"deleted":false`, `"deleted":true`, 1), "ignored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.body)
			r := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, tc.event, "delivery-"+strings.ReplaceAll(tc.name, " ", "-"), 0), 202)
			if r.State != tc.want {
				t.Fatal("provider semantics changed", r)
			}
		})
	}
	if autoCount(t, f, "suma_automation_occurrences") != 1 {
		t.Fatal("connection/deleted/incomplete event created occurrence")
	}
}

func TestAutomationIngressPublicConcurrencyBudgetRejectsBeforeIntake(t *testing.T) {
	f, a := automationFixture(t, "allow", "manual", "skip")
	a = autoCommand(t, f, a, "enable").Rule
	secret := strings.Repeat("d", 64)
	in := ingressTestEntry(t, f, a, "github", secret)
	for range cap(automationIngressSlots) {
		automationIngressSlots <- struct{}{}
	}
	defer func() {
		for range cap(automationIngressSlots) {
			<-automationIngressSlots
		}
	}()
	raw := ingressPush(strings.Repeat("a", 40))
	ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-concurrency-budget", 0), 429)
	if autoCount(t, f, "suma_automation_ingress_deliveries") != 0 {
		t.Fatal("busy receiver persisted input")
	}
}

func TestAutomationIngressNativeTagPushAndExactRefFilters(t *testing.T) {
	for _, tc := range []struct{ name, filter, eventRef, want string }{
		{"branch filter ignores tag", "refs/heads/main", "refs/tags/v1", "ignored"},
		{"exact tag accepts tag", "refs/tags/v1", "refs/tags/v1", "accepted"},
		{"tag filter ignores branch", "refs/tags/v1", "refs/heads/main", "ignored"},
		{"all refs accept tag", "", "refs/tags/v1", "accepted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, a := automationFixture(t, "allow", "manual", "skip")
			a = autoCommand(t, f, a, "enable").Rule
			secret := strings.Repeat("e", 64)
			in := ingressTestEntry(t, f, a, "github", secret)
			if tc.filter != in.Ref {
				spec := in.AutomationIngressSpec
				spec.Ref = tc.filter
				in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "revise", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision, Spec: &spec})
				in = ingressTestCommand(t, f, in, AutomationIngressCommandRequest{Action: "enable", ExpectedRuleVersion: a.Version, ExpectedRuleRevision: a.Revision})
			}
			raw := []byte(strings.Replace(string(ingressPush(strings.Repeat("a", 40))), "refs/heads/main", tc.eventRef, 1))
			receipt := ingressTestPost(t, f, in, raw, ingressTestHeaders(in, secret, raw, "push", "delivery-native-tag", 0), 202)
			if receipt.State != tc.want {
				t.Fatal("native tag ref filtering changed", receipt.State, tc.want)
			}
			if tc.want == "accepted" {
				autoTick(t, f)
				delivery, err := f.s.GetAutomationIngressDelivery(context.Background(), f.user, f.project, in.ID, receipt.ID)
				if err != nil {
					t.Fatal(err)
				}
				run, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, delivery.RunID)
				if err != nil || run.WorkflowVersion != 1 || run.Nodes[0].ID != "gate" {
					t.Fatal("tag input changed fixed workflow", err)
				}
			}
		})
	}
}
