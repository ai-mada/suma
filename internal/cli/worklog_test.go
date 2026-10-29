package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestWorklogsCLIReadRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, path, query string
		args              []string
	}{
		{"list", "/api/v1/worklogs", "cursor=next%2B%2F%3D&from_date=2026-10-01&limit=50&member_id=human-one&project_id=project-one&status=active&task_id=task-one&to_date=2026-10-31", []string{"list", "--task", "task-one", "--member", "human-one", "--from-date", "2026-10-01", "--to-date", "2026-10-31", "--status", "active", "--limit", "50", "--cursor", "next+/="}},
		{"list_defaults", "/api/v1/worklogs", "from_date=2026-10-29&limit=20&project_id=project-one&status=all&to_date=2026-10-29", []string{"list", "--from-date", "2026-10-29", "--to-date", "2026-10-29"}},
		{"get", "/api/v1/worklogs/log-one", "project_id=project-one", []string{"get", "log-one"}},
		{"history", "/api/v1/worklogs/log-one/revisions", "cursor=next%2B%2F%3D&limit=5&project_id=project-one", []string{"history", "log-one", "--limit", "5", "--cursor", "next+/="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.RawQuery != tc.query || r.Header.Get("Accept-Language") != "en-US" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Suma-Expected-User") != "" || r.ContentLength > 0 {
					t.Errorf("unexpected read: %s %s", r.Method, r.URL)
				}
				_, _ = io.WriteString(w, `{"items":[],"scope_version":"original-scope","version":9007199254740993,"note":"保留原文"}`)
			}))
			defer server.Close()
			code, out, stderr := contextInvoke(t, server.URL, "en-US", "", append([]string{"worklogs"}, tc.args...)...)
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "保留原文") || !strings.Contains(out, "original-scope") {
				t.Fatal(code, out, stderr, calls)
			}
		})
	}
}

func TestWorklogsCLIExplicitWrites(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		args             []string
	}{
		{"create", "/api/v1/worklogs", `{"project_id":"project-one","task_id":"task-one","expected_task_version":12,"member_id":"human-one","work_date":"2026-10-29","minutes":75,"note":"人工记录 原文","on_behalf_ack":false,"idempotency_key":"original-create-key"}`, []string{"create", "--task", "task-one", "--task-version", "12", "--member", "human-one", "--date", "2026-10-29", "--minutes", "75", "--note", "人工记录 原文", "--operation", "original-create-key"}},
		{"correct", "/api/v1/worklogs/log-one/corrections", `{"project_id":"project-one","expected_version":2,"task_id":"task-two","expected_task_version":13,"work_date":"2026-10-28","minutes":90,"note":"更正原文","reason":"重新核对记录","on_behalf_ack":true,"idempotency_key":"original-correct-key"}`, []string{"correct", "log-one", "--version", "2", "--task", "task-two", "--task-version", "13", "--date", "2026-10-28", "--minutes", "90", "--note", "更正原文", "--reason", "重新核对记录", "--on-behalf-ack", "--operation", "original-correct-key"}},
		{"void", "/api/v1/worklogs/log-one/void", `{"project_id":"project-one","expected_version":3,"reason":"重复人工记录","on_behalf_ack":true,"idempotency_key":"original-void-key"}`, []string{"void", "log-one", "--version", "3", "--reason", "重复人工记录", "--on-behalf-ack", "--operation", "original-void-key"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var actual, expected map[string]any
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&actual); err != nil {
					t.Fatal(err)
				}
				decoder = json.NewDecoder(strings.NewReader(tc.body))
				decoder.UseNumber()
				_ = decoder.Decode(&expected)
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.URL.RawQuery != "" || !reflect.DeepEqual(actual, expected) || r.Header.Get("Accept-Language") != "zh-CN" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Errorf("changed explicit worklog request: %s %s %#v", r.Method, r.URL, actual)
				}
				_, _ = io.WriteString(w, `{"id":"log-one","version":3,"source":"human_manual","note":"服务端原回执"}`)
			}))
			defer server.Close()
			code, out, stderr := contextInvoke(t, server.URL, "zh-CN", "", append([]string{"worklogs"}, tc.args...)...)
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, "human_manual") || !strings.Contains(out, "服务端原回执") {
				t.Fatal(code, out, stderr, calls)
			}
		})
	}
}

