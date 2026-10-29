package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolDiscoveryHTTPCurrentReportAndNoReport(t *testing.T) {
	f, token, profile := toolFixture(t)
	path := "/tool-discovery?project_id=" + url.QueryEscape(f.project) + "&worker_id=" + profile.WorkerID
	first := f.call(http.MethodGet, path, nil, 200)
	if first["state"] != "unreported" || len(first["items"].([]any)) != 0 {
		t.Fatal("no report became a successful probe", first)
	}
	if _, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	before := discoveryBusinessCounts(t, f)
	fresh := f.call(http.MethodGet, path, nil, 200)
	if fresh["project_id"] != f.project || fresh["worker_id"] != profile.WorkerID || fresh["state"] != "fresh" || len(fresh["items"].([]any)) != 1 {
		t.Fatal("current authorized report not exposed", fresh)
	}
	if !reflect.DeepEqual(before, discoveryBusinessCounts(t, f)) {
		t.Fatal("read caused business effects")
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE project_id=?`, f.project); err != nil {
		t.Fatal(err)
	}
	stale := f.call(http.MethodGet, path, nil, 200)
	if stale["state"] != "stale" || len(stale["items"].([]any)) != 0 {
		t.Fatal("stale observations disclosed", stale)
	}
}

func TestToolDiscoveryHTTPRejectsAmbiguousScopeAndWrites(t *testing.T) {
	f, _, profile := toolFixture(t)
	base := "/tool-discovery?project_id=" + url.QueryEscape(f.project) + "&worker_id=" + profile.WorkerID
	for _, suffix := range []string{"&worker_id=other", "&project_id=other", "&extra=value", "&worker_id=", "&bad=%ZZ"} {
		f.call(http.MethodGet, base+suffix, nil, 400)
	}
	f.call(http.MethodGet, "/tool-discovery?project_id="+f.project, nil, 400)
	f.call(http.MethodGet, "/tool-discovery?project_id="+f.project+"&worker_id=missing-worker", nil, 403)
	f.call(http.MethodPost, base, f.body(map[string]any{}), 404)
	f.call(http.MethodGet, "/tool-discovery/extra?project_id="+f.project+"&worker_id="+profile.WorkerID, nil, 404)
}

func TestToolDiscoveryHTTPRejectsRunnerCredentialAndMissingBrowser(t *testing.T) {
	f, token, profile := toolFixture(t)
	target := "http://suma.test/api/v1/tool-discovery?project_id=" + f.project + "&worker_id=" + profile.WorkerID
	for _, credential := range []string{"", token} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		want := http.StatusUnauthorized
		if credential != "" {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("credential scope got %d want %d", response.Code, want)
		}
	}
}
