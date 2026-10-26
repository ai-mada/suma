package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExpertPlanCLI(t *testing.T) {
	t.Run("help_and_confirmation", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, language := range []string{"en-US", "zh-CN"} {
			code, out, err := contextInvoke(t, s.URL, language, "", "team-plan", "--help")
			if code != 0 || err != "" || !strings.Contains(out, "--confirm") || !strings.Contains(out, "revisions") || language == "zh-CN" && !strings.Contains(out, "专家组交付计划") {
				t.Fatal(code, out, err)
			}
			for _, action := range []string{"create", "start", "command"} {
				args := []string{"team-plan", action}
				if action != "create" {
					args = append(args, "plan-one")
				}
				args = append(args, "--file", "-")
				code, _, err = contextInvoke(t, s.URL, language, "{}", args...)
				if code != 2 || !strings.Contains(err, "cli.team_plan.write_options") {
					t.Fatal(action, code, err)
				}
			}
		}
		if calls != 0 {
			t.Fatal("help/unconfirmed write made a request")
		}
	})
	t.Run("fixed_read_routes", func(t *testing.T) {
		for _, test := range []struct {
			args         []string
			path, before string
			limit        string
		}{{[]string{"list", "--limit", "5", "--before", "original-plan"}, "/api/v1/expert-team-plans", "original-plan", "5"}, {[]string{"get", "plan-one"}, "/api/v1/expert-team-plans/plan-one", "", ""}, {[]string{"revisions", "plan-one", "--limit", "100", "--before", "revision-cursor"}, "/api/v1/expert-team-plans/plan-one/revisions", "revision-cursor", "100"}, {[]string{"revisions", "plan-one", "2"}, "/api/v1/expert-team-plans/plan-one/revisions/2", "", ""}} {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != test.path || r.URL.Query().Get("project_id") != "project-one" || r.URL.Query().Get("before") != test.before || r.URL.Query().Get("limit") != test.limit || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Suma-Expected-User") != "" {
					t.Error("wrong fixed read route", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(`{"version":9007199254740993,"title":"用户原文不改"}`))
			}))
			code, out, err := contextInvoke(t, s.URL, "en-US", "", append([]string{"team-plan"}, test.args...)...)
			s.Close()
			if code != 0 || err != "" || calls != 1 || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "用户原文不改") {
				t.Fatal(code, out, err)
			}
		}
	})
	t.Run("typed_write_routes_and_original_basis", func(t *testing.T) {
		for _, test := range []struct {
			args       []string
			body, path string
		}{{[]string{"create"}, `{"team_id":"team-one","expected_configuration_version":2,"items":[{"task_id":"task-one","expected_task_version":1,"role_key":"developer"}],"idempotency_key":"original-create-key"}`, "/api/v1/expert-team-plans"}, {[]string{"start", "plan-one"}, `{"expected_version":2147483646,"acknowledge_dispatch":true,"idempotency_key":"original-start-key"}`, "/api/v1/expert-team-plans/plan-one/start"}, {[]string{"command", "plan-one"}, `{"expected_version":3,"action":"pause","reason":"明确暂停 原文","acknowledge":true,"idempotency_key":"original-pause-key"}`, "/api/v1/expert-team-plans/plan-one/commands"}, {[]string{"command", "plan-one"}, `{"expected_version":4,"action":"revise","reason":"明确修订 原文","acknowledge":true,"expected_configuration_version":2,"items":[{"task_id":"task-two","expected_task_version":1,"role_key":"developer"}],"idempotency_key":"original-revise-key"}`, "/api/v1/expert-team-plans/plan-one/commands"}} {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				var body, want map[string]any
				d := json.NewDecoder(strings.NewReader(string(raw)))
				d.UseNumber()
				_ = d.Decode(&body)
				d = json.NewDecoder(strings.NewReader(test.body))
				d.UseNumber()
				_ = d.Decode(&want)
				want["project_id"] = "project-one"
				actualJSON, _ := json.Marshal(body)
				expectedJSON, _ := json.Marshal(want)
				if r.Method != "POST" || r.URL.Path != test.path || string(actualJSON) != string(expectedJSON) || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("changed fixed write/current view/operation", r.Method, r.URL, string(raw))
				}
				_, _ = w.Write([]byte(`{"state":"active"}`))
			}))
			args := append([]string{"team-plan"}, test.args...)
			args = append(args, "--file", "-", "--confirm")
			code, _, err := contextInvoke(t, s.URL, "zh-CN", test.body, args...)
			s.Close()
			if code != 0 || err != "" || calls != 1 {
				t.Fatal(code, err, calls)
			}
		}
	})
	t.Run("invalid_input_never_sent", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, body := range []string{`{"expected_version":1.0,"acknowledge_dispatch":true,"idempotency_key":"original-key"}`, `{"expected_version":1e0,"acknowledge_dispatch":true,"idempotency_key":"original-key"}`, `{"expected_version":1,"expected_version":2,"acknowledge_dispatch":true,"idempotency_key":"original-key"}`, `{"expected_version":1,"acknowledge_dispatch":false,"idempotency_key":"original-key"}`, `{"expected_version":1,"acknowledge_dispatch":true}`, `{"expected_version":1,"acknowledge_dispatch":true,"idempotency_key":"original-key","credential_hash":"no"}`, `{"project_id":"other-project","expected_version":1,"acknowledge_dispatch":true,"idempotency_key":"original-key"}`, `{"expected_version":1,"acknowledge_dispatch":true,"idempotency_key":"bad-\ud800-key"}`} {
			code, _, err := contextInvoke(t, s.URL, "en-US", body, "team-plan", "start", "plan-one", "--file", "-", "--confirm")
			if code != 2 || err == "" {
				t.Fatal("invalid body accepted", body, code, err)
			}
		}
		code, _, _ := contextInvoke(t, s.URL, "en-US", `{"expected_version":1,"action":"accept","reason":"no implicit acceptance","acknowledge":true,"idempotency_key":"original-key"}`, "team-plan", "command", "plan-one", "--file", "-", "--confirm")
		if code != 2 || calls != 0 {
			t.Fatal("invalid input made a mutation", code, calls)
		}
	})
}
