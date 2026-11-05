package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func portfolioFixtureProject(t *testing.T, f *fixture, title, description string) Record {
	t.Helper()
	r, err := f.s.CreateProject(context.Background(), f.actor, map[string]any{
		"title": title, "description": description, "idempotency_key": newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func portfolioFixturePage(t *testing.T, f *fixture, actor Actor, search string, limit int, after string) ProjectPortfolioPage {
	t.Helper()
	page, err := f.s.ProjectPortfolio(context.Background(), actor, search, limit, after)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestProjectPortfolioProjectsHaveCompleteIndependentFacts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	empty := portfolioFixtureProject(t, f, "Empty project", "")
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, status := range []string{"accepted", "accepted", "blocked", "in_progress", "in_review"} {
		r := recordFrom(f.user, f.project, map[string]any{"title": "Task fixture", "status": status})
		if err = f.s.insertRecord(ctx, tx, r, "tasks"); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	for _, state := range []string{"open", "mitigating", "in_review", "closed"} {
		risk, _ := projectRiskCreateFixture(t, f, "", []ProjectRiskReference{})
		if _, err = f.s.DB.Exec(`UPDATE suma_risks SET state=? WHERE id=?`, state, risk.Risk.ID); err != nil {
			t.Fatal(err)
		}
	}
	period := func(path, title, start, end, target string) map[string]any {
		t.Helper()
		body := map[string]any{"title": title}
		if target != "" {
			body["target_date"] = target
		}
		if start != "" {
			body["start_date"], body["end_date"] = start, end
		}
		return f.call("POST", path, f.body(body), 201)
	}
	completed := period("/milestones", "Earlier completed", "", "", "2024-01-01")
	f.call("PATCH", "/milestones/"+completed["id"].(string), f.body(map[string]any{
		"status": "completed", "expected_version": completed["version"],
	}), 200)
	period("/iterations", "Earlier iteration", "2024-01-01", "2024-01-15", "")
	period("/milestones", "Later planned", "", "", "2027-01-01")
	first := period("/milestones", "Same-date A", "", "", "2025-02-03")
	second := period("/milestones", "Same-date B", "", "", "2025-02-03")
	f.call("PATCH", "/milestones/"+second["id"].(string), f.body(map[string]any{
		"status": "active", "expected_version": second["version"],
	}), 200)
	if first["id"].(string) > second["id"].(string) {
		first = second
	}

	page := portfolioFixturePage(t, f, f.actor, "", 20, "")
	if stamp, err := time.Parse(time.RFC3339, page.AsOf); err != nil || stamp.UTC().Format(time.RFC3339) != page.AsOf {
		t.Fatalf("as_of must be canonical UTC RFC3339: %q", page.AsOf)
	}
	if len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("unexpected project directory: %+v", page)
	}
	byID := map[string]ProjectPortfolioItem{}
	for _, item := range page.Items {
		byID[item.ProjectID] = item
	}
	if row := byID[empty.ID]; row.ProjectID == "" || row.TaskTotal != 0 || row.AcceptedTasks != 0 || row.BlockedTasks != 0 || row.ActiveRisks != 0 || row.NextMilestone != nil {
		t.Fatalf("projects without facts should remain visible with accurate empty values: %+v", row)
	}
	row := byID[f.project]
	wantMilestone := ProjectPortfolioMilestone{ID: first["id"].(string), Title: first["title"].(string), TargetDate: "2025-02-03"}
	if row.TaskTotal != 5 || row.AcceptedTasks != 2 || row.BlockedTasks != 1 || row.ActiveRisks != 3 ||
		row.NextMilestone == nil || *row.NextMilestone != wantMilestone {
		t.Fatalf("facts drifted from task, risk, or milestone sources: %+v", row)
	}
}

func TestProjectPortfolioLiteralSearchAndStableKeyset(t *testing.T) {
	f := newFixture(t)
	alpha := portfolioFixtureProject(t, f, "UI Project %_!", "前端 协作")
	beta := portfolioFixtureProject(t, f, "Ui Project beta", "ordinary")
	third := portfolioFixtureProject(t, f, "UI Project Gamma", "ordinary")
	tied := []string{alpha.ID, beta.ID}
	slices.Sort(tied)
	slices.Reverse(tied)
	for id, at := range map[string]string{
		alpha.ID: "2026-10-29T12:00:00.000000001Z",
		beta.ID:  "2026-10-29T12:00:00.000000001Z",
		third.ID: "2026-10-29T12:00:00Z",
	} {
		if _, err := f.s.DB.Exec(`UPDATE suma_projects SET created_at=? WHERE id=?`, at, id); err != nil {
			t.Fatal(err)
		}
	}
	want := append(tied, third.ID)
	cursor := ""
	for i, id := range want {
		page := portfolioFixturePage(t, f, f.actor, "  ui project  ", 1, cursor)
		if len(page.Items) != 1 || page.Items[0].ProjectID != id || (page.NextCursor != "") != (i < len(want)-1) {
			t.Fatalf("invalid order or pagination at %d: %+v", i, page)
		}
		cursor = page.NextCursor
	}
	for _, query := range []string{"%_!", "前端", "PROJECT %_!"} {
		page := portfolioFixturePage(t, f, f.actor, query, 20, "")
		if len(page.Items) != 1 || page.Items[0].ProjectID != alpha.ID {
			t.Fatalf("search must match title or description literally: %q %+v", query, page)
		}
	}
}

func TestProjectPortfolioOnlyCurrentOrganizationGrantsAndCursorAnchor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	visible := portfolioFixtureProject(t, f, "Accessible Shared Project", "")
	hidden := portfolioFixtureProject(t, f, "Hidden Shared Project", "")
	if _, err := f.s.DB.Exec(`UPDATE suma_projects SET created_at='2026-11-01T00:00:00Z' WHERE id=?`, visible.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_projects SET title='Also Shared Project',created_at='2026-01-01T00:00:00Z' WHERE id=?`, f.project); err != nil {
		t.Fatal(err)
	}
	email := "portfolio-member@example.test"
	invitation := governanceInvite(f, email, "member", []any{
		map[string]any{"project_id": f.project, "role": "viewer"},
		map[string]any{"project_id": visible.ID, "role": "reviewer"},
		map[string]any{"project_id": hidden.ID, "role": "viewer"},
	})
	governanceAccept(f, invitation, email)
	member := domainAActor(t, f.s, f.cookie.Value)
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='VIEWER' WHERE project_id=? AND user_id=?`, hidden.ID, member.credential.subjectID); err != nil {
		t.Fatal(err)
	}

	page := portfolioFixturePage(t, f, member, "Shared", 1, "")
	if len(page.Items) != 1 || page.Items[0].ProjectID != visible.ID || page.NextCursor == "" {
		t.Fatalf("authorized project missing: %+v", page)
	}
	next := portfolioFixturePage(t, f, member, "Shared", 1, page.NextCursor)
	if len(next.Items) != 1 || next.Items[0].ProjectID != f.project || next.NextCursor != "" {
		t.Fatalf("hidden or invalid-role project contributed to pagination: %+v", next)
	}
	for _, req := range []struct {
		actor Actor
		query string
		limit int
	}{{member, "Project", 1}, {member, "Shared", 2}, {f.actor, "Shared", 1}} {
		_, err := f.s.ProjectPortfolio(ctx, req.actor, req.query, req.limit, page.NextCursor)
		requireAuthorityStatus(t, err, 400)
	}
	forged := projectPortfolioCursor{
		Organization: f.user.TenantID, User: member.credential.subjectID, SearchSHA256: digest("Shared"), Limit: 1, ProjectID: hidden.ID,
	}
	token := base64.RawURLEncoding.EncodeToString([]byte(stringify(forged)))
	_, err := f.s.ProjectPortfolio(ctx, member, "Shared", 1, token)
	requireAuthorityStatus(t, err, 400)

	_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{
		"project_id": visible.ID, "user_id": member.credential.subjectID, "action": "remove", "expected_version": float64(1), "idempotency_key": newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ProjectPortfolio(ctx, member, "Shared", 1, page.NextCursor)
	requireAuthorityStatus(t, err, 400)
	fresh := portfolioFixturePage(t, f, member, "Accessible", 20, "")
	if fresh.Items == nil || len(fresh.Items) != 0 || fresh.NextCursor != "" {
		t.Fatalf("revoked project must be absent from a new request: %+v", fresh)
	}
}

func TestProjectPortfolioHTTPBrowserOnlyAndClosedQuery(t *testing.T) {
	f := newFixture(t)
	f.call("GET", "/auth/session", nil, 200)
	call := func(method, suffix, bearer string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/project-portfolio"+suffix, nil)
		if cookie != nil {
			req.AddCookie(cookie)
			req.Header.Set("X-Suma-Expected-User", f.view.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		reply := httptest.NewRecorder()
		f.handler.ServeHTTP(reply, req)
		if reply.Code != status {
			t.Fatalf("%s %s => %d, want %d: %s", method, suffix, reply.Code, status, reply.Body.String())
		}
		return reply
	}
	w := call(http.MethodGet, "?q=&limit=20", "", f.cookie, 200)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authenticated portfolio response must not be cached")
	}
	var page ProjectPortfolioPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].ProjectID != f.project {
		t.Fatalf("wrong authorized page: %+v %v", page, err)
	}
	for _, suffix := range []string{
		"?project_id=" + f.project, "?organization_id=other", "?q=a&q=b", "?q=%ZZ", "?q=%FF", "?q=%00", "?limit=", "?limit=0",
		"?limit=01", "?limit=51", "?limit=1&limit=1", "?after=", "?after=other", "?after=other&after=other",
		"?q=" + url.QueryEscape(strings.Repeat("界", 41)), "?q=" + strings.Repeat("x", 2049),
	} {
		call(http.MethodGet, suffix, "", f.cookie, 400)
	}
	w = call(http.MethodPost, "", "", f.cookie, 405)
	if w.Header().Get("Allow") != http.MethodGet {
		t.Fatal("method restriction must advertise GET")
	}
	call(http.MethodGet, "", "", nil, 401)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{
			ProjectID: f.project, Scope: scope, IdempotencyKey: newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodGet, "", issued.Token, nil, 403)
	}

	other := f.call("POST", "/organizations", map[string]any{"name": "Other", "idempotency_key": newID()}, 201)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": other["id"], "idempotency_key": newID()}, 200)
	w = call(http.MethodGet, "", "", f.cookie, 200)
	page = ProjectPortfolioPage{}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Items == nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatalf("tenant switch must expose only an empty current organization: %+v %v", page, err)
	}
}
