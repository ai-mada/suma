package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/store"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func replacementBody200(f *fixture, p ToolProfile, profile tc.Profile) map[string]any {
	return f.body(map[string]any{"expected_version": p.Version, "worker_id": p.WorkerID, "profile": profile, "pause_original": true, "name": "Reviewed replacement"})
}

func TestToolProfileReplacementExplicitReuseMigrationAndAtomicRollback(t *testing.T) {
	f, _, source := toolFixture(t)
	ctx := context.Background()
	source = toolDiagnosticsEnable(t, f, source)
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "replacement-worker200", "profiles": []any{}, "deployment": false}), 201)
	next := source.Profile
	next.ProfileSHA256 = strings.Repeat("c", 64)
	next.ToolVersion = "existing target200"
	target, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "replacement-worker200", Name: "Existing enabled target", Profile: next, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	target = toolDiagnosticsEnable(t, f, target)
	alt := next
	alt.CommandID = "alternate-target200"
	alt.ProfileSHA256 = strings.Repeat("d", 64)
	_, e = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, worker["token"].(string)), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: target.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: next, Available: true}, {Profile: alt, Available: true}}})
	if e != nil {
		t.Fatal(e)
	}
	create := replacementBody200(f, source, alt)
	create["worker_id"] = target.WorkerID
	path := "/tool-profiles/" + source.ID + "/replacements"
	before := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
	if _, e = f.s.DB.Exec(`CREATE TRIGGER owned_replacement_rollback BEFORE INSERT ON suma_tool_profile_replacements BEGIN SELECT RAISE(ABORT,'owned rollback');END`); e != nil {
		t.Fatal(e)
	}
	f.call("POST", path, create, 500)
	after := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
	current, e := f.s.GetToolProfile(ctx, f.actor, f.project, source.ID)
	if e != nil || current.Version != source.Version || current.State != "enabled" || stringify(before) != stringify(after) {
		t.Fatal("relation failure left a target, source pause or receipt", before, after, current, e)
	}
	if _, e = f.s.DB.Exec(`DROP TRIGGER owned_replacement_rollback`); e != nil {
		t.Fatal(e)
	}
	created := replacementReply200(t, f.call("POST", path, create, 201))
	source = created.Original
	reuse := f.body(map[string]any{"expected_version": source.Version, "worker_id": target.WorkerID, "profile": next, "pause_original": false, "reuse": map[string]any{"profile_id": target.ID, "expected_version": target.Version}})
	count := expertCount(t, f, "suma_tool_profiles")
	result := replacementReply200(t, f.call("POST", path, reuse, 201))
	if result.Disposition != "reused" || result.Replacement.ID != target.ID || result.Replacement.Version != target.Version || result.Replacement.State != "enabled" || result.Original.Version != source.Version || result.PauseOriginal || expertCount(t, f, "suma_tool_profiles") != count {
		t.Fatal("explicit reuse changed enabled target or ignored migration choice", result)
	}
	// Existing configurations cannot be silently adopted by a create intent.
	duplicate := replacementBody200(f, source, next)
	duplicate["worker_id"] = target.WorkerID
	f.call("POST", path, duplicate, 409)
	for _, change := range []func(map[string]any){
		func(body map[string]any) { delete(body, "pause_original") },
		func(body map[string]any) { body["pause_original"] = nil },
		func(body map[string]any) { body["reuse"] = nil },
		func(body map[string]any) { body["name"] = "" },
		func(body map[string]any) {
			body["reuse"] = map[string]any{"profile_id": source.ID, "expected_version": source.Version}
		},
	} {
		var body map[string]any
		raw, _ := json.Marshal(reuse)
		json.Unmarshal(raw, &body)
		body["idempotency_key"] = newID()
		change(body)
		f.call("POST", path, body, 400)
	}
	var stale map[string]any
	raw, _ := json.Marshal(reuse)
	json.Unmarshal(raw, &stale)
	stale["idempotency_key"] = newID()
	stale["reuse"] = map[string]any{"profile_id": target.ID, "expected_version": target.Version + 1}
	f.call("POST", path, stale, 409)
	if expertCount(t, f, "suma_tool_profile_replacements") != 2 || expertCount(t, f, "suma_tool_profiles") != count {
		t.Fatal("rejected replacement left state")
	}
	reviewer := invitedReviewer(t, f)
	if _, e = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, reviewer.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	var request ToolProfileReplacementRequest
	raw, _ = json.Marshal(reuse)
	json.Unmarshal(raw, &request)
	_, e = f.s.ReplaceToolProfile(ctx, reviewer, f.project, source.ID, request)
	requireAuthorityStatus(t, e, 403)
	// English errors stay fixed diagnostics, not untranslated provider strings.
	r := httptest.NewRequest(http.MethodPost, "/api/v1"+path, strings.NewReader(`{"project_id":"`+f.project+`"}`))
	r.AddCookie(f.cookie)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept-Language", "en-US")
	r.Header.Set("X-Suma-Expected-User", f.user.ID)
	r.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "The tool configuration replacement request or pagination parameters are invalid.") {
		t.Fatal("replacement diagnostic lost English localization", w.Code, w.Body.String())
	}
}

