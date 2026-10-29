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

type taskCollectionFixture struct {
	f                                           *fixture
	owner, other, requirement, otherRequirement Record
	tasks                                       []Record
}

func newTaskCollectionFixture(t *testing.T, count int) *taskCollectionFixture {
	t.Helper()
	f := newFixture(t)
	create := func(kind string, body map[string]any) Record {
		r, err := f.s.Create(context.Background(), f.actor, f.project, kind, f.body(body))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	h := &taskCollectionFixture{f: f}
	h.owner = create("members", map[string]any{"title": "Human owner", "kind": "human", "data": map[string]any{"user_id": f.user.ID}})
	h.other = create("members", map[string]any{"title": "Agent owner", "kind": "agent"})
	h.requirement = create("requirements", map[string]any{"title": "First requirement"})
	h.otherRequirement = create("requirements", map[string]any{"title": "Second requirement"})
	h.tasks = seedTaskCollection(t, f, count, h.owner.ID, h.other.ID, h.requirement.ID, h.otherRequirement.ID)
	return h
}

func seedTaskCollection(t *testing.T, f *fixture, count int, owner, other, requirement, otherRequirement string) []Record {
	t.Helper()
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	states := []string{"todo", "draft", "in_progress", "in_review", "blocked", "accepted", "cancelled", "waiting_vendor", "archived"}
	priorities := []string{"urgent", "high", "medium", "low"}
	owners := []string{owner, other, ""}
	requirements := []string{requirement, otherRequirement}
	items := make([]Record, 0, count)
	for i := 0; i < count; i++ {
		r := recordFrom(f.user, f.project, map[string]any{"title": fmt.Sprintf("Collection %03d", i), "status": states[i%len(states)], "priority": priorities[i%len(priorities)], "owner_id": owners[i%len(owners)], "requirement_id": requirements[i%len(requirements)]})
		r.ID, r.UpdatedAt = fmt.Sprintf("collection-%03d", i), "2026-10-29T00:00:00Z"
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

func taskCollectionDecode[T any](t *testing.T, raw map[string]any) T {
	t.Helper()
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func taskCollectionPageQuery(base TaskWorkspaceQuery, status string, limit int, cursor string) TaskWorkspaceQuery {
	base.Status, base.Limit, base.Cursor = status, limit, cursor
	return base
}

func TestTaskCollectionWholeProjectGroupsAndPagination(t *testing.T) {
	h := newTaskCollectionFixture(t, 153)
	f := h.f
	summary := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", "/task-collection/summary?project_id="+f.project, nil, 200))
	if summary.ProjectID != f.project || summary.Total != 153 || len(summary.Groups) != 9 || summary.AsOf == "" {
		t.Fatalf("incomplete project scope: %+v", summary)
	}
	lastStatus := ""
	for _, group := range summary.Groups {
		if group.Total != 17 || lastStatus != "" && lastStatus >= group.Key {
			t.Fatalf("wrong genuine groups: %+v", summary)
		}
		lastStatus = group.Key
	}
	page := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&limit=23", nil, 200))
	seen := map[string]bool{}
	lastID := ""
	for {
		if page.Total != 153 || page.Limit != 23 || len(page.Items) > 23 || page.AsOf == "" {
			t.Fatalf("wrong page: %+v", page)
		}
		for _, item := range page.Items {
			if seen[item.ID] || item.ProjectID != f.project || lastID != "" && lastID >= item.ID {
				t.Fatalf("incorrect project keyset: %+v", item)
			}
			seen[item.ID], lastID = true, item.ID
		}
		if page.NextCursor == "" {
			break
		}
		page = taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&limit=23&cursor="+url.QueryEscape(page.NextCursor), nil, 200))
	}
	if len(seen) != 153 {
		t.Fatalf("lost project tasks beyond first page: %d", len(seen))
	}
}

func TestTaskCollectionBoardColumnsUseExactDynamicState(t *testing.T) {
	h := newTaskCollectionFixture(t, 153)
	f := h.f
	for _, status := range []string{"draft", "waiting_vendor", "accepted", "archived"} {
		options := TaskWorkspaceQuery{Status: status, Limit: 7}
		seen := map[string]bool{}
		for {
			page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, options)
			if err != nil || page.Total != 17 || len(page.Items) > 7 {
				t.Fatalf("column %q: %+v %v", status, page, err)
			}
			for _, item := range page.Items {
				if item.Status != status || seen[item.ID] {
					t.Fatalf("mixed or duplicate column %+v", item)
				}
				seen[item.ID] = true
			}
			if page.NextCursor == "" {
				break
			}
			options.Cursor = page.NextCursor
		}
		if len(seen) != 17 {
			t.Fatalf("incomplete %q column: %d", status, len(seen))
		}
	}
	for _, status := range []string{"done", "another_current_state"} {
		page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Status: status})
		if err != nil || page.Total != 0 || len(page.Items) != 0 {
			t.Fatalf("invented status alias: %+v %v", page, err)
		}
	}
	page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{})
	if err != nil || page.Total != 153 {
		t.Fatalf("unfiltered status scope: %+v %v", page, err)
	}
	// "all" is also a permitted literal business state. Only an absent or
	// empty selector means the entire project, never a nonempty magic value.
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET status='all' WHERE id IN (?,?)`, h.tasks[7].ID, h.tasks[16].ID); err != nil {
		t.Fatal(err)
	}
	summary := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", "/task-collection/summary?project_id="+f.project, nil, 200))
	allGroup := 0
	for _, group := range summary.Groups {
		if group.Key == "all" {
			allGroup = group.Total
		}
	}
	if summary.Total != 153 || allGroup != 2 {
		t.Fatalf("literal group lost from summary: %+v", summary)
	}
	unfiltered := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&status=", nil, 200))
	if unfiltered.Total != 153 {
		t.Fatalf("empty selector is not whole project: %+v", unfiltered)
	}
	literal := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&status=all&limit=1", nil, 200))
	if literal.Total != 2 || len(literal.Items) != 1 || literal.Items[0].Status != "all" || literal.NextCursor == "" {
		t.Fatalf("literal all column became project wildcard: %+v", literal)
	}
	next := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&status=all&limit=1&cursor="+url.QueryEscape(literal.NextCursor), nil, 200))
	if next.Total != 2 || len(next.Items) != 1 || next.Items[0].Status != "all" || next.Items[0].ID == literal.Items[0].ID || next.NextCursor != "" {
		t.Fatalf("literal column pagination: %+v", next)
	}
	f.call("GET", "/task-workspace?project_id="+f.project+"&limit=1&cursor="+url.QueryEscape(literal.NextCursor), nil, 400)
}

func TestTaskWorkspaceQuerySearchLiteralAndWholeCounts(t *testing.T) {
	h := newTaskCollectionFixture(t, 153)
	f := h.f
	for _, entry := range []struct {
		filters TaskWorkspaceQuery
		total   int
	}{
		{TaskWorkspaceQuery{OwnerID: h.owner.ID}, 51}, {TaskWorkspaceQuery{OwnerID: h.other.ID}, 51}, {TaskWorkspaceQuery{Unassigned: true}, 51},
		{TaskWorkspaceQuery{Priority: "urgent"}, 39}, {TaskWorkspaceQuery{RequirementID: h.requirement.ID}, 77},
		{TaskWorkspaceQuery{OwnerID: h.owner.ID, Priority: "urgent", RequirementID: h.requirement.ID}, 13},
		{TaskWorkspaceQuery{OwnerID: "unknown-owner"}, 0}, {TaskWorkspaceQuery{RequirementID: "unknown-requirement"}, 0},
		{TaskWorkspaceQuery{Priority: "' OR 1=1 --"}, 0},
	} {
		summary, err := f.s.TaskCollectionSummary(context.Background(), f.actor, f.project, entry.filters, TaskCollectionGroupOptions{GroupBy: "status"})
		if err != nil || summary.Total != entry.total {
			t.Fatalf("whole filtered counts %+v: %+v %v", entry.filters, summary, err)
		}
		page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, entry.filters)
		if err != nil || page.Total != entry.total || len(page.Items) > 100 {
			t.Fatalf("items disagree with summary %+v: %+v %v", entry.filters, page, err)
		}
	}
	for i, title := range []string{"100%_! Alpha 甲", "100%_! ALPHA 乙", "100whatever Alpha"} {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET title=? WHERE id=?`, title, h.tasks[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct {
		q     string
		total int
	}{{"100%_! alpha", 2}, {" 100%_!", 0}, {"Alpha 甲", 1}} {
		summary, err := f.s.TaskCollectionSummary(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Q: entry.q}, TaskCollectionGroupOptions{GroupBy: "status"})
		if err != nil || summary.Total != entry.total {
			t.Fatalf("literal query %q: %+v %v", entry.q, summary, err)
		}
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET description='任务说明甲乙',data='{"hidden":"payload-only"}' WHERE id=?`, h.tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		q     string
		total int
	}{{"甲乙", 1}, {"payload-only", 0}} {
		page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Q: entry.q})
		if err != nil || page.Total != entry.total {
			t.Fatalf("description scope %q: %+v %v", entry.q, page, err)
		}
	}
}

func TestTaskCollectionCursorBindsEveryFilterAndCanonicalWire(t *testing.T) {
	h := newTaskCollectionFixture(t, 30)
	f := h.f
	page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("first page: %+v %v", page, err)
	}
	for _, options := range []TaskWorkspaceQuery{
		{Q: "x", Limit: 1}, {OwnerID: h.owner.ID, Limit: 1},
		{Unassigned: true, Limit: 1}, {Priority: "high", Limit: 1},
		{RequirementID: h.requirement.ID, Limit: 1}, {Status: "todo", Limit: 1}, {Limit: 2}, {PriorityUnset: true, Limit: 1}, {StatusUnset: true, Limit: 1}, {Sort: "created_desc", Limit: 1},
	} {
		options.Cursor = page.NextCursor
		_, err = f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, options)
		requireAuthorityStatus(t, err, 400)
	}
	raw, err := base64.RawURLEncoding.DecodeString(page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range []string{" " + string(raw), string(raw[:len(raw)-1]) + `,"extra":true}`, string(raw[:len(raw)-1]) + `,"limit":1}`} {
		_, err = f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Limit: 1, Cursor: base64.RawURLEncoding.EncodeToString([]byte(wire))})
		requireAuthorityStatus(t, err, 400)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Other", "idempotency_key": "task-collection-other"}, 201)["id"].(string)
	_, err = f.s.ListTaskWorkspace(context.Background(), f.actor, other, TaskWorkspaceQuery{Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
}

func TestTaskCollectionHTTPRejectsAmbiguousOrUnboundedQueries(t *testing.T) {
	h := newTaskCollectionFixture(t, 1)
	f := h.f
	for _, suffix := range []string{"&owner_id=x&unassigned=true", "&unassigned=1", "&unassigned=TRUE", "&status=%20", "&status=%00", "&status=" + strings.Repeat("s", 201), "&priority=" + strings.Repeat("p", 201), "&priority=%0A", "&limit=0", "&limit=101", "&limit=01", "&limit=1e1", "&cursor=", "&extra=x", "&status=todo&status=blocked", "&project_id=x", "&q=%FF", "&q=%00", "&q=" + url.QueryEscape(strings.Repeat("甲", 201))} {
		f.call("GET", "/task-workspace?project_id="+f.project+suffix, nil, 400)
	}
	for _, suffix := range []string{"&group_by=unknown", "&sort=title_asc", "&limit=20", "&cursor=opaque", "&owner_id=x&unassigned=true"} {
		f.call("GET", "/task-collection/summary?project_id="+f.project+suffix, nil, 400)
	}
	f.call("GET", "/task-workspace?project_id="+f.project+"&unassigned=false&q="+url.QueryEscape(strings.Repeat("甲", 200)), nil, 200)
	f.call("GET", "/task-workspace?project_id="+f.project+"&status="+url.QueryEscape("等待外部供应商"), nil, 200)
	f.call("GET", "/task-collection/summary?project_id=unavailable", nil, 404)
}

func TestTaskCollectionCurrentHumanAPIAndMachineScope(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	seedTaskCollection(t, f, 30, h.owner.ID, h.resource.ID, h.requirement.ID, h.requirement.ID)
	first, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	_, err = f.s.ListTaskWorkspace(context.Background(), h.receiver, f.project, TaskWorkspaceQuery{Limit: 1, Cursor: first.NextCursor})
	requireAuthorityStatus(t, err, 400)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, issueErr := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: "task-collection-kind-" + scope})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		actor := domainAActor(t, f.s, issued.Token)
		for _, collection := range []string{"summary", "items"} {
			route := "/task-collection/summary"
			if collection == "items" {
				route = "/task-workspace"
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1"+route+"?project_id="+f.project, nil)
			req.Header.Set("Authorization", "Bearer "+issued.Token)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			want := 403
			if scope == "api" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("%s %s => %d: %s", scope, collection, w.Code, w.Body.String())
			}
		}
		_, readErr := f.s.TaskCollectionSummary(context.Background(), actor, f.project, TaskWorkspaceQuery{}, TaskCollectionGroupOptions{GroupBy: "status"})
		if scope == "api" {
			if readErr != nil {
				t.Fatal(readErr)
			}
		} else {
			requireAuthorityStatus(t, readErr, 403)
		}
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.TaskCollectionSummary(context.Background(), h.receiver, f.project, TaskWorkspaceQuery{}, TaskCollectionGroupOptions{GroupBy: "status"})
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.ListTaskWorkspace(context.Background(), h.receiver, f.project, TaskWorkspaceQuery{})
	requireAuthorityStatus(t, err, 403)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	_, err = f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Limit: 1, Cursor: first.NextCursor})
	requireAuthorityStatus(t, err, 401)
}

func TestTaskCollectionLiveAssignmentAndStatusChanges(t *testing.T) {
	h := newTaskCollectionFixture(t, 30)
	f := h.f
	filters := TaskWorkspaceQuery{OwnerID: h.owner.ID}
	first, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, taskCollectionPageQuery(filters, "", 2, ""))
	if err != nil || first.Total != 10 || first.NextCursor == "" {
		t.Fatalf("initial assigned scope: %+v %v", first, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET owner_id=?,updated_at=? WHERE id=?`, h.other.ID, now(), h.tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	next, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, taskCollectionPageQuery(filters, "", 2, first.NextCursor))
	if err != nil || next.Total != 9 {
		t.Fatalf("cursor falsely retained snapshot total: %+v %v", next, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET status='customer_validation',updated_at=? WHERE id=?`, now(), h.tasks[3].ID); err != nil {
		t.Fatal(err)
	}
	summary, err := f.s.TaskCollectionSummary(context.Background(), f.actor, f.project, filters, TaskCollectionGroupOptions{GroupBy: "status"})
	if err != nil || summary.Total != 9 {
		t.Fatalf("changed summary: %+v %v", summary, err)
	}
	custom := 0
	for _, group := range summary.Groups {
		if group.Key == "customer_validation" {
			custom = group.Total
		}
	}
	if custom != 1 {
		t.Fatalf("current business state absent: %+v", summary)
	}
	page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, taskCollectionPageQuery(filters, "customer_validation", 0, ""))
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != h.tasks[3].ID {
		t.Fatalf("current custom column: %+v %v", page, err)
	}
}

func TestTaskCollectionCanonicalGroupingAndExplicitEmptyValues(t *testing.T) {
	h := newTaskCollectionFixture(t, 6)
	f := h.f
	states := []string{"todo", "archived", "", "todo", "all", "customer_validation"}
	owners := []string{h.owner.ID, "", h.other.ID, "__unassigned__", "", h.owner.ID}
	priorities := []string{"high", "", "", "custom", "high", "urgent"}
	for index, item := range h.tasks {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status=?,owner_id=?,priority=? WHERE id=?`, states[index], owners[index], priorities[index], item.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, groupBy := range []string{"status", "owner", "priority"} {
		path := "/task-collection/summary?project_id=" + f.project + "&group_by=" + groupBy
		summary := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", path, nil, 200))
		if summary.Total != 6 || summary.GroupBy != groupBy {
			t.Fatalf("incomplete %s grouping: %+v", groupBy, summary)
		}
		seen := map[string]bool{}
		sum := 0
		for _, group := range summary.Groups {
			q := TaskWorkspaceQuery{Limit: 2}
			switch groupBy {
			case "status":
				q.Status, q.StatusUnset = group.Key, group.Key == ""
			case "owner":
				q.OwnerID, q.Unassigned = group.Key, group.Key == ""
			case "priority":
				q.Priority, q.PriorityUnset = group.Key, group.Key == ""
			}
			page, err := f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, q)
			if err != nil || page.Total != group.Total {
				t.Fatalf("summary/column compiler mismatch %s:%q: %+v %v", groupBy, group.Key, page, err)
			}
			for _, item := range page.Items {
				if seen[item.ID] {
					t.Fatalf("one task entered multiple %s groups", groupBy)
				}
				seen[item.ID] = true
			}
			sum += group.Total
		}
		if sum != 6 || len(seen) != 6 {
			t.Fatalf("missing grouped tasks %s: sum=%d loaded=%d", groupBy, sum, len(seen))
		}
	}
	archived := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", "/task-collection/summary?project_id="+f.project+"&group_by=owner&status=archived", nil, 200))
	if archived.Total != 1 || len(archived.Groups) != 1 || archived.Groups[0].Key != "" {
		t.Fatalf("global status or empty owner discarded: %+v", archived)
	}
	for _, entry := range []struct {
		query string
		total int
	}{
		{"unassigned=true", 2}, {"owner_id=__unassigned__", 1}, {"priority_unset=true", 2}, {"status_unset=true", 1}, {"status=all", 1}, {"status=archived", 1},
	} {
		page := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&"+entry.query, nil, 200))
		if page.Total != entry.total {
			t.Fatalf("literal vs unset %s: %+v", entry.query, page)
		}
	}
	for _, suffix := range []string{"&priority=high&priority_unset=true", "&status=todo&status_unset=true", "&priority_unset=1", "&status_unset=TRUE"} {
		f.call("GET", "/task-workspace?project_id="+f.project+suffix, nil, 400)
		f.call("GET", "/task-collection/summary?project_id="+f.project+suffix, nil, 400)
	}
}

