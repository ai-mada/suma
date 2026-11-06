package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const recommendationOptionsTestUser = "1234567890abcdef1234567890abcdef"
const recommendationOptionsTestOrganization = "fedcba0987654321fedcba0987654321"

func recommendationOptionsTestArgs() []string {
	return []string{"task-one", "--expected-version", "2", "--workflow", "flow-one", "--workflow-version", "3", "--node", "develop"}
}

func recommendationOptionsTestClient(t *testing.T, server *httptest.Server) *client {
	t.Helper()
	c, err := newClient(server.URL, "project-one", "browser-session", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.setOriginalView(recommendationOptionsTestUser, recommendationOptionsTestOrganization); err != nil {
		t.Fatal(err)
	}
	return c
}

func recommendationOptionsTestSource() map[string]any {
	return map[string]any{
		"sha256": strings.Repeat("a", 64), "policy_sha256": strings.Repeat("b", 64),
		"context": map[string]any{"task": map[string]any{"id": "task-one", "version": 2}},
		"workflow": map[string]any{
			"id": "flow-one", "version": 3,
			"nodes": []any{map[string]any{"id": "develop", "type": "agent", "depends_on": nil}},
			"edges": nil,
		},
		"agent_node_id":   "develop",
		"execution_route": map[string]any{"project_id": "project-one", "tenant_id": recommendationOptionsTestOrganization},
		"constraints":     map[string]any{"require_hard_budget": false, "max_steps": 8, "max_output_tokens": 4096},
		"candidates":      []any{}, "coverage": map[string]any{"candidate_count": 0},
	}
}

func TestDispatchRecommendationOptionsCLISendsExactScopedRead(t *testing.T) {
	for _, budgeted := range []bool{false, true} {
		t.Run(fmt.Sprintf("hard_budget_%t", budgeted), func(t *testing.T) {
			calls := 0
			args := recommendationOptionsTestArgs()
			source := recommendationOptionsTestSource()
			want := map[string]any{
				"project_id": "project-one", "task_id": "task-one", "expected_task_version": float64(2),
				"workflow_id": "flow-one", "workflow_version": float64(3), "agent_node_id": "develop",
				"knowledge":   []any{},
				"constraints": map[string]any{"require_hard_budget": false, "max_steps": float64(8), "max_output_tokens": float64(4096)},
			}
			if budgeted {
				args = append(args, "--max-steps", "4", "--max-output-tokens", "2048", "--token-limit", "12000", "--require-hard-budget")
				source["constraints"] = map[string]any{"require_hard_budget": true, "max_steps": 4, "max_output_tokens": 2048, "budget": map[string]any{"token_limit": 12000}}
				want["constraints"] = map[string]any{"require_hard_budget": true, "max_steps": float64(4), "max_output_tokens": float64(2048), "budget": map[string]any{"token_limit": float64(12000)}}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/dispatch-recommendations/options" || len(r.URL.Query()) != 1 || r.URL.Query().Get("project_id") != "project-one" ||
					r.Header.Get("Authorization") != "Bearer browser-session" || r.Header.Get("X-Suma-Expected-User") != recommendationOptionsTestUser || r.Header.Get("X-Suma-Expected-Organization") != recommendationOptionsTestOrganization {
					t.Error("read lost the exact endpoint, project or browser view")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body, want) {
					t.Errorf("read body = %#v, want %#v (error %v)", body, want, err)
				}
				_ = json.NewEncoder(w).Encode(source)
			}))
			defer server.Close()
			var out bytes.Buffer
			if err := dispatchRecommendationOptionsCommand(context.Background(), recommendationOptionsTestClient(t, server), args, &out); err != nil || calls != 1 {
				t.Fatalf("recommendation read failed: %v (%d calls)", err, calls)
			}
			if !strings.Contains(out.String(), `"depends_on": null`) || !strings.Contains(out.String(), `"edges": null`) {
				t.Fatal("valid nullable workflow fields were not preserved")
			}
		})
	}
}

func TestDispatchRecommendationOptionsCLIRejectsMismatchedSource(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"project":      func(v map[string]any) { v["execution_route"].(map[string]any)["project_id"] = "another-project" },
		"organization": func(v map[string]any) { v["execution_route"].(map[string]any)["tenant_id"] = strings.Repeat("b", 32) },
		"task":         func(v map[string]any) { v["context"].(map[string]any)["task"].(map[string]any)["id"] = "another-task" },
		"aliased task": func(v map[string]any) {
			v["context"].(map[string]any)["task"] = json.RawMessage(`{"id":"another-task","ID":"task-one","version":2}`)
		},
		"workflow version":           func(v map[string]any) { v["workflow"].(map[string]any)["version"] = 8 },
		"node":                       func(v map[string]any) { v["agent_node_id"] = "another-node" },
		"missing selected node":      func(v map[string]any) { v["workflow"].(map[string]any)["nodes"] = []any{} },
		"constraints":                func(v map[string]any) { v["constraints"].(map[string]any)["max_steps"] = 7 },
		"absent hard budget boolean": func(v map[string]any) { delete(v["constraints"].(map[string]any), "require_hard_budget") },
		"invalid digest":             func(v map[string]any) { v["sha256"] = "not-a-digest" },
		"excess candidates": func(v map[string]any) {
			candidates := make([]any, 25)
			for i := range candidates {
				candidates[i] = map[string]any{"id": fmt.Sprintf("%064x", i)}
			}
			v["candidates"] = candidates
			v["coverage"] = map[string]any{"candidate_count": len(candidates)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := recommendationOptionsTestSource()
			change(source)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(source)
			}))
			defer server.Close()
			var out bytes.Buffer
			err := dispatchRecommendationOptionsCommand(context.Background(), recommendationOptionsTestClient(t, server), recommendationOptionsTestArgs(), &out)
			if failure, ok := err.(*failure); !ok || failure.Code != "response" || out.Len() != 0 {
				t.Fatalf("unsafe response reached output: error=%v bytes=%d", err, out.Len())
			}
		})
	}
}

func TestDispatchRecommendationOptionsCLIRejectsInvalidFlagsBeforeHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("invalid arguments reached control plane")
	}))
	defer server.Close()
	for name, extra := range map[string][]string{
		"noncanonical version": {"--expected-version", "01"},
		"max steps overflow":   {"--max-steps", "129"},
		"zero output":          {"--max-output-tokens", "0"},
		"budget missing":       {"--require-hard-budget"},
		"token limit overflow": {"--token-limit", "9007199254740992"},
		"write control":        {"--confirm"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			err := dispatchRecommendationOptionsCommand(context.Background(), recommendationOptionsTestClient(t, server), append(recommendationOptionsTestArgs(), extra...), &out)
			if failure, ok := err.(*failure); !ok || failure.Code != "usage" || out.Len() != 0 {
				t.Fatalf("invalid flags accepted: %v", err)
			}
		})
	}
}

func TestDispatchRecommendationOptionsCLIReadClassification(t *testing.T) {
	const base = "/api/v1/dispatch-recommendations"
	if !requestIsReadOnly(http.MethodPost, base+"/options?project_id=project-one") {
		t.Fatal("options query has mutation transport semantics")
	}
	for _, path := range []string{base, base + "/recommendation-one/adopt", base + "/options/extra"} {
		if requestIsReadOnly(http.MethodPost, path) {
			t.Errorf("write route %q classified as read-only", path)
		}
	}
}
