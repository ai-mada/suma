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
	"strings"
	"testing"
)

const compositionCreateFixture = `{"task_id":"integration-task","expected_task_version":2,"workflow_id":"workflow-one","workflow_version":3,"producer_node_id":"code","sources":[{"task_id":"task-a","expected_task_version":4,"source_node_id":"code"},{"task_id":"task-b","expected_task_version":5,"source_node_id":"code"}],"idempotency_key":"original-composition-preview"}`
const compositionBindFixture = `{"expected_task_version":1,"expected_binding_version":0,"composition_id":"plan-one","expected_composition_version":2,"acknowledge":true,"reason":"明确继承已验收合成","idempotency_key":"original-baseline-bind"}`

func TestCandidateCompositionCLI(t *testing.T) {
	t.Run("bilingual_help_and_confirmation", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, language := range []string{"en-US", "zh-CN"} {
			for _, command := range []string{"candidate-compositions", "task-code-baseline"} {
				code, out, err := contextInvoke(t, s.URL, language, "", command, "--help")
				if code != 0 || err != "" || !strings.Contains(out, "--confirm") || !strings.Contains(out, "expected_binding_version") || language == "zh-CN" && !strings.Contains(out, "代码基线") {
					t.Fatal(code, out, err)
				}
			}
			for _, args := range [][]string{{"candidate-compositions", "create"}, {"candidate-compositions", "start", "plan-one"}, {"candidate-compositions", "command", "plan-one"}, {"task-code-baseline", "bind", "task-c"}} {
				code, _, err := contextInvoke(t, s.URL, language, "{}", append(args, "--file", "-")...)
				if code != 2 || !strings.Contains(err, "cli.candidate_composition.write_options") {
					t.Fatal(code, err)
				}
			}
		}
		if calls != 0 {
			t.Fatal("help/unconfirmed command made a request")
		}
	})
	t.Run("exact_scoped_read_routes", func(t *testing.T) {
		for _, test := range []struct {
			args        []string
			path, query string
		}{
			{[]string{"candidate-compositions", "options", "--task", "integration-task"}, "/api/v1/candidate-compositions/options", "project_id=project-one&task_id=integration-task"},
			{[]string{"candidate-compositions", "list", "--task", "integration-task", "--limit", "5", "--before", "original+/="}, "/api/v1/candidate-compositions", "before=original%2B%2F%3D&limit=5&project_id=project-one&task_id=integration-task"},
			{[]string{"candidate-compositions", "get", "plan-one"}, "/api/v1/candidate-compositions/plan-one", "project_id=project-one"},
			{[]string{"task-code-baseline", "get", "task-c"}, "/api/v1/tasks/task-c/code-baseline", "project_id=project-one"},
		} {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != test.path || r.URL.RawQuery != test.query || r.Header.Get("X-Suma-Expected-User") != "" {
					t.Error("wrong read route", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(`{"original":"原始事实","version":9007199254740993}`))
			}))
			code, out, err := contextInvoke(t, s.URL, "en-US", "", test.args...)
			s.Close()
			if code != 0 || err != "" || calls != 1 || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "原始事实") {
				t.Fatal(code, out, err, calls)
			}
		}
	})
	t.Run("typed_write_preserves_order_key_and_current_headers", func(t *testing.T) {
		for _, test := range []struct {
			args       []string
			body, path string
		}{
			{[]string{"candidate-compositions", "create"}, compositionCreateFixture, "/api/v1/candidate-compositions"},
			{[]string{"candidate-compositions", "start", "plan-one"}, `{"expected_version":1,"acknowledge":true,"idempotency_key":"original-composition-start"}`, "/api/v1/candidate-compositions/plan-one/start"},
			{[]string{"candidate-compositions", "command", "plan-one"}, `{"expected_version":1,"action":"dismiss","reason":"保留来源","idempotency_key":"original-composition-dismiss"}`, "/api/v1/candidate-compositions/plan-one/commands"},
			{[]string{"task-code-baseline", "bind", "task-c"}, compositionBindFixture, "/api/v1/tasks/task-c/code-baseline"},
		} {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				d := json.NewDecoder(strings.NewReader(test.body))
				d.UseNumber()
				var expected map[string]any
				_ = d.Decode(&expected)
				expected["project_id"] = "project-one"
				want, _ := json.Marshal(expected)
				if r.Method != "POST" || r.URL.Path != test.path || string(raw) != string(want) || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("changed ordered source, operation or original identity", r.Method, r.URL, string(raw))
				}
				_, _ = w.Write([]byte(`{"state":"ready"}`))
			}))
			args := append(test.args, "--file", "-", "--confirm")
			code, _, err := contextInvoke(t, s.URL, "zh-CN", test.body, args...)
			s.Close()
			if code != 0 || err != "" || calls != 1 {
				t.Fatal(code, err, calls)
			}
		}
		file := filepath.Join(t.TempDir(), "create.json")
		if err := os.WriteFile(file, []byte(compositionCreateFixture), 0600); err != nil {
			t.Fatal(err)
		}
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
		defer s.Close()
		code, _, err := contextInvoke(t, s.URL, "en-US", "", "candidate-compositions", "create", "--file", file, "--confirm")
		if code != 0 || err != "" {
			t.Fatal(code, err)
		}
	})
	t.Run("strict_invalid_input_never_reaches_http", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
		defer s.Close()
		for _, body := range []string{
			strings.Replace(compositionCreateFixture, `"expected_task_version":2`, `"expected_task_version":2.0`, 1),
			strings.Replace(compositionCreateFixture, `"workflow_version":3`, `"workflow_version":3e0`, 1),
			strings.Replace(compositionCreateFixture, `"task_id":"task-b"`, `"task_id":"task-a"`, 1),
			strings.Replace(compositionCreateFixture, `"sources":`, `"Sources":`, 1),
			strings.Replace(compositionCreateFixture, `"source_node_id":"code"`, `"source_node_id":"code","source_node_id":"other"`, 1),
			strings.Replace(compositionCreateFixture, `"source_node_id":"code"`, `"source_node_id":null`, 1),
			strings.Replace(compositionCreateFixture, `"source_node_id":"code"`, `"source_node_id":"bad\ud800"`, 1),
			strings.Replace(compositionCreateFixture, `"workflow_id":`, `"candidate_commit":"invented","workflow_id":`, 1),
			strings.Replace(compositionCreateFixture, `"workflow_id":`, `"project_id":"wrong","workflow_id":`, 1),
			strings.Replace(compositionCreateFixture, `"idempotency_key":"original-composition-preview"`, `"idempotency_key":"short"`, 1),
		} {
			code, _, err := contextInvoke(t, s.URL, "en-US", body, "candidate-compositions", "create", "--file", "-", "--confirm")
			if code != 2 || err == "" {
				t.Fatal("invalid create accepted", body, code, err)
			}
		}
		for _, test := range []struct {
			args []string
			body string
		}{
			{[]string{"candidate-compositions", "start", "plan-one"}, `{"expected_version":1,"acknowledge":false,"idempotency_key":"original-start"}`},
			{[]string{"candidate-compositions", "command", "plan-one"}, `{"expected_version":1,"action":"accept","reason":"bad","idempotency_key":"original-dismiss"}`},
			{[]string{"task-code-baseline", "bind", "task-c"}, strings.Replace(compositionBindFixture, `"expected_binding_version":0`, `"expected_binding_version":-0`, 1)},
			{[]string{"task-code-baseline", "bind", "task-c"}, strings.Replace(compositionBindFixture, `"expected_binding_version":0`, `"expected_binding_version":"0"`, 1)},
			{[]string{"task-code-baseline", "bind", "task-c"}, strings.Replace(compositionBindFixture, `"acknowledge":true`, `"acknowledge":null`, 1)},
		} {
			code, _, _ := contextInvoke(t, s.URL, "zh-CN", test.body, append(test.args, "--file", "-", "--confirm")...)
			if code != 2 {
				t.Fatal("invalid typed write accepted", code, test.body)
			}
		}
		for _, args := range [][]string{{"candidate-compositions", "options"}, {"candidate-compositions", "list", "--limit", "01"}, {"candidate-compositions", "get", "../foreign"}, {"task-code-baseline", "get", "task", "extra"}} {
			code, _, _ := contextInvoke(t, s.URL, "en-US", "", args...)
			if code != 2 {
				t.Fatal("invalid route accepted", args, code)
			}
		}
		if calls != 0 {
			t.Fatal("invalid input made HTTP calls", calls)
		}
	})
	t.Run("server_authority_and_no_automatic_retry_or_browser", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Header.Get("X-Suma-Expected-User") != "" || r.Header.Get("Authorization") != "Bearer fixture-token" {
				t.Error("CLI manufactured browser identity")
			}
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"Current authority revoked 原文"}}`))
		}))
		defer s.Close()
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", s.URL, "--project", "project-one", "--token", "fixture-token", "--expected-user", "", "--expected-organization", "", "candidate-compositions", "create", "--file", "-", "--confirm"}, strings.NewReader(compositionCreateFixture), &out, &stderr)
		if code != 1 || calls != 1 || !strings.Contains(stderr.String(), "Current authority revoked 原文") {
			t.Fatal(code, stderr.String(), calls)
		}
	})
}
