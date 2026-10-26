package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const diagnosisCreateFixture = `{"issue_id":"issue-one","expected_issue_version":2,"run_id":"failed-run","node_id":"test","attempt":1,"expected_source_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","goal":"Review fixed failure 原文","locale":"zh-CN","tool_profile_id":"diagnostician","expected_tool_version":3,"max_steps":1,"max_output_tokens":4096,"acknowledge_model_use":true,"idempotency_key":"original-diagnosis-create"}`

func TestFailureDiagnosisCLI(t *testing.T) {
	t.Run("help_confirmation_and_original_browser_view", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, language := range []string{"en-US", "zh-CN"} {
			code, out, err := contextInvoke(t, s.URL, language, "", "failure-diagnosis", "--help")
			if code != 0 || err != "" || !strings.Contains(out, "--confirm") || !strings.Contains(out, "--attempt") || language == "zh-CN" && !strings.Contains(out, "失败分诊") {
				t.Fatal(code, out, err)
			}
			code, _, err = contextInvoke(t, s.URL, language, diagnosisCreateFixture, "failure-diagnosis", "create", "--file", "-")
			if code != 2 || !strings.Contains(err, "cli.failure_diagnosis.write_options") {
				t.Fatal("unconfirmed request accepted", code, err)
			}
		}
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", s.URL, "--project", "project-one", "--token", "fixture-token", "--expected-user", "", "--expected-organization", "", "failure-diagnosis", "create", "--file", "-", "--confirm"}, strings.NewReader(diagnosisCreateFixture), &out, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "cli.failure_diagnosis.browser_required") || calls != 0 {
			t.Fatal("missing original browser view made a request", code, stderr.String(), calls)
		}
	})
	t.Run("fixed_scoped_read_routes", func(t *testing.T) {
		for _, test := range []struct {
			args        []string
			path, query string
		}{
			{[]string{"context", "--run", "failed-run", "--node", "test", "--attempt", "1"}, "/api/v1/failure-diagnosis/context", "attempt=1&node_id=test&project_id=project-one&run_id=failed-run"},
			{[]string{"list", "--run", "failed-run", "--node", "test", "--attempt", "1", "--limit", "5", "--before", "original+/="}, "/api/v1/failure-diagnosis/analyses", "attempt=1&before=original%2B%2F%3D&limit=5&node_id=test&project_id=project-one&run_id=failed-run"},
			{[]string{"get", "analysis-one"}, "/api/v1/failure-diagnosis/analyses/analysis-one", "project_id=project-one"},
		} {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != test.path || r.URL.RawQuery != test.query || r.Header.Get("X-Suma-Expected-User") != "" || r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("wrong fixed scoped read", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(`{"version":9007199254740993,"original_output":"模型原件不翻译"}`))
			}))
			code, out, err := contextInvoke(t, s.URL, "en-US", "", append([]string{"failure-diagnosis"}, test.args...)...)
			s.Close()
			if code != 0 || err != "" || calls != 1 || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "模型原件不翻译") {
				t.Fatal(code, out, err, calls)
			}
		}
	})
	t.Run("exact_typed_write_body_and_current_headers", func(t *testing.T) {
		for _, test := range []struct {
			args       []string
			body, path string
		}{
			{[]string{"create"}, diagnosisCreateFixture, "/api/v1/failure-diagnosis/analyses"},
			{[]string{"command", "analysis-one"}, `{"expected_version":2,"action":"revise","repair":{"title":"明确修复 原文","description":"Review the actual failure","priority":"medium","acceptance_criteria":["Independent regression passes"]},"reason":"人工修订","idempotency_key":"original-diagnosis-revise"}`, "/api/v1/failure-diagnosis/analyses/analysis-one/commands"},
			{[]string{"command", "analysis-one"}, `{"expected_version":3,"action":"adopt","reason":"明确创建草稿","acknowledge_draft":true,"idempotency_key":"original-diagnosis-adopt"}`, "/api/v1/failure-diagnosis/analyses/analysis-one/commands"},
			{[]string{"command", "analysis-one"}, `{"expected_version":4,"action":"dismiss","reason":"保留原运行","idempotency_key":"original-diagnosis-dismiss"}`, "/api/v1/failure-diagnosis/analyses/analysis-one/commands"},
		} {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				var want map[string]any
				d := json.NewDecoder(strings.NewReader(test.body))
				d.UseNumber()
				_ = d.Decode(&want)
				want["project_id"] = "project-one"
				expected, _ := json.Marshal(want)
				if r.Method != "POST" || r.URL.Path != test.path || string(raw) != string(expected) || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("changed original body, operation or browser view", r.Method, r.URL, string(raw))
				}
				_, _ = w.Write([]byte(`{"state":"ready"}`))
			}))
			args := append([]string{"failure-diagnosis"}, test.args...)
			args = append(args, "--file", "-", "--confirm")
			code, _, err := contextInvoke(t, s.URL, "zh-CN", test.body, args...)
			s.Close()
			if code != 0 || err != "" || calls != 1 {
				t.Fatal(code, err, calls)
			}
		}
	})
	t.Run("strict_input_never_sent", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, body := range []string{
			strings.Replace(diagnosisCreateFixture, `"attempt":1`, `"attempt":1.0`, 1),
			strings.Replace(diagnosisCreateFixture, `"attempt":1`, `"attempt":"1"`, 1),
			strings.Replace(diagnosisCreateFixture, `"attempt":1`, `"attempt":1,"attempt":2`, 1),
			strings.Replace(diagnosisCreateFixture, `"attempt":1`, `"Attempt":1`, 1),
			strings.Replace(diagnosisCreateFixture, `"max_steps":1`, `"max_steps":2`, 1),
			strings.Replace(diagnosisCreateFixture, `"max_steps":1`, `"max_steps":null`, 1),
			strings.Replace(diagnosisCreateFixture, `"acknowledge_model_use":true`, `"acknowledge_model_use":false`, 1),
			strings.Replace(diagnosisCreateFixture, `"goal":"Review fixed failure 原文"`, `"goal":"bad\ud800"`, 1),
			strings.Replace(diagnosisCreateFixture, `"issue_id":`, `"credential_hash":"no","issue_id":`, 1),
			strings.Replace(diagnosisCreateFixture, `"issue_id":`, `"project_id":"other-project","issue_id":`, 1),
		} {
			code, _, err := contextInvoke(t, s.URL, "en-US", body, "failure-diagnosis", "create", "--file", "-", "--confirm")
			if code != 2 || err == "" {
				t.Fatal("invalid create accepted", body, code, err)
			}
		}
		for _, body := range []string{
			`{"expected_version":1e0,"action":"dismiss","reason":"No","idempotency_key":"original-key"}`,
			`{"expected_version":1,"action":"adopt","reason":"No","idempotency_key":"original-key"}`,
			`{"expected_version":1,"action":"dismiss","repair":null,"reason":"No","idempotency_key":"original-key"}`,
			`{"expected_version":1,"action":"revise","repair":{"title":"a","title":"b","description":"fix","priority":"high","acceptance_criteria":["check"]},"reason":"No","idempotency_key":"original-key"}`,
			`{"expected_version":1,"action":"revise","repair":{"title":"a","description":"fix","priority":"high","acceptance_criteria":["check"],"run_id":"invented"},"reason":"No","idempotency_key":"original-key"}`,
			`{"expected_version":1,"action":"accept","reason":"No","idempotency_key":"original-key"}`,
		} {
			code, _, _ := contextInvoke(t, s.URL, "zh-CN", body, "failure-diagnosis", "command", "analysis-one", "--file", "-", "--confirm")
			if code != 2 {
				t.Fatal("invalid command accepted", body, code)
			}
		}
		for _, attempt := range []string{"0", "01", "1e0", "+1"} {
			code, _, _ := contextInvoke(t, s.URL, "en-US", "", "failure-diagnosis", "context", "--run", "failed-run", "--node", "test", "--attempt", attempt)
			if code != 2 {
				t.Fatal("noncanonical attempt accepted", attempt)
			}
		}
		if calls != 0 {
			t.Fatal("strict boundary made HTTP requests", calls)
		}
	})
	t.Run("server_browser_authority_not_headers_and_no_retry", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"Browser credential revoked 原文"}}`))
		}))
		defer s.Close()
		code, _, err := contextInvoke(t, s.URL, "en-US", diagnosisCreateFixture, "failure-diagnosis", "create", "--file", "-", "--confirm")
		if code != 1 || calls != 1 || !strings.Contains(err, "Browser credential revoked 原文") {
			t.Fatal(code, err, calls)
		}
	})
}
