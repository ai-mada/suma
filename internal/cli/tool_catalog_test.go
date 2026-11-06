package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolCatalogCLIFiltersExactProjectCatalogLocally(t *testing.T) {
	const asOf = "2026-11-05T08:30:00.123Z"
	items := tc.AdapterCatalog()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/adapter-catalog" || len(query) != 1 || !reflect.DeepEqual(query["project_id"], []string{"project-one"}) || len(body) != 0 {
			t.Error("catalog must send only the exact project's GET", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer catalog-test-session" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) || r.Header.Get("Accept-Language") != "en-US" {
			t.Error("catalog did not retain the current caller view")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"project_id": "project-one", "as_of": asOf, "items": items})
	}))
	defer server.Close()

	for _, test := range []struct {
		name string
		args []string
		ids  []string
	}{
		{name: "default all"},
		{name: "case insensitive terms across fields", args: []string{"--query", "CoDeX\t\u2003APP_SERVER"}, ids: []string{"codex-app-server"}},
		{name: "client direction and exact name", args: []string{"--direction", "tool_to_suma", "--query", "WORKBUDDY"}, ids: []string{"workbuddy-connector"}},
		{name: "execution order", args: []string{"--direction", "suma_to_tool", "--query", "harness"}, ids: []string{"octop-acp", "dsh-multica", "harness-guide"}},
		{name: "empty match set", args: []string{"--query", "impossible adapter string"}, ids: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"--control", server.URL, "--project", "project-one", "--token", "catalog-test-session", "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "--language", "en-US", "tools", "catalog"}
			var out, stderr bytes.Buffer
			code := Execute(context.Background(), append(args, test.args...), nil, &out, &stderr)
			if code != 0 {
				t.Fatalf("catalog failed, exit %d, %s", code, stderr.String())
			}
			want := items
			if test.ids != nil {
				want = []tc.AdapterDescriptor{}
				for _, id := range test.ids {
					for _, item := range items {
						if item.ID == id {
							want = append(want, item)
						}
					}
				}
			}
			var response struct {
				ProjectID string                 `json:"project_id"`
				AsOf      string                 `json:"as_of"`
				Items     []tc.AdapterDescriptor `json:"items"`
			}
			if json.Unmarshal(out.Bytes(), &response) != nil || response.ProjectID != "project-one" || response.AsOf != asOf || !reflect.DeepEqual(response.Items, want) {
				t.Fatalf("catalog altered its scope, provenance, order or descriptor facts: %s", out.String())
			}
		})
	}
}

func TestToolCatalogCLIHelpAndProjectRequirement(t *testing.T) {
	for _, test := range []struct {
		locale, help, usage string
	}{
		{"en-US", "find adapter entry points", "requires --project"},
		{"zh-CN", "查找当前项目的工具接入入口", "需要 --project"},
	} {
		base := []string{"--language", test.locale, "tools", "catalog"}
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), append(base, "--help"), nil, &out, &stderr); code != 0 || !strings.Contains(out.String(), test.help) || !strings.Contains(out.String(), "all|suma_to_tool|tool_to_suma") || stderr.Len() != 0 {
			t.Fatalf("catalog help unavailable in %s, exit %d, stdout %s, stderr %s", test.locale, code, out.String(), stderr.String())
		}
		out.Reset()
		if code := Execute(context.Background(), base, nil, &out, &stderr); code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), test.usage) {
			t.Fatalf("missing project was not rejected in %s, exit %d, stderr %s", test.locale, code, stderr.String())
		}
	}
}

func TestToolCatalogCLIRejectsUntrustedResponsesAndInvalidOptions(t *testing.T) {
	const validEnvelope = `{"project_id":"project-one","as_of":"2026-11-05T08:30:00Z","items":%s}`
	item, err := json.Marshal(tc.AdapterCatalog()[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, body string
		options    []string
		status     int
		exit       int
	}{
		{name: "wrong scope", body: `{"project_id":"another-project","as_of":"2026-11-05T08:30:00Z","items":[]}`, exit: 1},
		{name: "unknown field", body: `{"project_id":"project-one","as_of":"2026-11-05T08:30:00Z","items":[],"unexpected":"untrusted-data"}`, exit: 1},
		{name: "duplicate descriptors", body: fmt.Sprintf(validEnvelope, "["+string(item)+","+string(item)+"]"), exit: 1},
		{name: "wrong direction setup", body: fmt.Sprintf(validEnvelope, "["+strings.Replace(string(item), `"runner_profile"`, `"client_export"`, 1)+"]"), exit: 1},
		{name: "current server refusal", body: `{"error":{"code":"forbidden","message":"current caller refused"}}`, status: http.StatusForbidden, exit: 1},
		{name: "unsupported direction", options: []string{"--direction", "both"}, exit: 2},
		{name: "query control characters", options: []string{"--query", "private\x00text"}, exit: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.exit == 2 {
					t.Error("invalid arguments issued a request")
				}
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			args := []string{"--control", server.URL, "--project", "project-one", "--language", "en-US", "tools", "catalog"}
			var out, stderr bytes.Buffer
			if code := Execute(context.Background(), append(args, test.options...), nil, &out, &stderr); code != test.exit || out.Len() != 0 || strings.Contains(stderr.String(), "untrusted-data") {
				t.Fatalf("invalid input or response published output, exit %d, stdout %s, stderr %s", code, out.String(), stderr.String())
			}
			if test.status == http.StatusForbidden && !strings.Contains(stderr.String(), "forbidden") {
				t.Fatal("server refusal was masked", stderr.String())
			}
		})
	}
}
