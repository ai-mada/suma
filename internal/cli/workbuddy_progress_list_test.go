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
	"sync/atomic"
	"testing"
)

func TestWorkBuddyProgressListCLIReadsExactPage(t *testing.T) {
	for _, tc := range []struct {
		name, limit, before, filter, response string
		args                                  []string
	}{
		{"first page", "10", "", "", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `]}`, nil},
		{"first page with continuation", "1", "", "", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `],"next_cursor":"first-cursor"}`, []string{"--limit", "1"}},
		{"continuation", "1", "first-cursor", "", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `]}`, []string{"--limit", "1", "--before", "first-cursor"}},
		{"empty page", "20", "", "", `{"project_id":"project-one","items":[]}`, []string{"--limit=20"}},
		{"filtered empty batch", "10", "", "attention", `{"project_id":"project-one","items":[],"filter":"attention","next_cursor":"first-cursor"}`, []string{"--filter", "attention"}},
		{"filtered short continuation", "10", "first-cursor", "attention", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `],"filter":"attention","next_cursor":"dGVzdC1uZXh0"}`, []string{"--filter=attention", "--before", "first-cursor"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				q := r.URL.Query()
				wantKeys := 2
				if tc.before != "" {
					wantKeys++
				}
				if tc.filter != "" {
					wantKeys++
				}
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workbuddy-cloud-progress" ||
					len(q) != wantKeys || len(q["limit"]) != 1 || len(q["project_id"]) != 1 ||
					q.Get("project_id") != "project-one" || q.Get("limit") != tc.limit || q.Get("before") != tc.before ||
					q.Get("filter") != tc.filter || tc.filter != "" && len(q["filter"]) != 1 ||
					r.Header.Get("Authorization") != "Bearer test-secret" || r.ContentLength != 0 {
					t.Error("WorkBuddy list did not preserve the exact request scope", r.Method, r.URL)
				}
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()

			code, stdout, stderr := invoke(t, server.URL, append([]string{"run", "workbuddy-list"}, tc.args...)...)
			var got, want any
			if code != 0 || stderr != "" || calls.Load() != 1 || json.Unmarshal([]byte(stdout), &got) != nil ||
				json.Unmarshal([]byte(tc.response), &want) != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout, stderr)
			}
		})
	}
}

func TestWorkBuddyProgressListCLIRejectsUnverifiedPage(t *testing.T) {
	const basePage = `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `]}`
	const filteredPage = `{"project_id":"project-one","filter":"attention","items":[` + workBuddyProgressJSON + `]}`
	for _, tc := range []struct {
		name, response string
		status         int
		code           string
		filtered       bool
	}{
		{"wrong project", strings.Replace(basePage, "project-one", "secret-marker", 1), 200, "workbuddy_progress_invalid", false},
		{"foreign item", strings.Replace(basePage, workBuddyProgressJSON, strings.Replace(workBuddyProgressJSON, "project-one", "secret-marker", 1), 1), 200, "workbuddy_progress_invalid", false},
		{"unexpected private field", strings.ReplaceAll(basePage, `"run_id":"run-1"`, `"run_id":"run-1","connection_id":"secret-marker"`), 200, "workbuddy_progress_invalid", false},
		{"missing observation fact", strings.Replace(basePage, `,"observation_failed":false`, "", 1), 200, "workbuddy_progress_invalid", false},
		{"page exceeds requested size", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + "," + strings.Replace(workBuddyProgressJSON, "run-1", "run-2", 1) + `]}`, 200, "workbuddy_progress_invalid", false},
		{"empty continuation", `{"project_id":"project-one","items":[],"next_cursor":"new-cursor"}`, 200, "workbuddy_progress_invalid", false},
		{"repeated continuation", strings.TrimSuffix(basePage, "}") + `,"next_cursor":"first-cursor"}`, 200, "workbuddy_progress_invalid", false},
		{"current refusal", `{"error":{"code":"forbidden","message":"secret-marker"}}`, 403, "forbidden", false},
		{"missing filter echo", basePage, 200, "workbuddy_progress_invalid", true},
		{"unrequested filter echo", filteredPage, 200, "workbuddy_progress_invalid", false},
		{"filtered repeated empty continuation", `{"project_id":"project-one","filter":"attention","items":[],"next_cursor":"first-cursor"}`, 200, "workbuddy_progress_invalid", true},
		{"filtered unrelated item", strings.Replace(filteredPage, `,"node_reason":"cloud.delivery_required"`, "", 1), 200, "workbuddy_progress_invalid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()
			args := []string{"run", "workbuddy-list", "--limit", "1", "--before", "first-cursor"}
			if tc.filtered {
				args = append(args, "--filter", "attention")
			}
			code, stdout, stderr := invoke(t, server.URL, args...)
			if code != 1 || stdout != "" || !strings.Contains(stderr, `"code": "`+tc.code+`"`) || strings.Contains(stderr, "secret-marker") {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestWorkBuddyProgressListCLIStrictOptionsAndHelp(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	for _, args := range [][]string{
		{"run-1"}, {"--limit", "0"}, {"--limit", "21"}, {"--limit", "0x10"}, {"--limit", "1", "--limit", "2"},
		{"--filter", ""}, {"--filter", "all"}, {"--filter", "Attention"}, {"--filter", "attention", "--filter", "attention"},
		{"--before", ""}, {"--before", "bad+cursor"}, {"--before", strings.Repeat("a", 1025)}, {"--before", "one", "--before", "two"},
	} {
		code, stdout, _ := invoke(t, server.URL, append([]string{"run", "workbuddy-list"}, args...)...)
		if code != 2 || stdout != "" || calls.Load() != 0 {
			t.Fatalf("invalid args %v returned %d, or sent a request", args, code)
		}
	}
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "--language", "en-US", "run", "workbuddy-list"}, nil, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || calls.Load() != 0 || !strings.Contains(stderr.String(), "--project is required") {
		t.Fatalf("missing project exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
	}
	for _, locale := range []string{"en-US", "zh-CN"} {
		for _, args := range [][]string{{"--help"}, {"run", "workbuddy-list", "--help"}} {
			code, stdout, stderr := invoke(t, server.URL, append([]string{"--language", locale}, args...)...)
			if code != 0 || stderr != "" || calls.Load() != 0 || !strings.Contains(stdout, "run workbuddy-list [--limit 1..20] [--filter attention] [--before NEXT_CURSOR]") {
				t.Fatalf("help %s %v exit=%d stdout=%q stderr=%q", locale, args, code, stdout, stderr)
			}
		}
	}
}
