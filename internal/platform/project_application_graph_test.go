package platform

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// One diamond with a real cycle checks reverse impact direction and deduplication;
// an exact historical pin remains valid while genuinely missing versions are gaps.
func TestApplicationGraphImpactPreservesPinsAndCycles(t *testing.T) {
	records := []applicationGraphRecord{}
	refs := map[string]applicationGraphVersionRef{}
	for _, id := range []string{"a", "b", "c", "d"} {
		version := &applicationVersionGraphProjection{ProjectID: "p", ApplicationID: id, VersionID: id + "v", VersionSHA256: digest(id), Label: id, Dependencies: []ApplicationDependency{}, DependencyCoverageComplete: true}
		records = append(records, applicationGraphRecord{app: ProjectApplication{ID: id, Name: id, ProjectID: "p", Kind: "service", State: "active"}, version: version})
		refs[id+"v"] = applicationGraphVersionRef{app: id, sha: digest(id)}
	}
	pin := func(from int, to string) {
		records[from].version.Dependencies = append(records[from].version.Dependencies, ApplicationDependency{ApplicationDependencyInput: ApplicationDependencyInput{TargetApplicationID: to, TargetVersionID: to + "v", TargetVersionSHA256: digest(to)}})
	}
	pin(1, "a")
	pin(2, "a")
	pin(3, "b")
	pin(3, "c")
	pin(1, "d")
	graph := buildApplicationGraph("p", records, 4, refs)
	impact, err := applicationGraphImpact(graph, "a")
	if err != nil || !impact.Complete || len(impact.Items) != 3 || !reflect.DeepEqual(graph.Cycles, [][]string{{"b", "d"}}) {
		t.Fatal("diamond/cycle not preserved", impact, err)
	}
	for _, item := range impact.Items {
		if item.Path[0] != "a" || item.Path[len(item.Path)-1] != item.ApplicationID {
			t.Fatal("impact path reversed", item)
		}
	}
	refs["old"] = applicationGraphVersionRef{app: "a", sha: digest("old")}
	records[1].version.Dependencies[0].TargetVersionID = "old"
	records[1].version.Dependencies[0].TargetVersionSHA256 = digest("old")
	historical := buildApplicationGraph("p", records, 4, refs)
	found := false
	for _, edge := range historical.Edges {
		if edge.TargetVersionID == "old" {
			found = edge.State == "historical_version"
		}
	}
	if !found {
		t.Fatal("historical pin incorrectly follows current pointer")
	}
	delete(refs, "old")
	missing := buildApplicationGraph("p", records, 4, refs)
	impact, err = applicationGraphImpact(missing, "a")
	if err != nil || impact.Complete {
		t.Fatal("missing immutable reference hidden", err)
	}
	truncated := buildApplicationGraph("p", records[:2], 5, refs)
	if truncated.Complete || truncated.EdgeTotalExact || truncated.ApplicationTotal != 5 {
		t.Fatal("bounded graph claimed complete totals")
	}
	records[0].version.DependencyCoverageComplete = false
	impact, _ = applicationGraphImpact(buildApplicationGraph("p", records, 4, refs), "a")
	if impact.Complete {
		t.Fatal("unknown declarations claimed full change coverage")
	}
}

func TestApplicationSourceCredentialAndRemoteTransportRemainExplicit(t *testing.T) {
	for _, origin := range []string{"https://provider.example", "https://provider.example:9443/", "http://127.0.0.1:1234", "http://[::1]:1234/"} {
		if !repositoryProviderOrigin(origin) {
			t.Fatal("valid fixed origin rejected", origin)
		}
	}
	for _, origin := range []string{"http://remote.example", "http://localhost:1234", "https://a.example/path", "https://u:secret@a.example", "https://a.example?", "https://a.example#", " https://a.example"} {
		if repositoryProviderOrigin(origin) {
			t.Fatal("unsafe fixed origin allowed", origin)
		}
	}
	config := map[string]any{"id": "p", "tenant_id": "t", "project_id": "j", "target_aliases": []string{"main"}, "origin": "https://provider.example", "token_env": "C", "reconcile_token_env": "R", "source_read_token_env": "S"}
	env := map[string]string{"C": strings.Repeat("c", 32), "R": strings.Repeat("r", 32), "S": strings.Repeat("s", 32)}
	load := func() ([]RepositoryProviderConfig, error) {
		raw, _ := json.Marshal([]any{config})
		return LoadRepositoryProviders(string(raw), func(key string) string { return env[key] })
	}
	loaded, err := load()
	if err != nil || loaded[0].SourceReadToken != env["S"] {
		t.Fatal("independent source authority lost", err)
	}
	env["S"] = env["R"]
	if _, err = load(); err == nil {
		t.Fatal("source credential reused reconcile authority")
	}
	delete(config, "source_read_token_env")
	loaded, err = load()
	if err != nil || loaded[0].SourceReadToken != "" {
		t.Fatal("optional source silently borrowed existing authority", err)
	}
}

// Preserve original number tokens at the shared HTTP decoder. Exponent and
// decimal notation must not become canonical integer versions after remarshal.
func TestDispatchRecommendationHTTPPreservesVersionNumberLexemes(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f := h.f
	options := h.request.DispatchRecommendationOptionsRequest
	raw, _ := json.Marshal(options)
	canonical := "\"expected_task_version\":" + stringify(options.ExpectedTaskVersion)
	if !strings.Contains(string(raw), canonical) {
		t.Fatal("fixture lacks original version")
	}
	for _, suffix := range []string{".0", "e0"} {
		body := strings.Replace(string(raw), canonical, canonical+suffix, 1)
		req := httptest.NewRequest("POST", "http://suma.test/api/v1/dispatch-recommendations/options", strings.NewReader(body))
		req.AddCookie(f.cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Expected-User", f.view.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatalf("noncanonical version %s accepted: %d %s", suffix, response.Code, response.Body.String())
		}
	}
}
