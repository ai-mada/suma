package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const recommendationHistoryCard = `{"id":"rec-one","project_id":"project-one","version":1,"source":{"context":{"task":{"id":"task-one","version":2}},"workflow":{"nodes":[{"depends_on":null}]}},"basis_current":false,"allowed_actions":[],"original_output":"saved result"}`

func TestDispatchRecommendationHistoryCLIScopedReads(t *testing.T) {
	const before = "cursor+old&value"
	const token = "fixture-api-credential"
	for _, test := range []struct {
		name, action, path, response string
		args                         []string
	}{
		{"page", "recommendations", "/api/v1/dispatch-recommendations", `{"items":[` + recommendationHistoryCard + `],"total":3,"next_cursor":"rec-one"}`, []string{"task-one", "--limit", "1", "--before", before}},
		{"detail", "recommendation", "/api/v1/dispatch-recommendations/rec-one", recommendationHistoryCard, []string{"rec-one"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != test.path || query.Get("project_id") != "project-one" || r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("unexpected scoped read", r.Method, r.URL)
				}
				if test.name == "page" {
					if len(query) != 4 || query.Get("task_id") != "task-one" || query.Get("limit") != "1" || query.Get("before") != before {
						t.Error("pagination changed", query)
					}
				} else if len(query) != 1 {
					t.Error("detail got extra query fields", query)
				}
				_, _ = io.WriteString(w, test.response)
			}))
			defer server.Close()
			c, err := newClient(server.URL, "project-one", token, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			if err := dispatchRecommendationHistoryCommand(context.Background(), c, test.action, test.args, &out); err != nil {
				t.Fatal(err)
			}
			var output, expected any
			if json.Unmarshal([]byte(out.String()), &output) != nil || json.Unmarshal([]byte(test.response), &expected) != nil {
				t.Fatal("invalid JSON output", out.String())
			}
			gotJSON, _ := json.Marshal(output)
			wantJSON, _ := json.Marshal(expected)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("response facts changed\n%s\n%s", gotJSON, wantJSON)
			}
		})
	}
}

func TestDispatchRecommendationHistoryCLIRejectsUnboundResponses(t *testing.T) {
	for _, test := range []struct {
		name, action, response string
	}{
		{"wrong detail ID", "recommendation", strings.Replace(recommendationHistoryCard, "rec-one", "rec-two", 1)},
		{"wrong project", "recommendation", strings.Replace(recommendationHistoryCard, "project-one", "project-two", 1)},
		{"missing source task", "recommendation", `{"id":"rec-one","project_id":"project-one","version":1,"source":{}}`},
		{"duplicate scope field", "recommendation", strings.Replace(recommendationHistoryCard, `"project_id":"project-one"`, `"project_id":"other","project_id":"project-one"`, 1)},
		{"case-aliased task scope", "recommendations", `{"items":[` + strings.Replace(recommendationHistoryCard, `"id":"task-one"`, `"id":"task-one","ID":"other"`, 1) + `],"total":1,"next_cursor":""}`},
		{"wrong task", "recommendations", `{"items":[` + strings.Replace(recommendationHistoryCard, "task-one", "task-two", 1) + `],"total":1,"next_cursor":""}`},
		{"duplicate record", "recommendations", `{"items":[` + recommendationHistoryCard + `,` + recommendationHistoryCard + `],"total":2,"next_cursor":""}`},
		{"invalid page total", "recommendations", `{"items":[` + recommendationHistoryCard + `],"total":0,"next_cursor":""}`},
		{"invalid cursor basis", "recommendations", `{"items":[],"total":0,"next_cursor":"rec-one"}`},
		{"null page", "recommendations", `{"items":null,"total":0,"next_cursor":""}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, test.response)
			}))
			defer server.Close()
			c, err := newClient(server.URL, "project-one", "", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			id := "rec-one"
			if test.action == "recommendations" {
				id = "task-one"
			}
			var out strings.Builder
			err = dispatchRecommendationHistoryCommand(context.Background(), c, test.action, []string{id}, &out)
			if err == nil || out.Len() != 0 {
				t.Fatalf("unbound response reached output: %v %q", err, out.String())
			}
		})
	}
}

func TestDispatchRecommendationHistoryCLIRejectsInvalidUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached control plane", r.URL)
	}))
	defer server.Close()
	for _, test := range []struct {
		project, action string
		args            []string
	}{
		{"project-one", "recommendations", nil},
		{"project-one", "recommendations", []string{"task-one", "--limit", "101"}},
		{"project-one", "recommendations", []string{"task-one", "--limit", "01"}},
		{"project-one", "recommendations", []string{"task-one", "--before", "bad\nvalue"}},
		{"project-one", "recommendation", []string{"../another"}},
		{"project-one", "recommendation", []string{"rec-one", "--confirm"}},
		{"", "recommendation", []string{"rec-one"}},
	} {
		c, err := newClient(server.URL, test.project, "", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := dispatchRecommendationHistoryCommand(context.Background(), c, test.action, test.args, io.Discard); err == nil {
			t.Fatalf("invalid history invocation accepted: %s %v", test.action, test.args)
		}
	}
}
