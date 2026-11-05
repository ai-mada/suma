package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const workBuddyProgressJSON = `{"project_id":"project-one","run_id":"run-1","node_id":"node-1","run_version":4,"run_state":"running","node_state":"waiting","node_reason":"cloud.delivery_required","attempt":1,"submission_state":"task_known","provider_status":"completed","observed_at":"2026-11-06T00:00:00Z","observation_failed":false}`

func TestWorkBuddyProgressCLIExactRead(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/runs/run-1/nodes/node-1/workbuddy-status" ||
			r.URL.RawQuery != "project_id=project-one" || r.Header.Get("Authorization") != "Bearer test-secret" ||
			r.Header.Get("Accept-Language") != "en-US" || r.ContentLength != 0 {
			t.Error("WorkBuddy progress did not use the exact read-only project route")
		}
		fmt.Fprint(w, workBuddyProgressJSON)
	}))
	defer server.Close()

	code, stdout, stderr := invoke(t, server.URL, "--language", "en-US", "run", "workbuddy-status", "run-1", "--node", "node-1")
	var got, want map[string]any
	if code != 0 || stderr != "" || calls.Load() != 1 || json.Unmarshal([]byte(stdout), &got) != nil || json.Unmarshal([]byte(workBuddyProgressJSON), &want) != nil ||
		len(got) != len(want) {
		t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout, stderr)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("field %q was changed or absent: %v", key, got[key])
		}
	}
}

func TestWorkBuddyProgressCLIRejectsUnverifiedResponse(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
		code           string
	}{
		{"other project", strings.Replace(workBuddyProgressJSON, "project-one", "secret-marker", 1), 200, "workbuddy_progress_invalid"},
		{"other run", strings.Replace(workBuddyProgressJSON, "run-1", "secret-marker", 1), 200, "workbuddy_progress_invalid"},
		{"other node", strings.Replace(workBuddyProgressJSON, "node-1", "secret-marker", 1), 200, "workbuddy_progress_invalid"},
		{"unexpected field", strings.TrimSuffix(workBuddyProgressJSON, "}") + `,"connection_id":"secret-marker"}`, 200, "workbuddy_progress_invalid"},
		{"missing attempt", strings.Replace(workBuddyProgressJSON, `,"attempt":1`, "", 1), 200, "workbuddy_progress_invalid"},
		{"missing observation flag", strings.Replace(workBuddyProgressJSON, `,"observation_failed":false`, "", 1), 200, "workbuddy_progress_invalid"},
		{"invalid observation", strings.Replace(workBuddyProgressJSON, "completed", "secret-marker", 1), 200, "workbuddy_progress_invalid"},
		{"current refusal", `{"error":{"code":"forbidden","message":"secret-marker","details":{"private":"secret-marker"}}}`, 403, "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.response)
			}))
			defer server.Close()
			code, stdout, stderr := invoke(t, server.URL, "run", "workbuddy-status", "run-1", "--node=node-1")
			if code != 1 || stdout != "" || calls.Load() != 1 || !strings.Contains(stderr, `"code": "`+tc.code+`"`) || strings.Contains(stderr, "secret-marker") {
				t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout, stderr)
			}
		})
	}
}

func TestWorkBuddyProgressCLIStrictScopeAndHelp(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, args := range [][]string{
		{}, {"run-1"}, {"run-1", "--node", "node-1", "extra"}, {"run-1", "--node", "node-1", "--node", "node-1"},
		{"other/run", "--node", "node-1"}, {"run-1", "--node", "other/node"},
	} {
		code, stdout, _ := invoke(t, server.URL, append([]string{"run", "workbuddy-status"}, args...)...)
		if code != 2 || stdout != "" || calls.Load() != 0 {
			t.Fatalf("invalid args %v returned %d, or sent a request", args, code)
		}
	}
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "--language", "en-US", "run", "workbuddy-status", "run-1", "--node", "node-1"}, nil, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || calls.Load() != 0 || !strings.Contains(stderr.String(), "--project is required") {
		t.Fatalf("missing project exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
	}
	for _, locale := range []string{"en-US", "zh-CN"} {
		for _, args := range [][]string{{"--help"}, {"run", "workbuddy-status", "--help"}} {
			code, stdout, stderr := invoke(t, server.URL, append([]string{"--language", locale}, args...)...)
			if code != 0 || stderr != "" || calls.Load() != 0 || !strings.Contains(stdout, "run workbuddy-status RUN --node NODE") {
				t.Fatalf("help %s %v exit=%d stdout=%q stderr=%q", locale, args, code, stdout, stderr)
			}
		}
	}
}