func TestToolProfileReplacementHistoryCursorAndInitialSchema(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	next := p.Profile
	next.ProfileSHA256 = strings.Repeat("c", 64)
	toolDiagnosticsReportInventory(t, f, token, p, tc.Observation{Profile: next, Available: true})
	first := replacementReply200(t, f.call("POST", "/tool-profiles/"+p.ID+"/replacements", replacementBody200(f, p, next), 201))
	if first.Original.Version != p.Version {
		t.Fatal("already paused source gained a fake version")
	}
	for i := 0; i < 2; i++ {
		req := ToolProfileReplacementRequest{ProjectID: f.project, ExpectedVersion: p.Version, WorkerID: p.WorkerID, Profile: next, PauseOriginal: false, IdempotencyKey: newID(), Reuse: &ToolProfileReplacementReuse{ProfileID: first.Replacement.ID, ExpectedVersion: first.Replacement.Version}}
		if _, e := f.s.ReplaceToolProfile(ctx, f.actor, f.project, p.ID, req); e != nil {
			t.Fatal(e)
		}
	}
	page, e := f.s.ListToolProfileReplacements(ctx, f.actor, f.project, p.ID, ToolProfileReplacementListQuery{Limit: 1})
	if e != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(page, e)
	}
	seen := map[string]bool{page.Items[0].ID: true}
	cursor := page.NextCursor
	for cursor != "" {
		page, e = f.s.ListToolProfileReplacements(ctx, f.actor, f.project, p.ID, ToolProfileReplacementListQuery{Limit: 1, Cursor: cursor})
		if e != nil || len(page.Items) != 1 || seen[page.Items[0].ID] {
			t.Fatal("unstable history page", page, e)
		}
		seen[page.Items[0].ID] = true
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatal("history skipped a committed handoff", seen)
	}
	page, e = f.s.ListToolProfileReplacements(ctx, f.actor, f.project, p.ID, ToolProfileReplacementListQuery{Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	for _, q := range []ToolProfileReplacementListQuery{{Direction: "incoming", Limit: 1, Cursor: page.NextCursor}, {Limit: 2, Cursor: page.NextCursor}, {Limit: 1, Cursor: strings.Repeat("a", 2049)}} {
		if _, e = f.s.ListToolProfileReplacements(ctx, f.actor, f.project, p.ID, q); applicationStatus(e) != 400 {
			t.Fatal("cursor crossed filters or limit", q, e)
		}
	}
	reviewer := invitedReviewer(t, f)
	if _, e = f.s.ListToolProfileReplacements(ctx, reviewer, f.project, p.ID, ToolProfileReplacementListQuery{Limit: 1, Cursor: page.NextCursor}); applicationStatus(e) != 400 {
		t.Fatal("actor borrowed another actor cursor", e)
	}
	if _, e = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, reviewer.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.ListToolProfileReplacements(ctx, reviewer, f.project, p.ID, ToolProfileReplacementListQuery{Direction: "invalid", Cursor: "broken"})
	requireAuthorityStatus(t, e, 403)
	for _, name := range []string{"suma_tool_profile_replacements", "Suma_Tool_Profile_Replacements"} {
		t.Run(name, func(t *testing.T) {
			db, e := store.Open("sqlite", filepath.Join(t.TempDir(), "unknown.sqlite"))
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			if _, e = db.Exec(`CREATE TABLE ` + name + `(user_text TEXT)`); e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(`INSERT INTO ` + name + ` VALUES('PRESERVE')`); e != nil {
				t.Fatal(e)
			}
			s := &Service{DB: db, Dialect: "sqlite"}
			if e = s.Initialize(ctx); !errors.Is(e, store.ErrInitialSchema) {
				t.Fatal("unknown relation schema accepted", e)
			}
			var text string
			var count int
			if e = db.QueryRow(`SELECT user_text FROM ` + name).Scan(&text); e != nil || text != "PRESERVE" {
				t.Fatal(text, e)
			}
			if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&count); e != nil || count != 1 {
				t.Fatal("DDL ran before relation preflight", count, e)
			}
		})
	}
	t.Run("committed_relation_must_remain_readable", func(t *testing.T) {
		f, token, _ := toolFixture(t)
		ctx := context.Background()
		profile := tc.Profile{CommandID: "codex-configuration200", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "owned catalog-only fixture"}
		original, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Original Codex configuration", Profile: profile, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		models := make([]cc.ModelOption, 256)
		for i := range models {
			efforts := make([]string, 32)
			for j := range efforts {
				efforts[j] = fmt.Sprintf("effort-%02d-", j) + strings.Repeat("x", 118)
			}
			models[i] = cc.ModelOption{Model: fmt.Sprintf("model-%03d", i), ReasoningEfforts: efforts, DefaultReasoningEffort: efforts[0]}
		}
		catalog, e := cc.NewCatalog(strings.Repeat("d", 64), models)
		if e != nil {
			t.Fatal(e)
		}
		observation := tc.Observation{Profile: profile, Available: true, CodexCatalog: &catalog}
		encoded, _ := json.Marshal([]tc.Observation{observation})
		if tc.ValidateObservation(observation) != nil || len(encoded) >= 2<<20 || len(encoded) <= 1<<20 {
			t.Fatal("fixture must be a legal bounded individual inventory", len(encoded))
		}
		reportCodexCatalog(t, f, token, original, catalog)
		original = toolDiagnosticsEnable(t, f, original)
		worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "large-target200", "profiles": []any{}, "deployment": false}), 201)
		_, e = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, worker["token"].(string)), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: "large-target200", OperationID: newID(), Items: []tc.Observation{observation}})
		if e != nil {
			t.Fatal(e)
		}
		before := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
		request := replacementBody200(f, original, profile)
		request["worker_id"] = "large-target200"
		body, _ := json.Marshal(request)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/tool-profiles/"+original.ID+"/replacements", strings.NewReader(string(body)))
		r.AddCookie(f.cookie)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Suma-Expected-User", f.user.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 409 {
			t.Fatal("a successful commit must not create a relation or receipt that the original 2MiB reader cannot recover", w.Code, "response bytes", w.Body.Len())
		}
		after := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
		current, e := f.s.GetToolProfile(ctx, f.actor, f.project, original.ID)
		if e != nil || current.Version != original.Version || current.State != "enabled" || stringify(before) != stringify(after) {
			t.Fatal("oversized snapshot left partial configuration effects", before, after, current.Version, e)
		}
	})
}
func replacementReply200(t *testing.T, value map[string]any) ToolProfileReplacement {
	t.Helper()
	var out ToolProfileReplacement
	raw, _ := json.Marshal(value)
	if e := json.Unmarshal(raw, &out); e != nil || !ValidToolProfileReplacement(out) {
		t.Fatal("invalid replacement receipt", out, e)
	}
	return out
}
func TestToolProfileReplacementAtomicCreationReceiptAndHistory(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	p = toolDiagnosticsEnable(t, f, p)
	next := p.Profile
	next.ProfileSHA256 = strings.Repeat("c", 64)
	next.ToolVersion = "reviewed version200"
	toolDiagnosticsReportInventory(t, f, token, p, tc.Observation{Profile: next, Available: true})
	body := replacementBody200(f, p, next)
	path := "/tool-profiles/" + p.ID + "/replacements"
	out := replacementReply200(t, f.call("POST", path, body, 201))
	if out.ProjectID != f.project || out.Original.ID != p.ID || out.Original.State != "paused" || out.Original.Version != p.Version+1 || !tc.EqualProfile(out.Original.Profile, p.Profile) || out.Replacement.ID == p.ID || out.Replacement.State != "paused" || out.Replacement.Version != 1 || out.Replacement.Available || out.Replacement.EnabledBy != "" || !tc.EqualProfile(out.Replacement.Profile, next) || out.Disposition != "created" || out.CreatedBy != f.user.ID {
		t.Fatal("handoff changed historical configuration or implicitly enabled target", out)
	}
	if expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("configuration handoff dispatched a task")
	}
	before := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
	// Original replay survives later source/target versions and stale inventory;
	// it is the same historical preparation, not a new readiness assessment.
	if _, e := f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: out.Original.Version, Action: "revoke", IdempotencyKey: newID()}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.CommandToolProfile(ctx, f.actor, f.project, out.Replacement.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: out.Replacement.Version, Action: "pause", IdempotencyKey: newID()}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z'`); e != nil {
		t.Fatal(e)
	}
	before[2] += 2
	replayed := replacementReply200(t, f.call("POST", path, body, 201))
	if stringify(replayed) != stringify(out) {
		t.Fatal("original receipt was recomputed", replayed, out)
	}
	after := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
	if stringify(before) != stringify(after) {
		t.Fatal("receipt replay repeated configuration effects", before, after)
	}
	detail := replacementReply200(t, f.call("GET", "/tool-profile-replacements/"+out.ID+"?project_id="+f.project, nil, 200))
	if stringify(detail) != stringify(out) {
		t.Fatal("historical relation changed with live configuration", detail)
	}
	for _, entry := range []struct{ id, direction string }{{p.ID, "outgoing"}, {out.Replacement.ID, "incoming"}} {
		page := f.call("GET", "/tool-profiles/"+entry.id+"/replacements?project_id="+f.project+"&direction="+entry.direction+"&limit=1", nil, 200)
		if page["profile_id"] != entry.id || page["direction"] != entry.direction || len(page["items"].([]any)) != 1 {
			t.Fatal("history lost exact direction/source", page)
		}
	}
}