func TestTaskCollectionBoundedGroupPagesAndScopedContinuation(t *testing.T) {
	h := newTaskCollectionFixture(t, 25)
	f := h.f
	ctx := context.Background()
	for index, item := range h.tasks {
		owner, priority := fmt.Sprintf("owner-%02d", index), fmt.Sprintf("priority-%02d", index)
		if index == 0 {
			owner, priority = "", ""
		}
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status=?,owner_id=?,priority=? WHERE id=?`, fmt.Sprintf("custom-%02d", index), owner, priority, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, groupBy := range []string{"status", "owner", "priority"} {
		options := TaskCollectionGroupOptions{GroupBy: groupBy, Limit: 2}
		seen := map[string]bool{}
		last := ""
		first := true
		for pages := 0; ; pages++ {
			if pages > 13 {
				t.Fatal("group continuation did not terminate")
			}
			summary, err := f.s.TaskCollectionSummary(ctx, f.actor, f.project, TaskWorkspaceQuery{}, options)
			if err != nil || summary.Total != 25 || summary.GroupsTotal != 25 || summary.GroupsLimit != 2 || len(summary.Groups) > 2 || summary.GroupBy != groupBy {
				t.Fatalf("unbounded or incomplete %s group page: %+v %v", groupBy, summary, err)
			}
			for _, group := range summary.Groups {
				if seen[group.Key] || !first && last >= group.Key || group.Total != 1 {
					t.Fatalf("duplicate/unordered group %s:%q", groupBy, group.Key)
				}
				seen[group.Key], last, first = true, group.Key, false
			}
			if summary.NextGroupCursor == "" {
				break
			}
			options.Cursor = summary.NextGroupCursor
		}
		if len(seen) != 25 {
			t.Fatalf("lost %s groups: %d", groupBy, len(seen))
		}
		if groupBy != "status" && !seen[""] {
			t.Fatalf("empty %s group lost", groupBy)
		}
	}
	emptyFirst, err := f.s.TaskCollectionSummary(ctx, f.actor, f.project, TaskWorkspaceQuery{}, TaskCollectionGroupOptions{GroupBy: "owner", Limit: 1})
	if err != nil || len(emptyFirst.Groups) != 1 || emptyFirst.Groups[0].Key != "" || emptyFirst.NextGroupCursor == "" {
		t.Fatalf("empty group cannot continue: %+v %v", emptyFirst, err)
	}
	afterEmpty, err := f.s.TaskCollectionSummary(ctx, f.actor, f.project, TaskWorkspaceQuery{}, TaskCollectionGroupOptions{GroupBy: "owner", Limit: 1, Cursor: emptyFirst.NextGroupCursor})
	if err != nil || len(afterEmpty.Groups) != 1 || afterEmpty.Groups[0].Key != "owner-01" {
		t.Fatalf("empty-key continuation skipped or repeated groups: %+v %v", afterEmpty, err)
	}
	base := TaskWorkspaceQuery{Q: "Collection "}
	options := TaskCollectionGroupOptions{GroupBy: "owner", Limit: 2}
	page, err := f.s.TaskCollectionSummary(ctx, f.actor, f.project, base, options)
	if err != nil || page.NextGroupCursor == "" {
		t.Fatalf("first scope: %+v %v", page, err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(page.NextGroupCursor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), base.Q) || len(page.NextGroupCursor) > 4096 {
		t.Fatal("group cursor exposes raw search or exceeds bound")
	}
	options.Cursor = page.NextGroupCursor
	for _, changed := range []TaskWorkspaceQuery{{Q: "different"}, {Q: base.Q, Status: "custom-01"}, {Q: base.Q, OwnerID: "owner-01"}, {Q: base.Q, Priority: "priority-01"}, {Q: base.Q, RequirementID: h.requirement.ID}, {Q: base.Q, Unassigned: true}, {Q: base.Q, PriorityUnset: true}, {Q: base.Q, StatusUnset: true}} {
		_, err = f.s.TaskCollectionSummary(ctx, f.actor, f.project, changed, options)
		requireAuthorityStatus(t, err, 400)
	}
	for _, changed := range []TaskCollectionGroupOptions{{GroupBy: "status", Limit: 2, Cursor: page.NextGroupCursor}, {GroupBy: "owner", Limit: 3, Cursor: page.NextGroupCursor}} {
		_, err = f.s.TaskCollectionSummary(ctx, f.actor, f.project, base, changed)
		requireAuthorityStatus(t, err, 400)
	}
	for _, wire := range []string{" " + string(raw), string(raw[:len(raw)-1]) + `,"unknown":true}`, string(raw[:len(raw)-1]) + `,"key":""}`} {
		changed := options
		changed.Cursor = base64.RawURLEncoding.EncodeToString([]byte(wire))
		_, err = f.s.TaskCollectionSummary(ctx, f.actor, f.project, base, changed)
		requireAuthorityStatus(t, err, 400)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Other group scope", "idempotency_key": "task-groups-other-scope"}, 201)["id"].(string)
	_, err = f.s.TaskCollectionSummary(ctx, f.actor, other, base, options)
	requireAuthorityStatus(t, err, 400)
	for _, suffix := range []string{"&group_limit=0", "&group_limit=101", "&group_limit=01", "&group_limit=", "&group_limit=1&group_limit=2", "&group_cursor=", "&group_cursor=invalid", "&group_by=owner&group_by=status"} {
		f.call("GET", "/task-collection/summary?project_id="+f.project+suffix, nil, 400)
	}
	firstHTTP := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", "/task-collection/summary?project_id="+f.project+"&group_by=owner&group_limit=2", nil, 200))
	if firstHTTP.Total != 25 || firstHTTP.GroupsTotal != 25 || len(firstHTTP.Groups) != 2 {
		t.Fatalf("HTTP group scope: %+v", firstHTTP)
	}
	f.call("GET", "/task-workspace?project_id="+f.project+"&group_limit=2", nil, 400)
	ownerCookie := f.cookie
	invite := governanceInvite(f, "task-group-reader@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invite, "task-group-reader@example.test")
	viewer := domainAActor(t, f.s, f.cookie.Value)
	_, err = f.s.TaskCollectionSummary(ctx, viewer, f.project, base, options)
	requireAuthorityStatus(t, err, 400)
	viewerPage, err := f.s.TaskCollectionSummary(ctx, viewer, f.project, base, TaskCollectionGroupOptions{GroupBy: "owner", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, viewer.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.TaskCollectionSummary(ctx, viewer, f.project, base, TaskCollectionGroupOptions{GroupBy: "owner", Limit: 2, Cursor: viewerPage.NextGroupCursor})
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(ctx, f.s, ownerCookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.TaskCollectionSummary(ctx, f.actor, f.project, base, options)
	requireAuthorityStatus(t, err, 401)
}
