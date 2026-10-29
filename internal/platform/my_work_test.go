package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func myWorkMember(t *testing.T, f *fixture) Record {
	t.Helper()
	r, err := f.s.Create(context.Background(), f.actor, f.project, "members", f.body(map[string]any{"title": "Current human", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func myWorkSeed(t *testing.T, f *fixture, owner string, count int) []Record {
	t.Helper()
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	items := make([]Record, 0, count)
	for i := 0; i < count; i++ {
		r := recordFrom(f.user, f.project, map[string]any{"title": fmt.Sprintf("Assigned %03d", i), "status": "todo", "owner_id": owner})
		r.ID = fmt.Sprintf("assigned-%03d", i)
		r.UpdatedAt = "2026-10-29T00:00:00Z"
		if err = f.s.insertRecord(context.Background(), tx, r, "tasks"); err != nil {
			t.Fatal(err)
		}
		items = append(items, r)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return items
}

func myWorkDecode(t *testing.T, raw map[string]any) MyWorkPage {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var out MyWorkPage
	if err = json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMyWorkCanonicalHumanWholeScopeAndLivePagination(t *testing.T) {
	f := newFixture(t)
	member := myWorkMember(t, f)
	if member.ID == f.user.ID {
		t.Fatal("fixture must distinguish resource and user")
	}
	tasks := myWorkSeed(t, f, member.ID, 107)
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='blocked' WHERE id IN (?,?)`, tasks[0].ID, tasks[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='archived' WHERE id=?`, tasks[2].ID); err != nil {
		t.Fatal(err)
	}
	// Neither a user ID masquerading as an owner nor an unassigned record is mine.
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET owner_id=? WHERE id=?`, f.user.ID, tasks[3].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET owner_id='' WHERE id=?`, tasks[4].ID); err != nil {
		t.Fatal(err)
	}
	page := myWorkDecode(t, f.call("GET", "/my-work?project_id="+f.project+"&limit=19", nil, 200))
	if page.UserID != f.user.ID || page.ResourceID != member.ID || page.ResourceName != member.Title || page.ResourceStatus != "active" || page.Total != 104 || page.Limit != 19 || len(page.Items) != 19 || page.AsOf == "" {
		t.Fatalf("incorrect canonical complete scope: %+v", page)
	}
	counts := map[string]int{}
	for _, count := range page.StatusCounts {
		counts[count.Status] = count.Total
	}
	if counts["blocked"] != 2 || counts["todo"] != 102 || len(counts) != 2 {
		t.Fatalf("wrong whole-scope counts: %#v", counts)
	}
	seen := map[string]bool{}
	last := ""
	for {
		for _, item := range page.Items {
			if item.OwnerID != member.ID || item.ProjectID != f.project || seen[item.ID] || last != "" && last >= item.ID {
				t.Fatalf("incorrect order or ownership: %+v", item)
			}
			seen[item.ID], last = true, item.ID
		}
		if page.NextCursor == "" {
			break
		}
		page = myWorkDecode(t, f.call("GET", "/my-work?project_id="+f.project+"&limit=19&cursor="+url.QueryEscape(page.NextCursor), nil, 200))
	}
	if len(seen) != 104 {
		t.Fatalf("incomplete server pagination: %d", len(seen))
	}
	// A fresh read reflects a real assignment change rather than cached first-page facts.
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET owner_id='' WHERE id=?`, tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	page = myWorkDecode(t, f.call("GET", "/my-work?project_id="+f.project+"&status=blocked", nil, 200))
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != tasks[1].ID {
		t.Fatalf("reassignment remained mine: %+v", page)
	}
}

func TestMyWorkLiteralSearchStatusCountsAndCanonicalCursor(t *testing.T) {
	f := newFixture(t)
	member := myWorkMember(t, f)
	tasks := myWorkSeed(t, f, member.ID, 5)
	for i, title := range []string{"100%_! Alpha 甲", "100%_! ALPHA 乙", "100other Alpha", "percent", "100%_! Alpha 丙"} {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET title=? WHERE id=?`, title, tasks[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='blocked' WHERE id=?`, tasks[1].ID); err != nil {
		t.Fatal(err)
	}
	query := "100%_! alpha"
	options := MyWorkOptions{Q: query, Limit: 1}
	page, err := f.s.MyWork(context.Background(), f.actor, f.project, options)
	if err != nil || page.Total != 3 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("literal case-insensitive query: %+v %v", page, err)
	}
	filtered, err := f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Q: query, Status: "blocked"})
	if err != nil || filtered.Total != 1 || len(filtered.StatusCounts) != 2 {
		t.Fatalf("status counts accidentally filtered: %+v %v", filtered, err)
	}
	for _, changed := range []MyWorkOptions{{Q: query + " ", Limit: 1, Cursor: page.NextCursor}, {Q: query, Status: "todo", Limit: 1, Cursor: page.NextCursor}, {Q: query, Limit: 2, Cursor: page.NextCursor}} {
		_, err = f.s.MyWork(context.Background(), f.actor, f.project, changed)
		requireAuthorityStatus(t, err, 400)
	}
	raw, err := base64.RawURLEncoding.DecodeString(page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{string(raw[:len(raw)-1]) + `,"unknown":true}`, " " + string(raw)} {
		options.Cursor = base64.RawURLEncoding.EncodeToString([]byte(invalid))
		_, err = f.s.MyWork(context.Background(), f.actor, f.project, options)
		requireAuthorityStatus(t, err, 400)
	}
	options.Cursor = page.NextCursor
	if _, err = f.s.DB.Exec(`UPDATE suma_members SET version=version+1 WHERE id=?`, member.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.MyWork(context.Background(), f.actor, f.project, options)
	requireAuthorityStatus(t, err, 400)
	// Spaces are literal and description text participates without searching payload JSON.
	page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Q: " 100%_!"})
	if err != nil || page.Total != 0 {
		t.Fatalf("query was silently trimmed: %+v %v", page, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET description='说明甲乙',data='{"private":"payload-only"}' WHERE id=?`, tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Q: "甲乙"})
	if err != nil || page.Total != 1 {
		t.Fatalf("Unicode description not searched: %+v %v", page, err)
	}
	page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Q: "payload-only"})
	if err != nil || page.Total != 0 {
		t.Fatalf("unexpected payload search: %+v %v", page, err)
	}
}

