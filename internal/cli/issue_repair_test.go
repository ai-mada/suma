package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIssueRepairCLIShow(t *testing.T) {
	const issue = "Issue_1"
	const user = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const organization = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const issueRecord = `{"id":"Issue_1","project_id":"project-one","version":2,"data":{"origin":"test_plan_case","case_key":"untrusted"}}`
	origin := `{"plan_id":"plan-1","run_id":"run-1","case_key":"first_case","plan_revision":3,"plan_sha256":"` + strings.Repeat("a", 64) + `"}`
	response := func(record, source string) string {
		return `{"issue":` + record + `,"cycle":1` + source + `}`
	}
	tests := []struct {
		name string
		body string
		ok   bool
	}{
		{"original case", response(issueRecord, `,"original_case":`+origin), true},
		{"no protected source", response(issueRecord, ""), true},
		{"opaque metadata", response(strings.Replace(issueRecord, `"untrusted"`, strings.Repeat("[", 30)+"0"+strings.Repeat("]", 30), 1), ""), true},
		{"ambiguous issue identity", response(strings.Replace(issueRecord, `"id":"Issue_1"`, `"id":"Issue_1","id":"Issue_1"`, 1), ""), false},
		{"different issue", response(strings.Replace(issueRecord, issue, "Issue_2", 1), ""), false},
		{"different project", response(strings.Replace(issueRecord, "project-one", "project-two", 1), ""), false},
		{"malformed origin", response(issueRecord, `,"original_case":`+strings.Replace(origin, `"plan_revision":3`, `"plan_revision":0`, 1)), false},
		{"null origin", response(issueRecord, `,"original_case":null`), false},
		{"ambiguous origin", response(issueRecord, `,"original_case":`+origin+`,"Original_Case":`+origin), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/issues/"+issue+"/repair" ||
					r.URL.RawQuery != "project_id=project-one" || r.Header.Get("Authorization") != "Bearer test-secret" ||
					r.Header.Get("X-Suma-Expected-User") != user || r.Header.Get("X-Suma-Expected-Organization") != organization {
					t.Error("unexpected issue repair request", r.Method, r.URL)
				}
				if body, err := io.ReadAll(r.Body); err != nil || len(body) != 0 {
					t.Error("issue repair read sent a body", err)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			code, out, stderr := invoke(t, server.URL, "--expected-user", user, "--expected-organization", organization, "issue-repair", "show", issue)
			if calls.Load() != 1 {
				t.Fatal("expected one read", calls.Load(), code, stderr)
			}
			if !tc.ok {
				if code != 1 || out != "" || !strings.Contains(stderr, `"response"`) {
					t.Fatal("invalid response was not refused", code, out, stderr)
				}
				return
			}
			var got, want any
			if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &got) != nil ||
				json.Unmarshal([]byte(tc.body), &want) != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("scoped read changed or omitted the response", code, out, stderr)
			}
		})
	}
}

func TestIssueRepairCLIHelpAndUsage(t *testing.T) {
	for _, tc := range []struct{ locale, label string }{{"en-US", "original case reference"}, {"zh-CN", "原始用例引用"}} {
		for _, args := range [][]string{{"--help"}, {"issue-repair", "--help"}, {"issue-repair", "show", "--help"}} {
			var out, stderr strings.Builder
			code := Execute(context.Background(), append([]string{"--language", tc.locale}, args...), strings.NewReader(""), &out, &stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), "issue-repair show ISSUE") || !strings.Contains(out.String(), tc.label) {
				t.Errorf("%s %v missing issue repair help: code=%d", tc.locale, args, code)
			}
		}
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"forbidden","message":"Access denied"}}`)
	}))
	defer server.Close()
	for _, args := range [][]string{{}, {"show"}, {"show", "../other"}, {"show", "--unexpected"}, {"show", "valid-issue", "extra"}} {
		code, out, stderr := invoke(t, server.URL, append([]string{"issue-repair"}, args...)...)
		if code != 2 || out != "" || !strings.Contains(stderr, "cli.issue_repair.usage") {
			t.Errorf("invalid input %q was not refused as usage", args)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid inputs reached the server", calls.Load())
	}
	code, out, stderr := invoke(t, server.URL, "issue-repair", "show", "valid-issue")
	if code != 1 || out != "" || !strings.Contains(stderr, `"forbidden"`) || calls.Load() != 1 {
		t.Fatal("server refusal did not remain a read error", code, calls.Load(), stderr)
	}
}
