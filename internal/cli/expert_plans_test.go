package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
				if r.Method != "GET" || r.URL.Path != test.path || r.URL.Query().Get("project_id") != "project-one" || r.URL.Query().Get("before") != test.before || r.URL.Query().Get("limit") != test.limit || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
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

func TestExpertPlanCLIExactTeamList(t *testing.T) {
	cursor := strings.Repeat("aB1_-", 40)
	const correct = `{"id":"plan-one","project_id":"project-one","team_id":"Team_one","detail":{"number":9007199254740993,"title":"用户原文"}}`
	for _, test := range []struct {
		name, body, before string
		ok                 bool
	}{
		{"first page", `{"items":[` + correct + `],"total":2,"next_cursor":"` + cursor + `"}`, "", true},
		{"opaque continuation", `{"items":[` + correct + `],"total":2}`, cursor, true},
		{"empty result", `{"items":[],"total":0}`, "", true},
		{"different team", `{"items":[` + strings.Replace(correct, `"Team_one"`, `"team_one"`, 1) + `],"total":1}`, "", false},
		{"different project", `{"items":[` + strings.Replace(correct, `"project-one"`, `"project-other"`, 1) + `],"total":1}`, "", false},
		{"ambiguous team", `{"items":[` + strings.Replace(correct, `"team_id":"Team_one"`, `"team_id":"Team_one","TEAM_ID":"team_other"`, 1) + `],"total":1}`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/expert-team-plans" || q.Get("team_id") != "Team_one" || q.Get("project_id") != "project-one" || q.Get("limit") != "1" || q.Get("before") != test.before || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("filtered request changed scope or cursor", r.Method, r.URL)
				}
				_, _ = io.WriteString(w, test.body)
			}))
			defer s.Close()
			args := []string{"team-plan", "list", "--team", "Team_one", "--limit", "1"}
			if test.before != "" {
				args = append(args, "--before", test.before)
			}
			code, out, problem := contextInvoke(t, s.URL, "en-US", "", args...)
			if !test.ok {
				if code == 0 || out != "" || !strings.Contains(problem, "cli.team_plan.scope_response_invalid") {
					t.Fatal("mismatched response was exposed", code, out, problem)
				}
				return
			}
			if code != 0 || problem != "" || !json.Valid([]byte(out)) {
				t.Fatal("valid exact page was rejected", code, out, problem)
			}
			if strings.Contains(test.body, "9007199254740993") && (!strings.Contains(out, "9007199254740993") || !strings.Contains(out, "用户原文")) || strings.Contains(test.body, cursor) && !strings.Contains(out, cursor) {
				t.Fatal("filtered read rewrote the source fields", out)
			}
		})
	}
	t.Run("invalid flags and bilingual help are local", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("unexpected request for invalid option or help", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer s.Close()
		for _, args := range [][]string{{"list", "--team", ""}, {"list", "--team", "../other"}, {"list", "--team", "team.other"}, {"list", "--team", "team other"}, {"revisions", "plan-one", "--team", "Team_one"}} {
			if code, out, _ := contextInvoke(t, s.URL, "en-US", "", append([]string{"team-plan"}, args...)...); code != 2 || out != "" {
				t.Fatal("invalid flags were accepted", args, code, out)
			}
		}
		for _, language := range []string{"en-US", "zh-CN"} {
			if code, out, problem := contextInvoke(t, s.URL, language, "", "team-plan", "--help"); code != 0 || problem != "" || !strings.Contains(out, "team-plan list [--team TEAM_ID]") {
				t.Fatal("help omitted team filter", language, code, out, problem)
			}
		}
	})
}

