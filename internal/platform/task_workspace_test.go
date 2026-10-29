package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func seedTaskWorkspace(t *testing.T, f *fixture, project string, count int) []Record {
	t.Helper()
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	out := []Record{}
	for i := 0; i < count; i++ {
		r := recordFrom(f.user, project, map[string]any{"title": fmt.Sprintf("Task %03d", i%11)})
		r.ID = fmt.Sprintf("%s-task-%03d", project, i)
		r.Priority = []string{"medium", "urgent", "low", "high", "custom"}[i%5]
		r.CreatedAt = fmt.Sprintf("2026-01-%02dT00:00:00Z", i%3+1)
		r.UpdatedAt = fmt.Sprintf("2026-02-%02dT00:00:00Z", i%4+1)
		if err = f.s.insertRecord(ctx, tx, r, "tasks"); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return out
}

func readTaskWorkspace(t *testing.T, f *fixture, q TaskWorkspaceQuery) TaskWorkspacePage {
	t.Helper()
	page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, q)
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil || !validReleaseTime(page.AsOf) {
		t.Fatalf("page must have a nonnull collection and observation time: %#v", page)
	}
	return page
}

func TestTaskWorkspaceFiltersFullProjectAndLiteralSearch(t *testing.T) {
	f := newFixture(t)
	rows := seedTaskWorkspace(t, f, f.project, 125)
	// Older matching records lie beyond the first 100 of the unfiltered list.
	for _, r := range rows[120:] {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET title='Tail task',description='Needle 100%_! literal',status='custom-state',priority='custom-priority',owner_id='member-a',updated_at='2000-01-01T00:00:00Z' WHERE id=?`, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	all := readTaskWorkspace(t, f, TaskWorkspaceQuery{})
	if all.Total != 125 || len(all.Items) != 100 || all.NextCursor == "" {
		t.Fatalf("default first page has incorrect full scope: %#v", all)
	}
	q := TaskWorkspaceQuery{Q: "nEEdLe", Status: "custom-state", Priority: "custom-priority", OwnerID: "member-a", Limit: 2}
	page := readTaskWorkspace(t, f, q)
	if page.Total != 5 || len(page.Items) != 2 || page.Items[0].ID != rows[120].ID {
		t.Fatalf("filter was applied after truncation or ignored a predicate: %#v", page)
	}
	if spaced := readTaskWorkspace(t, f, TaskWorkspaceQuery{Q: "  nEEdLe  "}); spaced.Total != 0 {
		t.Fatal("literal search whitespace was normalized")
	}
	for _, control := range []string{"\n", "\t", "\u0085"} {
		_, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Q: "literal" + control + "query"})
		requireAuthorityStatus(t, err, 400)
	}
	for _, search := range []string{"%_!", "100%", "_", "!", rows[124].ID} {
		got := readTaskWorkspace(t, f, TaskWorkspaceQuery{Q: search})
		want := 5
		if search == rows[124].ID {
			want = 1
		}
		if got.Total != want {
			t.Fatalf("literal search %q matched %d, want %d", search, got.Total, want)
		}
	}
	for _, query := range []TaskWorkspaceQuery{{Q: "' OR 1=1 --"}, {Status: "absent-status"}, {OwnerID: "absent-member"}, {Priority: "absent-priority"}, {Q: "needle", OwnerID: "__unassigned__"}} {
		got := readTaskWorkspace(t, f, query)
		if got.Total != 0 || len(got.Items) != 0 || got.NextCursor != "" {
			t.Fatalf("unknown exact filter leaked records: %#v %#v", query, got)
		}
	}
	if page := readTaskWorkspace(t, f, TaskWorkspaceQuery{Unassigned: true}); page.Total != 120 {
		t.Fatalf("unassigned was not an exact empty-owner filter: %d", page.Total)
	}
}

func TestTaskWorkspaceStablePagesForEverySort(t *testing.T) {
	f := newFixture(t)
	rows := seedTaskWorkspace(t, f, f.project, 127)
	for _, mode := range []string{"updated_desc", "created_desc", "title_asc", "priority_desc"} {
		t.Run(mode, func(t *testing.T) {
			want := append([]Record(nil), rows...)
			ranks := map[string]int{"urgent": 4, "high": 3, "medium": 2, "low": 1}
			sort.Slice(want, func(i, j int) bool {
				a, b := want[i], want[j]
				switch mode {
				case "updated_desc":
					if a.UpdatedAt != b.UpdatedAt {
						return a.UpdatedAt > b.UpdatedAt
					}
				case "created_desc":
					if a.CreatedAt != b.CreatedAt {
						return a.CreatedAt > b.CreatedAt
					}
				case "title_asc":
					if a.Title != b.Title {
						return a.Title < b.Title
					}
				case "priority_desc":
					if ranks[a.Priority] != ranks[b.Priority] {
						return ranks[a.Priority] > ranks[b.Priority]
					}
				}
				return a.ID < b.ID
			})
			got := []string{}
			q := TaskWorkspaceQuery{Sort: mode, Limit: 17}
			for pages := 0; ; pages++ {
				if pages > 8 {
					t.Fatal("pagination did not terminate")
				}
				page := readTaskWorkspace(t, f, q)
				if page.Total != len(rows) || len(page.Items) > q.Limit {
					t.Fatalf("incorrect page scope: %#v", page)
				}
				for _, r := range page.Items {
					got = append(got, r.ID)
				}
				if page.NextCursor == "" {
					break
				}
				q.Cursor = page.NextCursor
			}
			ids := []string{}
			for _, r := range want {
				ids = append(ids, r.ID)
			}
			if !reflect.DeepEqual(got, ids) {
				t.Fatalf("unstable/missing/duplicated %s page: got %v want %v", mode, got, ids)
			}
		})
	}
}

func TestTaskWorkspaceCursorBindsIdentityScopeAndAllFilters(t *testing.T) {
	f := newFixture(t)
	seedTaskWorkspace(t, f, f.project, 7)
	first := readTaskWorkspace(t, f, TaskWorkspaceQuery{Limit: 1})
	for _, q := range []TaskWorkspaceQuery{{Q: "Task"}, {Status: "todo"}, {Priority: "urgent"}, {OwnerID: "__unassigned__"}, {Sort: "title_asc"}} {
		q.Cursor = first.NextCursor
		_, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, q)
		requireAuthorityStatus(t, err, 400)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, 201)["id"].(string)
	seedTaskWorkspace(t, f, other, 2)
	_, err := f.s.ListTaskWorkspace(context.Background(), f.actor, other, TaskWorkspaceQuery{Cursor: first.NextCursor})
	requireAuthorityStatus(t, err, 400)
	invitation := governanceInvite(f, "task-workspace-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invitation, "task-workspace-viewer@example.test")
	viewer := domainAActor(t, f.s, f.cookie.Value)
	_, err = f.s.ListTaskWorkspace(context.Background(), viewer, f.project, TaskWorkspaceQuery{Cursor: first.NextCursor})
	requireAuthorityStatus(t, err, 400)
	// Canonical cursor encoding rejects appended fields and duplicate keys.
	raw, err := base64.RawURLEncoding.DecodeString(first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{`,"extra":1}`, `,"version":1}`} {
		changed := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(suffix)...)
		_, err = f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Cursor: base64.RawURLEncoding.EncodeToString(changed)})
		requireAuthorityStatus(t, err, 400)
	}
}

func TestTaskWorkspaceRechecksCurrentAuthorityAndReturnsNoPartialPage(t *testing.T) {
	for _, change := range []string{"logout", "membership"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			seedTaskWorkspace(t, f, f.project, 3)
			page := readTaskWorkspace(t, f, TaskWorkspaceQuery{Limit: 1})
			want := 403
			if change == "logout" {
				want = 401
				if err := logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
					t.Fatal(err)
				}
			}
			got, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Cursor: page.NextCursor})
			requireAuthorityStatus(t, err, want)
			if len(got.Items) != 0 || got.Total != 0 || got.NextCursor != "" || got.AsOf != "" {
				t.Fatalf("failed authority returned partial data: %#v", got)
			}
		})
	}
}

func TestTaskWorkspaceReadDoesNotInferDeliveryOrMutateFacts(t *testing.T) {
	f := newFixture(t)
	rows := seedTaskWorkspace(t, f, f.project, 1)
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET run_id='not-a-proof-of-success',status='in_review' WHERE id=?`, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, table := range []string{"suma_tasks", "dag_runs", "suma_reviews", "suma_operations", "suma_activity", "suma_tests"} {
		counts[table] = domainACount(t, f.s, table)
	}
	page := readTaskWorkspace(t, f, TaskWorkspaceQuery{})
	if page.Total != 1 || page.Items[0].Status != "in_review" || page.Items[0].RunID != "not-a-proof-of-success" {
		t.Fatalf("business and execution facts were conflated: %#v", page)
	}
	for table, count := range counts {
		if domainACount(t, f.s, table) != count {
			t.Fatalf("read mutated %s", table)
		}
	}
}