func TestMyWorkUnboundInactiveAndInconsistentResourceFailClosed(t *testing.T) {
	f := newFixture(t)
	page, err := f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{})
	if err != nil || page.ResourceStatus != "unbound" || page.ResourceID != "" || page.Total != 0 || page.Items == nil || page.StatusCounts == nil {
		t.Fatalf("unbound current human: %+v %v", page, err)
	}
	member := myWorkMember(t, f)
	myWorkSeed(t, f, member.ID, 2)
	for _, state := range []string{"inactive", "archived"} {
		if _, err = f.s.DB.Exec(`UPDATE suma_members SET status=? WHERE id=?`, state, member.ID); err != nil {
			t.Fatal(err)
		}
		page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{})
		if err != nil || page.ResourceStatus != "inactive" || page.ResourceID != member.ID || page.Total != 0 || len(page.Items) != 0 {
			t.Fatalf("inactive resource revealed work: %+v %v", page, err)
		}
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_members SET status='active',data='{"user_id":"other-user"}' WHERE id=?`, member.ID); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{})
	if err != nil || page.ResourceStatus != "inactive" || page.Total != 0 {
		t.Fatalf("noncanonical identity revealed work: %+v %v", page, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_members SET kind='agent',data=? WHERE id=?`, stringify(map[string]any{"user_id": f.user.ID}), member.ID); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{})
	if err != nil || page.ResourceStatus != "inactive" || page.Total != 0 {
		t.Fatalf("nonhuman resource revealed work: %+v %v", page, err)
	}
}

func TestMyWorkCurrentCanonicalBindingChangesInvalidateCursor(t *testing.T) {
	f := newFixture(t)
	member := myWorkMember(t, f)
	tasks := myWorkSeed(t, f, member.ID, 3)
	page, err := f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("first page: %+v %v", page, err)
	}
	// Model a current canonical binding change without relying on a browser's
	// cached member data or manufacturing a second registered account.
	replacement := recordFrom(f.user, f.project, map[string]any{"title": "Replacement binding", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}})
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = f.s.insertRecord(context.Background(), tx, replacement, "members"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE suma_human_resources SET resource_id=? WHERE tenant_id=? AND project_id=? AND user_id=?`, replacement.ID, f.user.TenantID, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE suma_tasks SET owner_id=? WHERE id=?`, replacement.ID, tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
	page, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{})
	if err != nil || page.ResourceID != replacement.ID || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != tasks[0].ID {
		t.Fatalf("stale binding used: %+v %v", page, err)
	}
}

