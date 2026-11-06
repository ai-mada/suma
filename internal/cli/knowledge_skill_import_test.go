package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
)

type knowledgeImportFixture struct {
	kind, command, filename, raw, title, summary, content, credential string
}

func TestKnowledgeImportPreviewAndExplicitDraft(t *testing.T) {
	const title, summary, skill = "review-plan", "Review the exact candidate.", "# Steps\n\n- Review the diff.\n"
	const spec = "# 并发派发规范\n\n- Check the exact attempt.\n"
	fixtures := []knowledgeImportFixture{
		{kind: "skill", command: "import-skill", filename: "SKILL.md", title: title, summary: summary, content: skill, credential: summary,
			raw: "---\r\nname: " + title + "\r\ndescription: " + summary + "\r\nallowed-tools: Bash\r\nmetadata:\r\n  origin: private-fixture-only\r\n---\r\n" + strings.ReplaceAll(skill, "\n", "\r\n")},
		{kind: "spec", command: "import-helix-spec", filename: "spec.md", title: "并发派发规范", content: spec, credential: "Check the exact attempt.",
			raw: "\uFEFF" + strings.ReplaceAll(spec, "\n", "\r\n")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.kind, func(t *testing.T) { testKnowledgeImportPreviewAndExplicitDraft(t, fixture) })
	}
}

func testKnowledgeImportPreviewAndExplicitDraft(t *testing.T, fixture knowledgeImportFixture) {
	for _, name := range []string{"SUMA_TOKEN", "SUMA_EXPECTED_USER", "SUMA_EXPECTED_ORGANIZATION"} {
		t.Setenv(name, "")
	}
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open("sqlite", filepath.Join(dir, "knowledge-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &platform.Service{DB: db, Dialect: "sqlite", DataDir: dir}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	user, token, err := service.Setup(ctx, map[string]any{
		"name": "Skill import operator", "email": "skill-import@example.test",
		"password": "fixture-skill-import-password", "project": "Skill import fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	var project string
	if err := db.QueryRowContext(ctx, "SELECT id FROM suma_projects WHERE tenant_id=?", user.TenantID).Scan(&project); err != nil {
		t.Fatal(err)
	}

	handler := (&platform.Server{Service: service, Engine: engine, WebDir: dir}).Handler()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/project-knowledge" || r.URL.Query().Get("project_id") != project {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	run := func(input io.Reader, args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := Execute(ctx, append([]string{"--control", server.URL, "--language", "en-US"}, args...), input, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	filename := filepath.Join(dir, fixture.filename)
	if err := os.WriteFile(filename, []byte(fixture.raw), 0600); err != nil {
		t.Fatal(err)
	}
	preview := func(global []string, want map[string]any) string {
		t.Helper()
		code, out, stderr := run(nil, append(global, "knowledge", fixture.command, "--file", filename)...)
		var got map[string]any
		if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &got) != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected import preview: exit %d, stdout %q, stderr %q", code, out, stderr)
		}
		return out
	}
	want := map[string]any{"kind": fixture.kind, "title": fixture.title, "summary": fixture.summary, "content": fixture.content, "expected_version": float64(0), "sources": []any{}}
	preview(nil, want)
	want["project_id"] = project
	input := preview([]string{"--project", project}, want)
	if got := requests.Load(); got != 0 {
		t.Fatalf("preview made %d control requests", got)
	}

	if code, out, stderr := run(nil, "--token", "private-fixture-only", "knowledge", fixture.command, "--file", filename, "--file", filename); code != 2 || out != "" || !strings.Contains(stderr, "cli.knowledge."+strings.ReplaceAll(fixture.command, "-", "_")+"_usage") {
		t.Fatalf("duplicate input was not rejected: exit %d, stdout %q, stderr %q", code, out, stderr)
	}
	if code, out, stderr := run(nil, "--token", fixture.credential, "knowledge", fixture.command, "--file", filename); code != 2 || out != "" || strings.Contains(stderr, fixture.credential) {
		t.Fatalf("current credential leaked or was accepted: exit %d", code)
	}
	if requests.Load() != 0 {
		t.Fatal("invalid preview attempted a network request")
	}

	write := func(target string) (int, string, string) {
		return run(strings.NewReader(input), "--project", target, "--token", token, "--expected-user", user.ID,
			"--expected-organization", user.TenantID, "knowledge", "create", "--file", "-")
	}
	if code, out, stderr := write("wrong-project"); code != 2 || out != "" || !strings.Contains(stderr, "project_id") || requests.Load() != 0 {
		t.Fatalf("mismatched project was not rejected locally: exit %d, stdout %q, stderr %q", code, out, stderr)
	}
	create := func() platform.ProjectKnowledgeDetail {
		t.Helper()
		code, out, stderr := write(project)
		var saved platform.ProjectKnowledgeDetail
		if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &saved) != nil {
			t.Fatalf("unexpected create response: exit %d, stdout %q, stderr %q", code, out, stderr)
		}
		return saved
	}
	first, replay := create(), create()
	if first.ID == "" || replay.ID != first.ID || first.ProjectID != project || first.Kind != fixture.kind || first.State != "draft" ||
		first.LatestRevision != 1 || first.PublishedRevision != 0 || first.Published != nil || first.Latest.Method != "manual" ||
		first.Latest.Title != fixture.title || first.Latest.Summary != fixture.summary || first.Latest.Content != fixture.content || len(first.Latest.Sources) != 0 {
		t.Fatalf("unexpected draft or replay: first %+v, replay %+v", first.ProjectKnowledgeSummary, replay.ProjectKnowledgeSummary)
	}
	var entries, revisions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM suma_project_knowledge WHERE tenant_id=? AND project_id=?", user.TenantID, project).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM suma_project_knowledge_revisions WHERE tenant_id=? AND project_id=?", user.TenantID, project).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if entries != 1 || revisions != 1 || requests.Load() != 2 {
		t.Fatalf("expected one draft, one revision and two explicit requests, got %d, %d and %d", entries, revisions, requests.Load())
	}
}