func TestTaskWorkspaceLongTitleCursorAndCurrentTotals(t *testing.T) {
	f := newFixture(t)
	rows := seedTaskWorkspace(t, f, f.project, 3)
	// requireTitle accepts up to 500 bytes; a legitimate multibyte title must
	// remain usable as a title-order cursor at that exact boundary.
	var page TaskWorkspacePage
	for _, longTitle := range []string{strings.Repeat("中", 166) + "ab", strings.Repeat("<", 500), strings.Repeat("\t", 500)} {
		if err := requireTitle(longTitle); err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET title=? WHERE id=?`, longTitle, r.ID); err != nil {
				t.Fatal(err)
			}
		}
		page = readTaskWorkspace(t, f, TaskWorkspaceQuery{Sort: "title_asc", Limit: 1})
		if page.Total != 3 || page.NextCursor == "" {
			t.Fatalf("missing long-title first page: %#v", page)
		}
		next := readTaskWorkspace(t, f, TaskWorkspaceQuery{Sort: "title_asc", Limit: 1, Cursor: page.NextCursor})
		if len(next.Items) != 1 || next.Items[0].ID != rows[1].ID {
			t.Fatalf("legal title cannot continue: %#v", next)
		}
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_tasks WHERE id=?`, rows[2].ID); err != nil {
		t.Fatal(err)
	}
	next := readTaskWorkspace(t, f, TaskWorkspaceQuery{Sort: "title_asc", Limit: 1, Cursor: page.NextCursor})
	if next.Total != 2 || len(next.Items) != 1 || next.Items[0].ID != rows[1].ID || next.NextCursor != "" {
		t.Fatalf("continuation is not a current authorized count and stable seek: %#v", next)
	}
}

