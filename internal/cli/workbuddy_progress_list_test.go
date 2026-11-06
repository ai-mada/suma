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

const workBuddyProgressTaskLinkJSON = `{"run_id":"run-1","node_id":"node-1","task_id":"task-1","task_version":3,"run_relation":"current","task_title":"修复同步冲突"}`

func TestWorkBuddyProgressListCLIReadsExactPage(t *testing.T) {
	for _, tc := range []struct {
		name, limit, before, filter, response string
		args                                  []string
	}{
		{"first page", "10", "", "", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `]}`, nil},
		{"formal task link", "10", "", "", `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `],"task_links":[` + workBuddyProgressTaskLinkJSON + `]}`, nil},
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
	const linkedPage = `{"project_id":"project-one","items":[` + workBuddyProgressJSON + `],"task_links":[` + workBuddyProgressTaskLinkJSON + `]}`
	for _, tc := range []struct {
		name, response string
		status         int
		code           string
		filtered       bool
	}{
		{"wrong project", strings.Replace(basePage, "project-one", "secret-marker", 1), 200, "workbuddy_progress_invalid", false},
		{"foreign item", strings.Replace(basePage, workBuddyProgressJSON, strings.Replace(workBuddyProgressJSON, "project-one", "secret-marker", 1), 1), 200, "workbuddy_progress_invalid", false},
		{"unexpected private field", strings.ReplaceAll(basePage, `"run_id":"run-1"`, `"run_id":"run-1","connection_id":"secret-marker"`), 200, "workbuddy_progress_invalid", false},
		{"orphan task link", strings.Replace(linkedPage, workBuddyProgressTaskLinkJSON, strings.Replace(workBuddyProgressTaskLinkJSON, "node-1", "node-2", 1), 1), 200, "workbuddy_progress_invalid", false},
		{"duplicate task link", strings.Replace(linkedPage, workBuddyProgressTaskLinkJSON, workBuddyProgressTaskLinkJSON+","+workBuddyProgressTaskLinkJSON, 1), 200, "workbuddy_progress_invalid", false},
		{"private task link field", strings.Replace(linkedPage, `"task_id":"task-1"`, `"task_id":"task-1","connection_id":"secret-marker"`, 1), 200, "workbuddy_progress_invalid", false},
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

func TestWorkBuddyProgressListCLIPreservesFormalTaskScope(t *testing.T) {
	const items = `"items":[` + workBuddyProgressJSON + `]`
	inputItem := strings.NewReplacer("cloud.delivery_required", "cloud.waiting_input", "completed", "waiting_input").Replace(workBuddyProgressJSON)
	inputItems := `"items":[` + inputItem + `]`
	const links = `"task_links":[` + workBuddyProgressTaskLinkJSON + `]`
	for _, tc := range []struct {
		name, task, filter, before, fields string
		valid                              bool
	}{
		{"short task page", "task-1", "", "", `"task_id":"task-1","next_cursor":"first-cursor",` + items + "," + links, true},
		{"empty task continuation", "task-1", "", "first-cursor", `"task_id":"task-1","items":[],"next_cursor":"dGVzdC1uZXh0"`, true},
		{"task attention", "task-1", "attention", "", `"task_id":"task-1","filter":"attention",` + items + "," + links, true},
		{"task delivery continuation", "task-1", "delivery", "first-cursor", `"task_id":"task-1","filter":"delivery","next_cursor":"dGVzdC1uZXh0",` + items + "," + links, true},
		{"task delivery empty batch", "task-1", "delivery", "first-cursor", `"task_id":"task-1","filter":"delivery","items":[],"next_cursor":"dGVzdC1uZXh0"`, true},
		{"project delivery with current task", "", "delivery", "", `"filter":"delivery",` + items + "," + links, true},
		{"project delivery missing formal link", "", "delivery", "", `"filter":"delivery",` + items, false},
		{"delivery with rebound task", "task-1", "delivery", "", `"task_id":"task-1","filter":"delivery",` + items + "," + strings.Replace(links, `"run_relation":"current"`, `"run_relation":"changed"`, 1), false},
		{"project delivery with unbound task", "", "delivery", "", `"filter":"delivery",` + items + "," + strings.Replace(links, `"run_relation":"current"`, `"run_relation":"unbound"`, 1), false},
		{"attention still includes rebound task", "task-1", "attention", "", `"task_id":"task-1","filter":"attention",` + items + "," + strings.Replace(links, `"run_relation":"current"`, `"run_relation":"changed"`, 1), true},
		{"delivery returned attention", "task-1", "delivery", "", `"task_id":"task-1","filter":"attention",` + items + "," + links, false},
		{"delivery returned paused run", "task-1", "delivery", "", `"task_id":"task-1","filter":"delivery",` + strings.Replace(items, `"run_state":"running"`, `"run_state":"paused"`, 1) + "," + links, false},
		{"task input continuation", "task-1", "input", "first-cursor", `"task_id":"task-1","filter":"input","next_cursor":"dGVzdC1uZXh0",` + inputItems + "," + links, true},
		{"project input empty batch", "", "input", "first-cursor", `"filter":"input","items":[],"next_cursor":"dGVzdC1uZXh0"`, true},
		{"input response with stale observation", "", "input", "", `"filter":"input",` + strings.Replace(inputItems, `"observation_failed":false`, `"observation_failed":true`, 1) + "," + links, false},
		{"input response echoed attention", "", "input", "", `"filter":"attention",` + inputItems + "," + links, false},
		{"input response with rebound task", "", "input", "", `"filter":"input",` + inputItems + "," + strings.Replace(links, `"run_relation":"current"`, `"run_relation":"changed"`, 1), false},
		{"input response with running provider", "", "input", "", `"filter":"input",` + strings.Replace(inputItems, `"provider_status":"waiting_input"`, `"provider_status":"running"`, 1) + "," + links, false},
		{"missing task echo", "task-1", "", "", items + "," + links, false},
		{"unrequested task echo", "", "", "", `"task_id":"task-1",` + items + "," + links, false},
		{"explicit empty task echo", "", "", "", `"task_id":"","items":[]`, false},
		{"foreign echo", "task-1", "", "", `"task_id":"secret-marker","items":[]`, false},
		{"wrong formal link", "task-1", "", "", `"task_id":"task-1",` + items + "," + strings.ReplaceAll(links, "task-1", "secret-marker"), false},
		{"missing formal link", "task-1", "", "", `"task_id":"task-1",` + items, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := `{"project_id":"project-one",` + tc.fields + "}"
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workbuddy-cloud-progress" ||
					q.Get("project_id") != "project-one" || q.Get("limit") != "10" ||
					q.Get("task_id") != tc.task || q.Has("task_id") != (tc.task != "") || len(q["task_id"]) > 1 ||
					q.Get("filter") != tc.filter || q.Get("before") != tc.before {
					t.Error("request did not keep its project, task, filter and cursor", r.Method, r.URL)
				}
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()

			args := []string{"run", "workbuddy-list"}
			if tc.task != "" {
				args = append(args, "--task", tc.task)
			}
			if tc.filter != "" {
				args = append(args, "--filter", tc.filter)
			}
			if tc.before != "" {
				args = append(args, "--before", tc.before)
			}
			code, stdout, stderr := invoke(t, server.URL, args...)
			if calls.Load() != 1 {
				t.Fatalf("sent %d requests, want 1", calls.Load())
			}
			if !tc.valid {
				if code != 1 || stdout != "" || !strings.Contains(stderr, `"code": "workbuddy_progress_invalid"`) || strings.Contains(stderr, "secret-marker") {
					t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
				return
			}
			var got, want any
			if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &got) != nil ||
				json.Unmarshal([]byte(response), &want) != nil || !reflect.DeepEqual(got, want) {
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
		{"--filter", "Delivery"}, {"--filter", "delivery", "--filter", "attention"},
		{"--filter", "Input"}, {"--filter", "input", "--filter", "input"},
		{"--task", ""}, {"--task", ".."}, {"--task", "task/other"}, {"--task", "task 1"},
		{"--task", strings.Repeat("a", 65)}, {"--task", "one", "--task", "two"},
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
			if code != 0 || stderr != "" || calls.Load() != 0 || !strings.Contains(stdout, "run workbuddy-list [--limit 1..20] [--filter attention|delivery|input] [--task TASK] [--before NEXT_CURSOR]") {
				t.Fatalf("help %s %v exit=%d stdout=%q stderr=%q", locale, args, code, stdout, stderr)
			}
		}
	}
}