func TestWorklogsCLIInvalidInputNeverWrites(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	create := []string{"create", "--task", "task-one", "--task-version", "12", "--member", "human-one", "--date", "2026-10-29", "--minutes", "75", "--operation", "original-create-key"}
	correct := []string{"correct", "log-one", "--version", "2", "--task", "task-one", "--task-version", "12", "--date", "2026-10-29", "--minutes", "75", "--reason", "核对更正", "--operation", "original-correct-key"}
	void := []string{"void", "log-one", "--version", "2", "--reason", "重复记录", "--operation", "original-void-key"}
	invalid := [][]string{
		{}, {"agent"}, {"get", "../foreign"}, {"get", "log-one", "extra"}, {"history", "log-one", "--limit", "51"},
		{"list"}, {"list", "--from-date", "2026-10-01"}, {"list", "--from-date", "2026-10-01", "--to-date", "2026-11-01"},
		{"list", "--from-date", "2026-10-29", "--to-date", "2026-10-28"}, {"list", "--from-date", "2026-02-30", "--to-date", "2026-03-01"},
		{"list", "--from-date", "2026-10-01", "--to-date", "2026-10-31", "--status", "unknown"},
		append(append([]string{}, create...), "--task-version", "0"), append(append([]string{}, create...), "--minutes", "0"),
		append(append([]string{}, create...), "--minutes", "1441"), append(append([]string{}, create...), "--minutes", "1.5"),
		append(append([]string{}, create...), "--date", "2026-02-30"), append(append([]string{}, create...), "--member", "../other"),
		append(append([]string{}, create...), "--operation", "short"), append(append([]string{}, create...), "--note", strings.Repeat("中", 201)),
		append(append([]string{}, correct...), "--version", "0"), append(append([]string{}, correct...), "--task-version", "0"),
		append(append([]string{}, correct...), "--reason", "  "), append(append([]string{}, correct...), "--member", "different-person"),
		append(append([]string{}, void...), "--version", "0"), append(append([]string{}, void...), "--reason", strings.Repeat("中", 201)),
		append(append([]string{}, void...), "--operation", "bad\x00operation"),
	}
	for _, language := range []string{"en-US", "zh-CN"} {
		for _, args := range invalid {
			code, _, stderr := contextInvoke(t, server.URL, language, "", append([]string{"worklogs"}, args...)...)
			if code != 2 || !strings.Contains(stderr, `"message_key": "cli.worklog.`) || !strings.Contains(stderr, `"localized": true`) || !strings.Contains(stderr, `"locale": "`+language+`"`) {
				t.Errorf("invalid input not localized: %v %d %s", args, code, stderr)
			}
		}
	}
	if calls != 0 {
		t.Fatal("invalid input reached HTTP", calls)
	}
}

func TestWorklogsCLIHelpIsBilingualAndReadOnly(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, language := range []string{"en-US", "zh-CN"} {
		for _, args := range [][]string{{"worklogs", "--help"}, {"help"}} {
			code, out, stderr := contextInvoke(t, server.URL, language, "", args...)
			for _, term := range []string{"worklogs list", "worklogs get", "worklogs create", "worklogs correct", "worklogs void", "worklogs history", "--project", "--task-version", "--on-behalf-ack", "--operation", "--cursor", "--from-date", "--to-date"} {
				if !strings.Contains(out, term) {
					t.Error("missing help", term, out)
				}
			}
			if code != 0 || stderr != "" || language == "zh-CN" && !strings.Contains(out, "人工工时") || language == "en-US" && !strings.Contains(out, "Manual human worklogs") {
				t.Fatal(code, out, stderr)
			}
		}
	}
	if calls != 0 {
		t.Fatal("help made HTTP requests", calls)
	}
}

func TestWorklogsCLICurrentServerAuthorityWithoutRetry(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusConflict} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/worklogs/log-one/void" {
				t.Error("unexpected preflight or implicit action", r.Method, r.URL)
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"current_authority","message":"Current access changed 原文","details":{"expected_version":2}}}`)
		}))
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", server.URL, "--project", "project-one", "--token", "fixture-token", "--language", "zh-CN", "--expected-user", "", "--expected-organization", "", "worklogs", "void", "log-one", "--version", "2", "--reason", "重复记录", "--operation", "original-void-key"}, strings.NewReader(""), &out, &stderr)
		server.Close()
		if code != 1 || calls != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "Current access changed 原文") || !strings.Contains(stderr.String(), `"expected_version": 2`) {
			t.Fatal(code, calls, stderr.String())
		}
	}
}

func TestWorklogsCLIProjectRequiredForWrites(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, args := range [][]string{
		{"create", "--task", "task-one", "--task-version", "1", "--member", "human-one", "--date", "2026-10-29", "--minutes", "30", "--operation", "original-create-key"},
		{"correct", "log-one", "--version", "1", "--task", "task-one", "--task-version", "1", "--date", "2026-10-29", "--minutes", "30", "--reason", "correction", "--operation", "original-correct-key"},
		{"void", "log-one", "--version", "1", "--reason", "duplicate", "--operation", "original-void-key"},
	} {
		var out, stderr bytes.Buffer
		all := []string{"--control", server.URL, "--project", "", "--token", "fixture-token", "--language", "en-US", "worklogs"}
		code := Execute(context.Background(), append(all, args...), strings.NewReader(""), &out, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "cli.project.required") || out.Len() != 0 {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
	if calls != 0 {
		t.Fatal("unscoped worklog mutation reached HTTP", calls)
	}
}