func TestTaskWorkspaceProjectAPIScopeIsNotAgentOrRunnerAuthority(t *testing.T) {
	f := newFixture(t)
	seedTaskWorkspace(t, f, f.project, 2)
	ctx := context.Background()
	other := f.call("POST", "/projects", map[string]any{"title": "Other API scope", "idempotency_key": newID()}, 201)["id"].(string)
	for _, scope := range []string{"api", "agent", "runner"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		actor := domainAActor(t, f.s, issued.Token)
		page, err := f.s.ListTaskWorkspace(ctx, actor, f.project, TaskWorkspaceQuery{})
		if scope == "api" {
			if err != nil || page.Total != 2 {
				t.Fatalf("ordinary API read failed: %#v %v", page, err)
			}
			_, err = f.s.ListTaskWorkspace(ctx, actor, other, TaskWorkspaceQuery{})
			requireAuthorityStatus(t, err, 403)
		} else {
			requireAuthorityStatus(t, err, 403)
		}
	}
}

func TestTaskWorkspaceHTTPRejectsMalformedQueries(t *testing.T) {
	f := newFixture(t)
	server := &Server{Service: f.s}
	for _, suffix := range []string{
		"&limit=0", "&limit=101", "&limit=01", "&limit=1.5", "&limit=", "&limit=1&limit=2", "&sort=updated", "&sort=", "&owner_id=a/b", "&status=todo%00", "&q=%FF", "&q=" + strings.Repeat("a", 201), "&status=" + strings.Repeat("s", 201), "&priority=" + strings.Repeat("p", 201), "&cursor=", "&cursor=broken", "&unknown=1", "&project_id=" + f.project, "&q=a;q=b", "&q=x&q=y",
	} {
		t.Run(suffix, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/task-workspace?project_id="+f.project+suffix, nil)
			w := httptest.NewRecorder()
			server.taskWorkspaceHTTP(w, r, f.actor, f.project)
			if w.Code != 400 {
				t.Fatalf("malformed query accepted: %d %s", w.Code, w.Body.String())
			}
		})
	}
	for _, path := range []string{"/api/v1/task-workspace", "/api/v1/task-workspace?project_id=another"} {
		w := httptest.NewRecorder()
		server.taskWorkspaceHTTP(w, httptest.NewRequest(http.MethodGet, path, nil), f.actor, f.project)
		if w.Code != 400 {
			t.Fatalf("handler accepted inconsistent project: %d", w.Code)
		}
	}
	// Unknown but well-formed values have the same empty response; no membership lookup leaks.
	w := httptest.NewRecorder()
	server.taskWorkspaceHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/task-workspace?project_id="+f.project+"&status=unknown&owner_id=foreign-member&priority=custom&q="+url.QueryEscape(" literal %_! "), nil), f.actor, f.project)
	var page TaskWorkspacePage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Total != 0 || page.Items == nil {
		t.Fatalf("unknown exact filters did not return an empty collection: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private task page is cacheable")
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodHead} {
		w := httptest.NewRecorder()
		server.taskWorkspaceHTTP(w, httptest.NewRequest(method, "/api/v1/task-workspace?project_id="+f.project, nil), f.actor, f.project)
		if w.Code != 405 || w.Header().Get("Allow") != "GET" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read endpoint accepted %s: %d %v", method, w.Code, w.Header())
		}
	}
	// Exercise the root-mounted route as well as its strict handler.
	f.call("GET", "/task-workspace?project_id="+f.project, nil, 200)
}