func TestExpertPlanCLIExactRunList(t *testing.T) {
	const run = "Run-One"
	const item = `{"id":"plan-one","project_id":"project-one","team_id":"Team_one","items":[{"task_id":"waiting"},{"task_id":"started","run_id":"Run-One"}],"detail":{"number":9007199254740993}}`
	cursor := strings.Repeat("aB1_-", 40)
	for _, test := range []struct {
		name, body, team, before string
		ok                       bool
	}{
		{"exact run", `{"items":[` + item + `],"total":1}`, "", "", true},
		{"team intersection", `{"items":[` + item + `],"total":1,"next_cursor":""}`, "Team_one", "", true},
		{"no binding", `{"items":[],"total":0}`, "", "", true},
		{"unexpected continuation", `{"items":[` + item + `],"total":1,"next_cursor":"` + cursor + `"}`, "", "", false},
		{"different team", `{"items":[` + item + `],"total":1}`, "Team_other", "", false},
		{"different run", `{"items":[` + strings.Replace(item, "Run-One", "run-one", 1) + `],"total":1}`, "", "", false},
		{"wrong project", `{"items":[` + strings.Replace(item, "project-one", "project-two", 1) + `],"total":1}`, "", "", false},
		{"ambiguous run", `{"items":[` + strings.Replace(item, `"run_id":"Run-One"`, `"run_id":"Run-One","RUN_ID":"Run-Other"`, 1) + `],"total":1}`, "", "", false},
		{"ambiguous plan items", `{"items":[` + strings.Replace(item, `"items":`, `"ITEMS":[],"items":`, 1) + `],"total":1}`, "", "", false},
		{"multiple bindings in plan", `{"items":[` + strings.Replace(item, `{"task_id":"waiting"}`, `{"task_id":"waiting","run_id":"Run-One"}`, 1) + `],"total":1}`, "", "", false},
		{"unexplained missing match", `{"items":[],"total":1}`, "", "", false},
		{"impossible additional match", `{"items":[` + item + `],"total":2}`, "", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/expert-team-plans" || q.Get("run_id") != run || len(q["run_id"]) != 1 || q.Get("team_id") != test.team || q.Get("project_id") != "project-one" || q.Get("limit") != "1" || q.Get("before") != test.before || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("run-filtered request changed scope or cursor", r.Method, r.URL)
				}
				_, _ = io.WriteString(w, test.body)
			}))
			defer s.Close()
			args := []string{"team-plan", "list", "--run", run, "--limit", "1"}
			if test.team != "" {
				args = append(args, "--team", test.team)
			}
			if test.before != "" {
				args = append(args, "--before", test.before)
			}
			code, out, problem := contextInvoke(t, s.URL, "en-US", "", args...)
			if calls != 1 {
				t.Fatal("expected one lookup", calls, code, problem)
			}
			if !test.ok {
				if code == 0 || out != "" || !strings.Contains(problem, "cli.team_plan.scope_response_invalid") {
					t.Fatal("mismatched response was exposed", code, out, problem)
				}
				return
			}
			if code != 0 || problem != "" || !json.Valid([]byte(out)) || strings.Contains(test.body, "9007199254740993") && !strings.Contains(out, "9007199254740993") {
				t.Fatal("valid run page was rejected or rewritten", code, out, problem)
			}
		})
	}
	t.Run("invalid flags and bilingual help are local", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("unexpected request for invalid run option or help", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer s.Close()
		for _, args := range [][]string{{"list", "--run", ""}, {"list", "--run", "Run.One"}, {"list", "--run", strings.Repeat("a", 65)}, {"list", "--run", run, "--run", "other"}, {"list", "--run", run, "--before", cursor}, {"revisions", "plan-one", "--run", run}} {
			if code, out, _ := contextInvoke(t, s.URL, "en-US", "", append([]string{"team-plan"}, args...)...); code != 2 || out != "" {
				t.Fatal("invalid run flags were accepted", args, code, out)
			}
		}
		for _, language := range []string{"en-US", "zh-CN"} {
			if code, out, problem := contextInvoke(t, s.URL, language, "", "team-plan", "--help"); code != 0 || problem != "" || !strings.Contains(out, "[--run RUN_ID]") {
				t.Fatal("help omitted run filter", language, code, out, problem)
			}
		}
	})
}