func TestMyWorkCurrentHumanCredentialsAndRevocation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	myWorkSeed(t, f, h.owner.ID, 3)
	page, err := f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("first page: %+v %v", page, err)
	}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, issueErr := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "Personal work test", IdempotencyKey: "my-work-kind-" + scope})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		actor := domainAActor(t, f.s, issued.Token)
		out, readErr := f.s.MyWork(context.Background(), actor, f.project, MyWorkOptions{})
		if scope == "api" {
			if readErr != nil || out.UserID != f.user.ID || out.ResourceID != h.owner.ID || out.Total != 4 {
				t.Fatalf("human API identity: %+v %v", out, readErr)
			}
		} else {
			requireAuthorityStatus(t, readErr, 403)
		}
		r := httptest.NewRequest(http.MethodGet, "/api/v1/my-work?project_id="+f.project, nil)
		r.Header.Set("Authorization", "Bearer "+issued.Token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		want := 403
		if scope == "api" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("%s HTTP => %d: %s", scope, w.Code, w.Body.String())
		}
	}
	// A second valid human's cursor cannot select the original human's work.
	_, err = f.s.MyWork(context.Background(), h.receiver, f.project, MyWorkOptions{Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.MyWork(context.Background(), h.receiver, f.project, MyWorkOptions{})
	requireAuthorityStatus(t, err, 403)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	_, err = f.s.MyWork(context.Background(), f.actor, f.project, MyWorkOptions{Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 401)
}

func TestMyWorkDraftAndCustomStatesRemainInCurrentCounts(t *testing.T) {
	f := newFixture(t)
	member := myWorkMember(t, f)
	items := myWorkSeed(t, f, member.ID, 2)
	for i, state := range []string{"draft", "triage_pending"} {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status=? WHERE id=?`, state, items[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	all := myWorkDecode(t, f.call("GET", "/my-work?project_id="+f.project, nil, 200))
	if all.Total != 2 || len(all.Items) != 2 || len(all.StatusCounts) != 2 {
		t.Fatal("current business states disappeared", all)
	}
	draft := myWorkDecode(t, f.call("GET", "/my-work?project_id="+f.project+"&status=draft", nil, 200))
	if draft.Total != 1 || len(draft.Items) != 1 || draft.Items[0].Status != "draft" || len(draft.StatusCounts) != 2 {
		t.Fatal("draft filter must retain all-state counts", draft)
	}
}

func TestMyWorkHTTPClosedQueriesAndBounds(t *testing.T) {
	f := newFixture(t)
	for _, suffix := range []string{"&owner_id=forged", "&user_id=forged", "&status=archived", "&status=done", "&status=todo&status=blocked", "&limit=01", "&limit=0", "&limit=51", "&limit=1e1", "&cursor=", "&q=%00", "&q=" + url.QueryEscape(strings.Repeat("甲", 201)), "&q=%FF", "&project_id=another"} {
		f.call("GET", "/my-work?project_id="+f.project+suffix, nil, 400)
	}
	for _, status := range []string{"all", "draft", "todo", "in_progress", "in_review", "accepted", "blocked", "cancelled"} {
		f.call("GET", "/my-work?project_id="+f.project+"&status="+status+"&q="+url.QueryEscape(strings.Repeat("甲", 200)), nil, 200)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Other project", "idempotency_key": "my-work-other-project"}, 201)["id"].(string)
	member := myWorkMember(t, f)
	myWorkSeed(t, f, member.ID, 2)
	page := myWorkDecode(t, f.call("GET", "/my-work?project_id="+f.project+"&limit=1", nil, 200))
	f.call("GET", "/my-work?project_id="+other+"&limit=1&cursor="+url.QueryEscape(page.NextCursor), nil, 400)
	f.call("GET", "/my-work?project_id=unavailable", nil, 404)
}
