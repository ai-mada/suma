package platform

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	rp "github.com/ai-mada/suma/internal/repositoryprovider"
	"github.com/ai-mada/suma/internal/runner"
)

// One real source-to-catalog journey, with no model calls: a fixed Node command
// produces and transfers a candidate, another Worker tests its exact SHA, the
// fixture owner formally accepts it, and the real provider atomically applies
// it. Application versions are registered metadata, never deployment claims.
type actualApplicationFixture struct {
	f                          *fixture
	task                       Record
	delivery                   actualFixedCandidateEvidence
	manifests                  map[string]string
	control                    *httptest.Server
	target                     RepositoryIntegrationTarget
	plan                       RepositoryIntegrationPlan
	library, web               ProjectApplication
	libraryVersion, webVersion ApplicationVersionView
	libraryPreview             ApplicationVersionPreview
	webRequest                 ApplicationVersionPreviewRequest
	preview                    func(ProjectApplication, ApplicationVersionPreviewRequest) ApplicationVersionPreview
	register                   func(ProjectApplication, ApplicationVersionPreviewRequest, ApplicationVersionPreview) ApplicationVersionView
}

func newActualApplicationFixture(t *testing.T) *actualApplicationFixture {
	t.Helper()
	ctx := context.Background()
	f := newFixture(t)
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "Owned workspace application catalog", "description": "Register exact repository code scopes after an independently tested correction.", "acceptance_criteria": []string{"The fixed candidate returns the correct sum and retains the declared package manifests"}}), 201)
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Prepare the application source candidate", "description": "Owned deterministic fixture, not an AI or vendor execution.", "requirement_id": requirement["id"], "acceptance_criteria": []string{"The fixed candidate returns the correct sum and retains the declared package manifests"}}), 201))
	manifests := map[string]string{
		"package.json":                  `{"name":"application-fixture","version":"1.0.0","private":true,"workspaces":["apps/*","packages/*"]}` + "\n",
		"pnpm-workspace.yaml":           "packages:\n  - 'apps/*'\n  - 'packages/*'\n",
		"packages/library/package.json": `{"name":"@fixture/library","version":"1.2.0"}` + "\n",
		"apps/web/package.json":         `{"name":"@fixture/web","version":"2.0.0","dependencies":{"@fixture/library":"^1.2.0"}}` + "\n",
	}
	delivery := actualFixedCandidateDelivery(t, f, task, manifests, true)
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	control := httptest.NewServer(f.handler)
	t.Cleanup(control.Close)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(t.TempDir(), "application-target.git")
	workspaceGitTestCommand(t, delivery.Repository, "clone", "--bare", "--no-hardlinks", delivery.Repository, bare)
	const providerID = "application-provider123"
	for key, value := range map[string]string{"SUMA_APP_CONTROL123": strings.Repeat("c", 48), "SUMA_APP_RUNNER123": strings.Repeat("r", 48), "SUMA_APP_RECONCILE123": strings.Repeat("x", 48), "SUMA_APP_SOURCE123": strings.Repeat("s", 48)} {
		t.Setenv(key, value)
	}
	provider, err := rp.New(rp.Config{ProviderID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, ControlURL: control.URL, ControlTokenEnv: "SUMA_APP_CONTROL123", RunnerTokenEnv: "SUMA_APP_RUNNER123", ReconcileTokenEnv: "SUMA_APP_RECONCILE123", SourceReadTokenEnv: "SUMA_APP_SOURCE123", GitExecutable: git, Targets: map[string]rp.TargetConfig{"main": {BareRepository: bare, Ref: "refs/heads/main"}}, StateDir: filepath.Join(t.TempDir(), "provider-state")})
	if err != nil {
		t.Fatal("start actual local repository provider", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	providerHTTP := httptest.NewServer(provider.Handler())
	t.Cleanup(providerHTTP.Close)
	f.s.RepositoryProviders = []RepositoryProviderConfig{{ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, TargetAliases: []string{"main"}, TokenHash: digest(os.Getenv("SUMA_APP_CONTROL123")), Origin: providerHTTP.URL, ReconcileToken: os.Getenv("SUMA_APP_RECONCILE123"), SourceReadToken: os.Getenv("SUMA_APP_SOURCE123")}}
	target := issueRepairRuntimeDecode[RepositoryIntegrationTarget](t, f.call("POST", "/repository-integration-targets", RepositoryIntegrationTargetCreateRequest{ProjectID: f.project, Name: "Actual governed application repository", ProviderID: providerID, TargetAlias: "main", IdempotencyKey: newID()}, 201))
	plan := issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, f.call("POST", "/repository-integrations/plans", RepositoryIntegrationPlanCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, SourceNodeID: delivery.SourceNodeID, TargetID: target.ID, ExpectedTargetVersion: target.Version, IdempotencyKey: newID()}, 201))
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "application-integration-worker123", "profiles": []any{}, "deployment": false}), 201)
	client := &runner.Client{BaseURL: control.URL, Token: worker["token"].(string), ProjectID: f.project}
	integrationCfg := runner.ExecConfig{AllowExec: true, Artifacts: client, External: map[string]runner.ExternalSpec{providerID: {URL: providerHTTP.URL, TokenEnv: "SUMA_APP_RUNNER123", Protocol: ri.RepositoryProtocol}}}
	completePhase := func(phase, runID string) {
		t.Helper()
		assignment, err := client.Claim(ctx, "application-integration-worker123", []string{"external_workflow:" + providerID})
		if err != nil || assignment == nil || assignment.RunID != runID || assignment.RepositoryIntegration == nil || assignment.RepositoryIntegration.Phase != phase {
			t.Fatal("actual repository phase claim missing", phase, err)
		}
		result := runner.Execute(ctx, *assignment, integrationCfg)
		result.OperationID = newID()
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown {
			// Keep the original accepted observation reason: a new provider GET
			// may replace it with the generic missing-Git-receipt observation.
			var observation ri.RepositoryObservation
			if evidence := result.IntegrationEvidence; evidence != nil {
				var raw string
				if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT data FROM suma_repository_observations WHERE tenant_id=? AND project_id=? AND operation_id=? AND version=?`), f.user.TenantID, f.project, evidence.OperationID, evidence.Version).Scan(&raw); err == nil {
					_ = json.Unmarshal([]byte(raw), &observation)
				}
			}
			t.Fatalf("actual repository %s failed: exit=%d error=%s output=%q provider_phase=%q provider_state=%q provider_reason=%q", phase, result.ExitCode, result.Error, result.Output, observation.Phase, observation.State, observation.Reason)
		}
		if err = client.Complete(ctx, result); err != nil {
			t.Fatal("actual repository result rejected", phase, err)
		}
	}
	completePhase("prepare", plan.PrepareRunID)
	plan = issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, f.call("GET", "/repository-integrations/plans/"+plan.ID+"?project_id="+f.project, nil, 200))
	if plan.State != "ready" || plan.Preview == nil || plan.Preview.CandidateHead != delivery.CandidateCommit {
		t.Fatal("actual repository preparation did not preserve the candidate", plan.State)
	}
	command := func(action string) {
		t.Helper()
		plan = issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, f.call("POST", "/repository-integrations/plans/"+plan.ID+"/commands", RepositoryIntegrationPlanCommandRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Action: action, Acknowledge: true, Reason: "Fixture owner explicitly " + action + "s the exact prepared repository plan; no production approval is claimed.", IdempotencyKey: newID()}, 200))
	}
	command("approve")
	command("execute")
	completePhase("apply", plan.ApplyRunID)
	plan = issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, f.call("GET", "/repository-integrations/plans/"+plan.ID+"?project_id="+f.project, nil, 200))
	if plan.State != "integrated" || plan.Observation == nil || plan.Observation.State != "applied" || plan.Observation.Receipt == nil || plan.Observation.Receipt.ResultCommit != delivery.CandidateCommit {
		t.Fatal("real repository apply receipt missing", plan.State)
	}
	if actual := workspaceGitTestCommand(t, bare, "rev-parse", "refs/heads/main"); actual != delivery.CandidateCommit {
		t.Fatal("actual target ref differs from applied receipt")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "packages", "source-inspector", "dist", "source-inspector.bundle.mjs")
	f.s.ApplicationSourceInspector, err = LoadApplicationSourceInspector(func(name string) string {
		switch name {
		case "SUMA_SOURCE_INSPECTOR_NODE":
			return node
		case "SUMA_SOURCE_INSPECTOR_ENTRY":
			return entry
		}
		return ""
	})
	if err != nil {
		t.Fatal("the fixed actual source parser must be built before this journey", err)
	}
	team := applicationTeamFixture(t, f)
	targets, err := f.s.ListRepositoryIntegrationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets) != 1 {
		t.Fatal("current governed target missing", err)
	}
	target = targets[0]
	createApp := func(key, name, path string) ProjectApplication {
		t.Helper()
		out := issueRepairRuntimeDecode[ProjectApplicationView](t, f.call("POST", "/applications", ApplicationWriteRequest{ProjectID: f.project, Key: key, Name: name, Description: "Actual governed source; registration is distinct from release or deployment.", Kind: "application", OwnerTeamID: team, State: "active", CodeScopes: []ApplicationCodeScopeInput{{TargetID: target.ID, ExpectedTargetVersion: target.Version, RelativeRoot: path}}, Reason: "Fixture owner explicitly assigns this exact code subtree to the accountable team.", IdempotencyKey: newID()}, 201))
		return out.Application
	}
	library := createApp("library123", "Shared library", "packages/library")
	web := createApp("web123", "Web application", "apps/web")
	request := func(app ProjectApplication, label string) ApplicationVersionPreviewRequest {
		return ApplicationVersionPreviewRequest{ProjectID: f.project, ExpectedApplicationVersion: app.Version, Label: label, Sources: []ApplicationVersionSourceInput{{ScopeID: app.CodeScopes[0].ID, IntegrationPlanID: plan.ID, ExpectedIntegrationVersion: plan.Version}}, Dependencies: []ApplicationDependencyInput{}, Reason: "Inspect the exact applied commit and review only the supported manifest facts."}
	}
	preview := func(app ProjectApplication, req ApplicationVersionPreviewRequest) ApplicationVersionPreview {
		t.Helper()
		return issueRepairRuntimeDecode[ApplicationVersionPreview](t, f.call("POST", "/applications/"+app.ID+"/versions/preview", req, 200))
	}
	register := func(app ProjectApplication, req ApplicationVersionPreviewRequest, p ApplicationVersionPreview) ApplicationVersionView {
		t.Helper()
		if !p.CanRegister {
			t.Fatal("explicit version preview is not registerable", p.Blockers)
		}
		return issueRepairRuntimeDecode[ApplicationVersionView](t, f.call("POST", "/applications/"+app.ID+"/versions", ApplicationVersionRegisterRequest{ApplicationVersionPreviewRequest: req, ExpectedPreviewSHA256: p.PreviewSHA256, IdempotencyKey: newID()}, 201))
	}
	libraryRequest := request(library, "library-1.2.0")
	libraryPreview := preview(library, libraryRequest)
	if len(libraryPreview.Version.Sources) != 1 || len(libraryPreview.Version.Sources[0].Inspection.Manifests) != 1 || libraryPreview.Version.Sources[0].Commit != delivery.CandidateCommit || libraryPreview.Version.Sources[0].Inspection.Manifests[0].PackageName != "@fixture/library" || libraryPreview.Version.Sources[0].Inspection.ParserSHA256 == "" {
		t.Fatal("library source was not read and parsed from the exact real Git subtree", libraryPreview)
	}
	libraryVersion := register(library, libraryRequest, libraryPreview)
	webRequest := request(web, "web-2.0.0")
	webPreview := preview(web, webRequest)
	if len(webPreview.Version.Observations) != 1 {
		t.Fatal("actual parser did not retain the web dependency declaration", webPreview)
	}
	observation := webPreview.Version.Observations[0]
	if observation.Name != "@fixture/library" || observation.DeclaredRange != "^1.2.0" || observation.RangeState != "valid" || len(observation.CandidateApplicationIDs) != 1 || observation.CandidateApplicationIDs[0] != library.ID {
		t.Fatal("the declared dependency was not matched to the registered application", observation)
	}
	webRequest.Dependencies = []ApplicationDependencyInput{{TargetApplicationID: library.ID, TargetVersionID: libraryVersion.Version.ID, TargetVersionSHA256: libraryVersion.Version.SnapshotSHA256, ObservationID: observation.ID, DeclaredRange: observation.DeclaredRange, Reason: "Fixture owner pins this exact registered library version; runtime and external lock resolution remain unverified."}}
	webRequest.AcknowledgeDependencyGaps = true
	webPreview = preview(web, webRequest)
	webVersion := register(web, webRequest, webPreview)
	return &actualApplicationFixture{f: f, task: task, delivery: delivery, manifests: manifests, control: control, target: target, plan: plan, library: library, web: web, libraryVersion: libraryVersion, webVersion: webVersion, libraryPreview: libraryPreview, webRequest: webRequest, preview: preview, register: register}
}

func TestProjectApplicationActualIntegratedSourceAndPinnedVersions(t *testing.T) {
	h := newActualApplicationFixture(t)
	f, task, delivery, manifests, target, plan := h.f, h.task, h.delivery, h.manifests, h.target, h.plan
	library, web, libraryVersion, webVersion, libraryPreview, webRequest := h.library, h.web, h.libraryVersion, h.webVersion, h.libraryPreview, h.webRequest
	preview, register := h.preview, h.register
	graph := issueRepairRuntimeDecode[ApplicationDependencyGraph](t, f.call("GET", "/applications/graph?project_id="+f.project, nil, 200))
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Edges[0].TargetVersionID != libraryVersion.Version.ID || graph.Edges[0].TargetVersionSHA256 != libraryVersion.Version.SnapshotSHA256 {
		t.Fatal("the current graph did not preserve the exact selected library version", graph)
	}
	impact := issueRepairRuntimeDecode[ApplicationImpact](t, f.call("POST", "/applications/impact", ApplicationImpactRequest{ProjectID: f.project, ApplicationID: library.ID, ExpectedGraphSHA256: graph.SHA256}, 200))
	if len(impact.Items) != 1 || impact.Items[0].ApplicationID != web.ID {
		t.Fatal("direct dependency impact did not locate the web application", impact)
	}
	currentWeb := issueRepairRuntimeDecode[ProjectApplicationView](t, f.call("GET", "/applications/"+web.ID+"?project_id="+f.project, nil, 200)).Application
	revised := webRequest
	revised.ExpectedApplicationVersion = currentWeb.Version
	revised.Label = "web-2.0.0-reviewed"
	revised.Reason = "Explicitly register a reviewed dependency-policy revision without changing the original source or historical version."
	revised.Dependencies = append([]ApplicationDependencyInput{}, webRequest.Dependencies...)
	revised.Dependencies[0].DeclaredRange = ">=1.2.0 <2.0.0"
	revised.Dependencies[0].Reason = "Human policy correction, retaining the original observed ^1.2.0 declaration and exact library version."
	second := register(currentWeb, revised, preview(currentWeb, revised))
	if second.Version.ID == webVersion.Version.ID || second.Version.Sequence != 2 {
		t.Fatal("an explicit revision mutated the first immutable application version")
	}
	originalVersion := issueRepairRuntimeDecode[ApplicationVersionView](t, f.call("GET", "/applications/"+web.ID+"/versions/"+webVersion.Version.ID+"?project_id="+f.project, nil, 200))
	if len(originalVersion.Version.Dependencies) != 1 || originalVersion.Version.Dependencies[0].DeclaredRange != "^1.2.0" {
		t.Fatal("human correction rewrote historical dependency facts")
	}
	currentWeb = issueRepairRuntimeDecode[ProjectApplicationView](t, f.call("GET", "/applications/"+web.ID+"?project_id="+f.project, nil, 200)).Application
	staleRequest := revised
	staleRequest.ExpectedApplicationVersion = currentWeb.Version
	staleRequest.Label = "stale-scope-preview"
	stalePreview := preview(currentWeb, staleRequest)
	edit := ApplicationWriteRequest{ProjectID: f.project, ExpectedVersion: currentWeb.Version, Key: currentWeb.Key, Name: currentWeb.Name, Description: currentWeb.Description, Kind: currentWeb.Kind, ParentApplicationID: currentWeb.ParentApplicationID, OwnerTeamID: currentWeb.OwnerTeamID, State: currentWeb.State, CodeScopes: []ApplicationCodeScopeInput{{ID: currentWeb.CodeScopes[0].ID, TargetID: target.ID, ExpectedTargetVersion: target.Version, RelativeRoot: "apps", Reason: "Explicitly broaden the application scope for the next separately reviewed version."}}, Reason: "Fixture owner changes the governed code scope; the old source preview must become stale.", IdempotencyKey: newID()}
	f.call("PATCH", "/applications/"+web.ID, edit, 200)
	f.call("POST", "/applications/"+web.ID+"/versions", ApplicationVersionRegisterRequest{ApplicationVersionPreviewRequest: staleRequest, ExpectedPreviewSHA256: stalePreview.PreviewSHA256, IdempotencyKey: newID()}, 409)
	historical := issueRepairRuntimeDecode[ApplicationVersionView](t, f.call("GET", "/applications/"+web.ID+"/versions/"+second.Version.ID+"?project_id="+f.project, nil, 200))
	if historical.BasisCurrent || historical.Version.SnapshotSHA256 != second.Version.SnapshotSHA256 {
		t.Fatal("source scope drift failed to invalidate applicability or changed immutable history")
	}
	if raw, _ := json.Marshal(historical); strings.Contains(string(raw), "content_base64") || strings.Contains(string(raw), os.Getenv("SUMA_APP_SOURCE123")) {
		t.Fatal("public application version leaked source transport or provider credential")
	}
	receipt := map[string]any{"stage": "T-APPLICATION-123", "task_id": task.ID, "run_id": delivery.RunID, "candidate_commit": delivery.CandidateCommit, "base_commit": delivery.BaseCommit, "repository_target_id": target.ID, "integration_plan_id": plan.ID, "integration_receipt_sha256": plan.Observation.ReceiptSHA256, "library_id": library.ID, "library_version_id": libraryVersion.Version.ID, "library_version_sha256": libraryVersion.Version.SnapshotSHA256, "web_id": web.ID, "web_original_version_id": webVersion.Version.ID, "web_reviewed_version_id": second.Version.ID, "source_snapshot_sha256": libraryPreview.Version.Sources[0].Inspection.SnapshotSHA256, "parser_sha256": libraryPreview.Version.Sources[0].Inspection.ParserSHA256, "independent_junit_sha256": delivery.ReportSHA256, "model_calls": 0, "customer_human_signoff": false}
	t.Logf("project_application_actual_receipt=%s", stringify(receipt))
	actualApplicationRuntimeHold(t, f, delivery, manifests, receipt, actualApplicationHoldOptions{CacheDirectory: "application-123", DirectoryEnvironment: "SUMA_APPLICATION_UI_DIR", WebEnvironment: "SUMA_APPLICATION_WEB_DIR", Hostname: "applications123.localhost", Kind: "actual_application_source_version_delivery"})
}

type actualApplicationHoldOptions struct {
	CacheDirectory, DirectoryEnvironment, WebEnvironment, Hostname, Kind string
	AdditionalTables, ProtectedQueries                                   []string
}

func actualApplicationRuntimeHold(t *testing.T, f *fixture, source actualFixedCandidateEvidence, extra map[string]string, receipt map[string]any, options actualApplicationHoldOptions) {
	t.Helper()
	dir, web := os.Getenv(options.DirectoryEnvironment), os.Getenv(options.WebEnvironment)
	if dir == "" && web == "" {
		return
	}
	if dir == "" || web == "" {
		t.Fatal("both owned application GUI directories are required")
	}
	if options.CacheDirectory != "application-123" && options.CacheDirectory != "release-set-124" && options.CacheDirectory != "rollout-125" {
		t.Fatal("unknown owned application evidence stage")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".cache", "e2e", options.CacheDirectory))
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil || dir == root || !under(root, dir) {
		t.Fatal("application receipts must stay in the exact owned cache")
	}
	web, err = filepath.Abs(web)
	if err != nil || web == root || !under(root, web) || under(web, dir) || under(dir, web) {
		t.Fatal("application web assets and private receipts must be separate owned paths")
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("build the final application UI before the unique actual journey", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{dir, web} {
		actual, err := filepath.EvalSymlinks(value)
		if err != nil || !under(resolved, actual) {
			t.Fatal("application GUI paths must resolve inside the exact cache")
		}
	}
	for _, name := range []string{"login.local.json", "login.ready", "stop.request", "result.public.json", "fixture.sqlite", "base-source.tar", "candidate-source.tar", "retained-data"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatal("application evidence directory must be fresh", name)
		}
	}
	server, origin, port := modelSelectionGUIServer(t, f, web, options.Hostname)
	counts := func() map[string]int {
		out := modelSelectionGUICounts(t, f)
		for _, table := range []string{"suma_tasks", "suma_requirements", "suma_test_plan_reports", "dag_events", "suma_project_applications", "suma_application_versions", "suma_repository_plans", "suma_repository_observations"} {
			out[table] = expertCount(t, f, table)
		}
		for _, table := range options.AdditionalTables {
			out[table] = expertCount(t, f, table)
		}
		return out
	}
	protected := func() string {
		values := [][]string{}
		queries := append([]string{`SELECT payload FROM suma_project_applications ORDER BY id`, `SELECT payload FROM suma_application_versions ORDER BY id`}, options.ProtectedQueries...)
		for _, query := range queries {
			rows, err := f.s.DB.Query(query)
			if err != nil {
				t.Fatal(err)
			}
			items := []string{}
			for rows.Next() {
				var raw string
				if err = rows.Scan(&raw); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				items = append(items, raw)
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			rows.Close()
			values = append(values, items)
		}
		return dispatchRecommendationRuntimeHash([]byte(stringify(values)))
	}
	seed, seedSHA := counts(), protected()
	state := map[string]any{"status": "ready", "kind": options.Kind, "origin": origin, "receipt": receipt, "seed_counts": seed, "seed_protected_sha256": seedSHA, "model_calls": 0, "customer_human_signoff": false}
	t.Cleanup(func() {
		server.Close()
		final, finalSHA := counts(), protected()
		state["final_counts"], state["final_protected_sha256"] = final, finalSHA
		if !reflect.DeepEqual(seed, final) || seedSHA != finalSHA {
			state["status"] = "read_only_guard_failed"
			t.Error("read-only browser inspection changed application/source/version facts")
		}
		allow := map[string]bool{".gitignore": true, "sum.cjs": true}
		for path := range extra {
			allow[path] = true
		}
		archives := []map[string]any{}
		for _, item := range []struct{ name, commit string }{{"base-source.tar", source.BaseCommit}, {"candidate-source.tar", source.CandidateCommit}} {
			path := filepath.Join(dir, item.name)
			workspaceGitTestCommand(t, source.Repository, "archive", "--format=tar", "--output="+path, item.commit)
			file, err := os.Open(path)
			if err != nil {
				t.Error(err)
				continue
			}
			reader := tar.NewReader(file)
			entries := []map[string]any{}
			for {
				header, err := reader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Error(err)
					break
				}
				if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader {
					continue
				}
				if !allow[header.Name] || header.Size < 0 || header.Size > 1<<20 || header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
					t.Error("unexpected owned source archive entry")
					break
				}
				content, err := io.ReadAll(io.LimitReader(reader, 1<<20+1))
				if err != nil || int64(len(content)) != header.Size {
					t.Error("source archive content incomplete")
					break
				}
				entries = append(entries, map[string]any{"path": header.Name, "bytes": len(content), "sha256": dispatchRecommendationRuntimeHash(content)})
			}
			file.Close()
			if len(entries) != len(allow) {
				t.Error("owned application source archive was incomplete")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Error(err)
				continue
			}
			archives = append(archives, map[string]any{"archive": item.name, "commit": item.commit, "sha256": dispatchRecommendationRuntimeHash(raw), "bytes": len(raw), "files": entries})
		}
		state["source_archives"] = archives
		modelSelectionGUIJSON(t, filepath.Join(dir, "source.manifest.public.json"), map[string]any{"archives": archives})
		state["retained_data"] = applicationRuntimeRetainData(t, f.s.DataDir, filepath.Join(dir, "retained-data"))
		database := filepath.Join(dir, "fixture.sqlite")
		if _, err := f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(database, "'", "''") + "'"); err != nil {
			t.Error("retain unique application database", err)
		} else if raw, err := os.ReadFile(database); err != nil {
			t.Error(err)
		} else {
			state["database_sha256"], state["database_bytes"] = fmt.Sprintf("%x", sha256.Sum256(raw)), len(raw)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"kind": options.Kind, "allowed_actions": []string{"read", "refresh"}, "origin": origin, "base_url": server.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "user_id": f.user.ID, "receipt": receipt})
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "kind": options.Kind, "origin": origin, "port": port, "seed_counts": seed, "model_calls": 0})
	t.Logf("actual application GUI ready: %s; read/refresh only", origin)
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	state["stop_reason"] = "timeout"
	for waiting := true; waiting; {
		select {
		case <-timer.C:
			waiting = false
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop.request")); err == nil {
				state["stop_reason"] = "explicit_stop"
				waiting = false
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	state["status"] = "closed_without_new_execution"
}

// Preserve immutable bundle/report bytes referenced by the retained database.
// The input is this fixture's owned DataDir; links and special files are never
// traversed or copied, and receipts stay outside the disposable source tree.
func applicationRuntimeRetainData(t *testing.T, source, destination string) []map[string]any {
	t.Helper()
	manifest := []map[string]any{}
	var total int64
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destination, 0700)
		}
		target := filepath.Join(destination, relative)
		if !under(destination, target) || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("owned retained data contains a linked or escaped path")
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 || len(manifest) >= 512 || total+info.Size() > 128<<20 {
			return fmt.Errorf("owned retained data exceeded its bound")
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, info.Size()+1))
		closeErr := output.Close()
		if err != nil || closeErr != nil || size != info.Size() {
			return fmt.Errorf("retained data changed during copy")
		}
		total += size
		manifest = append(manifest, map[string]any{"path": filepath.ToSlash(relative), "bytes": size, "sha256": fmt.Sprintf("%x", hash.Sum(nil))})
		return nil
	})
	if err != nil {
		t.Error("retain application bundle/report bytes", err)
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i]["path"].(string) < manifest[j]["path"].(string) })
	return manifest
}