func TestExpertPlanCLICodexSelectionExactRecoveryAndStrictFields(t *testing.T) {
	selection := `{"catalog_sha256":"` + strings.Repeat("a", 64) + `","model":"codex-a","reasoning_effort":"high","token_notice_limit":9007199254740991}`
	item := func(choice string) string {
		return `{"task_id":"task-one","expected_task_version":1,"role_key":"developer","codex_execution_selection":` + choice + `}`
	}
	for _, action := range []string{"create", "revise"} {
		t.Run(action, func(t *testing.T) {
			body := `{"team_id":"team-one","expected_configuration_version":2,"items":[` + item(selection) + `],"idempotency_key":"codex-original-create"}`
			args := []string{"team-plan", "create", "--file", "-", "--confirm"}
			if action == "revise" {
				body = `{"expected_version":3,"action":"revise","reason":"Explicit original Codex source","acknowledge":true,"expected_configuration_version":2,"items":[` + item(selection) + `],"idempotency_key":"codex-original-revise"}`
				args = []string{"team-plan", "command", "plan-one", "--file", "-", "--confirm"}
			}
			requests := []map[string]any{}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				var request map[string]any
				if err := decoder.Decode(&request); err != nil {
					t.Error(err)
				}
				requests = append(requests, request)
				if len(requests) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":{"code":"uncertain","message":"Keep the original operation"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"original_receipt":true}`))
			}))
			defer s.Close()
			code, _, problem := contextInvoke(t, s.URL, "en-US", body, args...)
			if code == 0 || problem == "" || len(requests) != 1 {
				t.Fatal("uncertain mutation was lost or retried automatically", code, problem, len(requests))
			}
			code, out, problem := contextInvoke(t, s.URL, "en-US", body, args...)
			if code != 0 || problem != "" || !strings.Contains(out, `"original_receipt": true`) || len(requests) != 2 || !reflect.DeepEqual(requests[0], requests[1]) {
				t.Fatal("explicit recovery changed original Codex operation", code, out, problem, requests)
			}
			var want map[string]any
			decoder := json.NewDecoder(strings.NewReader(body))
			decoder.UseNumber()
			if err := decoder.Decode(&want); err != nil {
				t.Fatal(err)
			}
			want["project_id"] = "project-one"
			if !reflect.DeepEqual(requests[0], want) {
				t.Fatal("transport changed public selection or integer lexemes", requests[0])
			}
		})
	}
	t.Run("private_and_ambiguous_values_never_sent", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, invalid := range []string{
			`null`,
			strings.Replace(selection, `"model":`, `"api_key":"private","model":`, 1),
			strings.Replace(selection, `"model":`, `"configuration_sha256":"`+strings.Repeat("b", 64)+`","model":`, 1),
			strings.Replace(selection, `"model":`, `"provider":"private","model":`, 1),
			strings.Replace(selection, `"model":`, `"budget_mode":"enforce","model":`, 1),
			strings.Replace(selection, `"model":`, `"model":"duplicate","model":`, 1),
			strings.Replace(selection, `9007199254740991`, `1e2`, 1),
			strings.Replace(selection, `9007199254740991`, `1.0`, 1),
			strings.Replace(selection, `9007199254740991`, `9007199254740992`, 1),
			strings.Replace(selection, `9007199254740991`, `0`, 1),
			strings.Replace(selection, `"codex-a"`, `{"api_key":"private"}`, 1),
		} {
			body := `{"team_id":"team-one","expected_configuration_version":2,"items":[` + item(invalid) + `],"idempotency_key":"codex-original-create"}`
			code, _, problem := contextInvoke(t, s.URL, "en-US", body, "team-plan", "create", "--file", "-", "--confirm")
			if code != 2 || problem == "" {
				t.Fatal("invalid Codex selection accepted", code, invalid)
			}
		}
		dual := strings.Replace(item(selection), `"codex_execution_selection":`, `"model_execution_selection":null,"codex_execution_selection":`, 1)
		body := `{"team_id":"team-one","expected_configuration_version":2,"items":[` + dual + `],"idempotency_key":"codex-original-create"}`
		if _, err := expertPlanObject("-", strings.NewReader(body), "create"); err == nil {
			t.Fatal("dual native/Codex selection accepted")
		}
		if calls != 0 {
			t.Fatal("invalid input reached server", calls)
		}
	})
}

func TestExpertPlanCLIModelSelection(t *testing.T) {
	selection := `{"deployment_id":"owned-a","deployment_revision":2,"deployment_sha256":"` + strings.Repeat("a", 64) + `","parameters":{"max_steps":2,"max_output_tokens":100},"budget":{"token_limit":20000,"cost_limit_micros":20000,"currency":"CNY"}}`
	payload := func(value string) string {
		return `{"team_id":"team-a","expected_configuration_version":2,"items":[{"task_id":"task-a","expected_task_version":1,"role_key":"developer","model_execution_selection":` + value + `}],"idempotency_key":"model-selection-key"}`
	}
	body, err := expertPlanObject("-", strings.NewReader(payload(selection)), "create")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"max_output_tokens":100`) || !strings.Contains(string(raw), `"token_limit":20000`) {
		t.Fatal(string(raw))
	}
	for _, invalid := range []string{
		strings.Replace(selection, `"max_steps":2`, `"max_steps":17`, 1),
		strings.Replace(selection, `"max_output_tokens":100`, `"max_output_tokens":1e2`, 1),
		strings.Replace(selection, `"parameters":{`, `"parameters":{"api_key":"private",`, 1),
		strings.Replace(selection, `"token_limit":20000`, `"token_limit":"private"`, 1),
		strings.Replace(selection, `"currency":"CNY"`, `"currency":"EUR"`, 1),
	} {
		if _, err = expertPlanObject("-", strings.NewReader(payload(invalid)), "create"); err == nil {
			t.Fatal("invalid model selection passed local gate", invalid)
		}
	}
}
