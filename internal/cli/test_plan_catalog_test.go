package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTestPlanCatalogCLIReadsExactProjectPagesAndPlan(t *testing.T) {
	const user = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const org = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const cursor = "TmV4dF9wYWdlLQ"
	tests := []struct {
		name     string
		args     []string
		path     string
		query    url.Values
		response string
	}{
		{"filtered", []string{"list", "--state", "published", "--task", "Task_1", "--limit", "7", "--before", cursor},
			"/api/v1/test-plans", url.Values{"project_id": {"project-one"}, "state": {"published"}, "task_id": {"Task_1"}, "limit": {"7"}, "before": {cursor}},
			`{"items":[{"id":"Plan_2","state":"published","task_id":"Task_1","latest_revision":2}],"total":8,"next_cursor":"TmV4dF9wYWdlLQ"}`},
		{"unfiltered", []string{"list", "--before=Plan-1"},
			"/api/v1/test-plans", url.Values{"project_id": {"project-one"}, "limit": {"20"}, "before": {"Plan-1"}},
			`{"items":[],"total":1,"next_cursor":""}`},
		{"exact plan", []string{"show", "Plan-1"},
			"/api/v1/test-plans/Plan-1", url.Values{"project_id": {"project-one"}},
			`{"id":"Plan-1","task_id":"task-1","state":"draft","published_revision":0,"allowed_actions":[]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != tc.path || !reflect.DeepEqual(r.URL.Query(), tc.query) ||
					r.Header.Get("Authorization") != "Bearer test-secret" ||
					r.Header.Get("X-Suma-Expected-User") != user || r.Header.Get("X-Suma-Expected-Organization") != org {
					t.Error("unexpected test plan query", r.Method, r.URL)
				}
				if body, err := io.ReadAll(r.Body); err != nil || len(body) != 0 {
					t.Error("test plan read sent a body", err)
				}
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()

			args := append([]string{"--expected-user", user, "--expected-organization", org, "test-plan"}, tc.args...)
			code, out, stderr := invoke(t, server.URL, args...)
			if code != 0 || stderr != "" || calls.Load() != 1 {
				t.Fatal("scoped read failed", code, calls.Load(), stderr)
			}
			var got, want any
			if json.Unmarshal([]byte(out), &got) != nil || json.Unmarshal([]byte(tc.response), &want) != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("test plan projection was changed", out)
			}
		})
	}
}

func TestTestPlanCatalogCLIRejectsBadOptionsBeforeTransport(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"forbidden","message":"Access denied"}}`)
	}))
	defer server.Close()

	for _, args := range [][]string{
		{}, {"unknown"}, {"list", "unexpected"}, {"list", "--unexpected", "value"},
		{"list", "--state="}, {"list", "--state", "active"}, {"list", "--state", "draft", "--state", "published"},
		{"list", "--task="}, {"list", "--task", "a/b"}, {"list", "--task", "--limit"}, {"list", "--task", "a", "--task", "a"},
		{"list", "--limit="}, {"list", "--limit", "01"}, {"list", "--limit", "101"}, {"list", "--limit", "1", "--limit", "1"},
		{"list", "--before="}, {"list", "--before", "a/b"}, {"list", "--before", "--state"}, {"list", "--before", strings.Repeat("a", 65)},
		{"list", "--before", "Plan-1", "--before", "Plan-2"}, {"list", "--state", "retired", "--before", "%%"},
		{"list", "--state", "retired", "--before", "YQ\n"}, {"list", "--state", "retired", "--before", strings.Repeat("a", 1025)},
		{"show"}, {"show", ""}, {"show", "--bogus"}, {"show", ".."}, {"show", "Plan-1", "--state", "draft"},
	} {
		code, out, stderr := invoke(t, server.URL, append([]string{"test-plan"}, args...)...)
		if code != 2 || out != "" || !strings.Contains(stderr, "cli.test_plan.usage") {
			t.Errorf("invalid arguments %q were not refused locally: code=%d", args, code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached the server", calls.Load())
	}

	code, out, stderr := invoke(t, server.URL, "test-plan", "show", "Plan-1")
	if code != 1 || out != "" || !strings.Contains(stderr, `"forbidden"`) || calls.Load() != 1 {
		t.Fatal("server refusal did not remain a read error", code, calls.Load(), stderr)
	}
}

func TestTestPlanCatalogCLIHelpLanguages(t *testing.T) {
	for _, tc := range []struct{ locale, label, cursorHelp string }{
		{"en-US", "test plans", "original project and filters"},
		{"zh-CN", "测试计划", "保留原项目和筛选条件"},
	} {
		for _, args := range [][]string{{"--help"}, {"test-plan", "--help"}, {"test-plan", "list", "--help"}, {"test-plan", "show", "--help"}} {
			var out, stderr strings.Builder
			code := Execute(context.Background(), append([]string{"--language", tc.locale}, args...), strings.NewReader(""), &out, &stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), tc.label) {
				t.Errorf("%s %v help failed: code=%d", tc.locale, args, code)
			}
			if len(args) > 1 && (!strings.Contains(out.String(), "--state draft|published|retired") || !strings.Contains(out.String(), "test-plan show PLAN") || !strings.Contains(out.String(), tc.cursorHelp)) {
				t.Errorf("%s %v help is missing list or detail syntax", tc.locale, args)
			}
		}
	}
}
