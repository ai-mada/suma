package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func applicationTeamFixture(t *testing.T, f *fixture) string {
	t.Helper()
	ctx := context.Background()
	team, err := f.s.SaveTeam(ctx, f.actor, "", map[string]any{"name": "Application accountable team", "status": "active", "expected_version": float64(0), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	id := team["id"].(string)
	if _, err = f.s.SaveTeamMember(ctx, f.actor, id, map[string]any{"resource_kind": "human", "resource_id": f.user.ID, "project_id": "", "role": "member", "expected_version": float64(0), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	return id
}
func applicationCreateInput(h *repositoryFixture, team, key, root string) ApplicationWriteRequest {
	return ApplicationWriteRequest{ProjectID: h.f.project, Key: key, Name: key, Kind: "application", OwnerTeamID: team, State: "active", CodeScopes: []ApplicationCodeScopeInput{{TargetID: h.target.ID, ExpectedTargetVersion: h.target.Version, RelativeRoot: root}}, Reason: "Explicit application code ownership", IdempotencyKey: newID()}
}
func TestProjectApplicationCatalogOwnershipOriginalReceiptsAndHierarchy(t *testing.T) {
	ctx := context.Background()
	h := newRepositoryFixture(t)
	f := h.f
	team := applicationTeamFixture(t, f)
	q := applicationCreateInput(h, team, "catalog-main", "apps/main")
	out, err := f.s.CreateProjectApplication(ctx, f.actor, f.project, q)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.CreateProjectApplication(ctx, f.actor, f.project, q)
	if err != nil || stringify(replay) != stringify(out) {
		t.Fatal("original command was not restored", err)
	}
	overlap := applicationCreateInput(h, team, "catalog-child", "apps/main/child")
	if _, err = f.s.CreateProjectApplication(ctx, f.actor, f.project, overlap); err == nil {
		t.Fatal("implicit overlapping ownership accepted")
	}
	save := q
	save.CodeScopes = append([]ApplicationCodeScopeInput{}, q.CodeScopes...)
	save.ExpectedVersion = out.Application.Version
	save.CodeScopes[0].ID = out.Application.CodeScopes[0].ID
	save.CodeScopes[0].Shared = true
	save.CodeScopes[0].Reason = "Both owners explicitly share this repository scope"
	save.IdempotencyKey = newID()
	out, err = f.s.SaveProjectApplication(ctx, f.actor, f.project, out.Application.ID, save)
	if err != nil {
		t.Fatal(err)
	}
	overlap.CodeScopes[0].Shared = true
	overlap.CodeScopes[0].Reason = "Explicit shared child"
	overlap.ParentApplicationID = out.Application.ID
	child, err := f.s.CreateProjectApplication(ctx, f.actor, f.project, overlap)
	if err != nil {
		t.Fatal(err)
	}
	cycle := save
	cycle.ExpectedVersion = out.Application.Version
	cycle.ParentApplicationID = child.Application.ID
	cycle.IdempotencyKey = newID()
	if _, err = f.s.SaveProjectApplication(ctx, f.actor, f.project, out.Application.ID, cycle); err == nil {
		t.Fatal("parent cycle accepted")
	}
	archive := save
	archive.ExpectedVersion = out.Application.Version
	archive.State = "archived"
	archive.IdempotencyKey = newID()
	if _, err = f.s.SaveProjectApplication(ctx, f.actor, f.project, out.Application.ID, archive); err == nil {
		t.Fatal("active children silently orphaned")
	}
	replay, err = f.s.CreateProjectApplication(ctx, f.actor, f.project, q)
	if err != nil || replay.Application.Version != 1 {
		t.Fatal("stale original receipt was recomputed", replay, err)
	}
	page, err := f.s.ListProjectApplications(ctx, f.actor, f.project, "", 1, "")
	if err != nil || len(page.Items) != 1 || page.Total != 2 || page.NextCursor == "" {
		t.Fatal("catalog pagination lost coverage", page, err)
	}
}
func TestProjectApplicationSourceRequiresIndependentBoundCandidate(t *testing.T) {
	ctx := context.Background()
	h := newRepositoryFixture(t)
	a, g := repositoryApplying(t, h)
	effect, err := h.f.s.BeginRepositoryEffect(ctx, h.secret, ri.RepositoryBeginEffect{AuthorizationID: g.AuthorizationID, OperationID: g.OperationID, Assignment: g.Assignment, RequestSHA256: g.RequestSHA256, Generation: g.Execution.Generation})
	if err != nil {
		t.Fatal(err)
	}
	h.complete(t, a, h.observation(t, g, "applied", effect.EffectGrantID))
	plan := h.current(t)
	if plan.State != "integrated" {
		t.Fatal("typed integration fixture did not apply", plan.State)
	}
	// Existing repository boundary fixture deliberately has no current bound
	// producer/test tool proof. An applied receipt alone must not register code.
	bound, auth, close, err := h.f.s.applicationRead(ctx, h.f.actor, h.f.project, "")
	if err != nil {
		t.Fatal(err)
	}
	target, err := h.f.s.repositoryTargetTx(bound, auth.tx, h.f.user.TenantID, h.f.project, h.target.ID)
	close()
	if err != nil {
		t.Fatal(err)
	}
	h.target = target
	team := applicationTeamFixture(t, h.f)
	app, err := h.f.s.CreateProjectApplication(ctx, h.f.actor, h.f.project, applicationCreateInput(h, team, "insufficient-source", ""))
	if err != nil {
		t.Fatal(err)
	}
	request := ApplicationVersionPreviewRequest{ProjectID: h.f.project, ExpectedApplicationVersion: app.Application.Version, Label: "not-accepted", Sources: []ApplicationVersionSourceInput{{ScopeID: app.Application.CodeScopes[0].ID, IntegrationPlanID: plan.ID, ExpectedIntegrationVersion: plan.Version}}, Dependencies: []ApplicationDependencyInput{}, AcknowledgeDependencyGaps: true, Reason: "Applied fixture lacks independent bound code proof"}
	if _, err = h.f.s.PreviewApplicationVersion(ctx, h.f.actor, h.f.project, app.Application.ID, request); err == nil {
		t.Fatal("applied receipt substituted missing current independent code evidence")
	}
}
func TestProjectApplicationHTTPRejectsAliasesNullAndMachineWrites(t *testing.T) {
	h := newRepositoryFixture(t)
	f := h.f
	team := applicationTeamFixture(t, f)
	q := applicationCreateInput(h, team, "http-app", "app")
	body := map[string]any{}
	json.Unmarshal([]byte(stringify(q)), &body)
	server := &Server{Service: f.s}
	request := httptest.NewRequest("POST", "/api/v1/applications", nil)
	w := httptest.NewRecorder()
	bad := map[string]any{}
	for k, v := range body {
		bad[k] = v
	}
	bad["Name"] = bad["name"]
	delete(bad, "name")
	server.projectApplicationsHTTP(w, request, f.actor, "", f.project, []string{"applications"}, bad)
	if w.Code != 400 {
		t.Fatal("case-folded key accepted", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	bad = map[string]any{}
	for k, v := range body {
		bad[k] = v
	}
	bad["code_scopes"] = nil
	server.projectApplicationsHTTP(w, request, f.actor, "", f.project, []string{"applications"}, bad)
	if w.Code != 400 {
		t.Fatal("null scopes accepted", w.Code)
	}
	if _, err := f.s.CreateProjectApplication(context.Background(), Actor{}, f.project, q); err == nil {
		t.Fatal("unproven machine actor wrote catalog")
	}
}
func TestProjectApplicationImmutableVersionAndPinnedDependencyFacts(t *testing.T) {
	ctx := context.Background()
	h := newRepositoryFixture(t)
	f := h.f
	team := applicationTeamFixture(t, f)
	a, err := f.s.CreateProjectApplication(ctx, f.actor, f.project, applicationCreateInput(h, team, "immutable-app", "app"))
	if err != nil {
		t.Fatal(err)
	}
	// This is a persistence-contract fixture, not a successful source inspection
	// or a release. Registration's source/effect gates are exercised separately.
	v := ApplicationCodeVersion{ID: newID(), ProjectID: f.project, ApplicationID: a.Application.ID, Sequence: 1, Label: "code-2026.10", Application: a.Application, Sources: []ApplicationVersionSource{}, Observations: []ApplicationDependencyObservation{}, Dependencies: []ApplicationDependency{}, DependencyCoverageComplete: false, DependencyGapsAcknowledged: true, RegisteredPreviewSHA256: strings.Repeat("a", 64), RegisteredBy: f.user.ID, RegisteredAt: now(), Reason: "Storage integrity fixture"}
	v.SnapshotSHA256 = applicationVersionDigest(v)
	if _, err = f.s.DB.ExecContext(ctx, f.s.q(`INSERT INTO suma_application_versions(id,tenant_id,project_id,application_id,sequence,label,label_key,snapshot_sha256,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?)`), v.ID, f.user.TenantID, f.project, v.ApplicationID, v.Sequence, v.Label, digest(v.Label), v.SnapshotSHA256, v.RegisteredAt, stringify(v)); err != nil {
		t.Fatal(err)
	}
	bounded, auth, close, err := f.s.applicationRead(ctx, f.actor, f.project, a.Application.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := f.s.applicationVersionTx(bounded, auth, f.project, a.Application.ID, v.ID)
	close()
	if err != nil || loaded.SnapshotSHA256 != v.SnapshotSHA256 {
		t.Fatal("immutable version unreadable", err)
	}
	changed := v
	changed.Reason = "tampered source fact"
	if _, err = f.s.DB.ExecContext(ctx, f.s.q(`UPDATE suma_application_versions SET payload=? WHERE id=?`), stringify(changed), v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetApplicationVersion(ctx, f.actor, f.project, a.Application.ID, v.ID); err == nil {
		t.Fatal("payload mutation without immutable digest was accepted")
	}
}
