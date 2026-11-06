package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func runWatchCommandFrame(sequence int, run, data string) string {
	return fmt.Sprintf("id: %d\nevent: run_event\ndata: {\"sequence\":%d,\"run_id\":%q,\"node_id\":\"test\",\"type\":\"node_progress\",\"at\":\"2026-11-06T00:00:00Z\",\"data\":%s}\n\n", sequence, sequence, run, data)
}

func TestRunWatchCommandResumesAndPrintsNDJSON(t *testing.T) {
	var calls atomic.Int32
	user := strings.Repeat("a", 32)
	org := strings.Repeat("b", 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/runs/run-1/events/stream" || r.URL.Query().Get("project_id") != "project-one" ||
			r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Authorization") != "Bearer test-secret" ||
			r.Header.Get("X-Suma-Expected-User") != user || r.Header.Get("X-Suma-Expected-Organization") != org ||
			r.Header.Get("Accept-Language") != "en-US" {
			t.Error("run stream lost the exact request scope")
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch calls.Add(1) {
		case 1:
			if r.URL.Query().Get("after") != "7" {
				t.Error("did not use the initial cursor")
			}
			fmt.Fprint(w, runWatchCommandFrame(9, "run-1", `{"exact_number":9007199254740993,"value":"test-secret"}`))
			fmt.Fprint(w, strings.TrimSuffix(runWatchCommandFrame(11, "run-1", `{}`), "\n"))
		case 2:
			if r.URL.Query().Get("after") != "9" {
				t.Error("did not resume from the last complete printed event")
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":{"code":"stream_unavailable","message":"Stream unavailable"}}`)
		case 3:
			if r.URL.Query().Get("after") != "9" {
				t.Error("transient failure changed the saved cursor")
			}
			fmt.Fprint(w, runWatchCommandFrame(9, "run-1", `{}`))
			fmt.Fprint(w, runWatchCommandFrame(12, "run-1", `{}`))
			fmt.Fprint(w, runWatchCommandFrame(13, "run-1", `{}`))
		default:
			t.Error("unexpected reconnect after the output limit")
		}
	}))
	defer server.Close()

	code, stdout, stderr := invoke(t, server.URL, "--language", "en-US", "--expected-user", user, "--expected-organization", org,
		"run", "watch", "run-1", "--after", "7", "--max-events", "2")
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if code != 0 || stderr != "" || calls.Load() != 3 || len(lines) != 2 ||
		!strings.Contains(lines[0], `"sequence":9`) || !strings.Contains(lines[1], `"sequence":12`) ||
		!strings.Contains(lines[0], `"exact_number":9007199254740993`) || !strings.Contains(lines[0], "[redacted]") ||
		strings.Contains(stdout, "test-secret") {
		t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout, stderr)
	}
}

func TestRunWatchCommandStopsOnProtocolOrAuthorityFailure(t *testing.T) {
	for _, tc := range []struct {
		name, stream, code string
	}{
		{"wrong run", runWatchCommandFrame(3, "other-run", `{}`), "run_watch_protocol"},
		{"access revoked", "event: stream_error\ndata: {\"status\":403,\"code\":\"forbidden\"}\n\n", "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.stream)
			}))
			defer server.Close()
			code, stdout, stderr := invoke(t, server.URL, "run", "watch", "run-1", "--max-events", "1")
			if code != 1 || stdout != "" || calls.Load() != 1 || !strings.Contains(stderr, `"code": "`+tc.code+`"`) {
				t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout, stderr)
			}
		})
	}
}

func TestRunWatchCommandCancellationAndInvalidInput(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": connected\n\n")
		w.(http.Flusher).Flush()
		cancel()
	}))
	defer server.Close()
	for _, flags := range [][]string{
		{"--after", "9007199254740992"}, {"--after", "01"}, {"--max-events", "0"}, {"--max-events", ""},
	} {
		code, stdout, _ := invoke(t, server.URL, append([]string{"run", "watch", "run-1"}, flags...)...)
		if code != 2 || stdout != "" || calls.Load() != 0 {
			t.Fatalf("invalid flags %v sent a request or returned %d", flags, code)
		}
	}
	var stdout, stderr bytes.Buffer
	code := Execute(ctx, []string{"--control", server.URL, "--project", "project-one", "run", "watch", "run-1"}, nil, &stdout, &stderr)
	if code != 0 || calls.Load() != 1 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("cancel exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
	}
}

func TestRunWatchCommandSchedulingReads(t *testing.T) {
	const admission = `{"run_id":"run-1","node_id":"test","status":"blocked","blockers":[{"code":"tool_profile_paused","source":"tool"}],"tool_binding":{"profile_id":"tool-one","command_id":"check","profile_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("project_id") != "project-one" {
			t.Error("scheduling read has wrong method or project")
		}
		switch calls.Add(1) {
		case 1:
			if r.URL.Path != "/api/v1/scheduling/queue" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("cursor") != "opaque_cursor" {
				t.Error("queue read lost its cursor or limit")
			}
			fmt.Fprint(w, `{"items":[{"run_id":"run-1","node_id":"test","admission":`+admission+`}],"next_cursor":"another_cursor"}`)
		case 2:
			if r.URL.Path != "/api/v1/runs/run-1/nodes/test/scheduling" {
				t.Error("scheduling read is not scoped to the exact run and node")
			}
			fmt.Fprint(w, admission)
		default:
			t.Error("unexpected scheduling request")
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"run", "queue", "--limit", "2", "--cursor", "opaque_cursor"}, "another_cursor"},
		{[]string{"run", "scheduling", "run-1", "--node", "test"}, "blockers"},
	} {
		code, stdout, stderr := invoke(t, server.URL, tc.args...)
		if code != 0 || !strings.Contains(stdout, tc.want) || !strings.Contains(stdout, `"tool_profile_paused"`) ||
			!strings.Contains(stdout, `"profile_id": "tool-one"`) || stderr != "" {
			t.Fatalf("%v exit=%d stdout=%q stderr=%q", tc.args, code, stdout, stderr)
		}
	}
}

func TestRunQueueSendsExactFiltersWithOriginalCursor(t *testing.T) {
	for _, tc := range []struct {
		name, route, run, tool, cursor string
		args                           []string
	}{
		{
			name:  "pool and exact run continuation",
			args:  []string{"--route=pool", "--run", "Abc_12.v-1", "--tool", "Codex_01", "--limit", "3", "--cursor", "Original_Cursor-9"},
			route: "pool", run: "Abc_12.v-1", tool: "Codex_01", cursor: "Original_Cursor-9",
		},
		{
			name: "project route only", args: []string{"--route", "project", "--limit", "3"}, route: "project",
		},
		{
			name: "run across routes", args: []string{"--run=" + strings.Repeat("r", 100), "--limit", "3"}, run: strings.Repeat("r", 100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/scheduling/queue" ||
					q.Get("project_id") != "project-one" || q.Get("limit") != "3" || r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Errorf("incorrect queue read: %s %s", r.Method, r.URL)
				}
				for key, want := range map[string]string{"route": tc.route, "run_id": tc.run, "tool_profile_id": tc.tool, "cursor": tc.cursor} {
					if want == "" && q.Has(key) || want != "" && (len(q[key]) != 1 || q.Get(key) != want) {
						t.Errorf("queue query %q = %v, want %q", key, q[key], want)
					}
				}
				fmt.Fprint(w, "{\"items\":[],\"total\":8,\"next_cursor\":\"next-in-same-scope\"}")
			}))
			defer server.Close()
			code, stdout, stderr := invoke(t, server.URL, append([]string{"run", "queue"}, tc.args...)...)
			if code != 0 || stderr != "" || calls.Load() != 1 || !strings.Contains(stdout, "\"next-in-same-scope\"") || !strings.Contains(stdout, "\"total\": 8") {
				t.Fatalf("exit=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout, stderr)
			}
		})
	}
}

func TestRunQueueRejectsInvalidFiltersWithoutReading(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, "{\"items\":[],\"total\":0}")
	}))
	defer server.Close()
	for _, flags := range [][]string{
		{"--route"}, {"--route="}, {"--route", "all"}, {"--route", "Pool"}, {"--route", "pool "},
		{"--route", "project", "--route=pool"}, {"--run"}, {"--run="}, {"--run", "_leading"},
		{"--run", "bad/run"}, {"--run", "two words"}, {"--run", "运行"}, {"--run", strings.Repeat("r", 101)},
		{"--run", "same", "--run=same"},
		{"--tool"}, {"--tool="}, {"--tool", "bad/tool"}, {"--tool", "same", "--tool=same"}, {"--tool", strings.Repeat("t", 101)},
	} {
		code, stdout, _ := invoke(t, server.URL, append([]string{"run", "queue"}, flags...)...)
		if code != 2 || stdout != "" || calls.Load() != 0 {
			t.Fatalf("invalid filters %v sent a request or returned exit=%d stdout=%q", flags, code, stdout)
		}
	}
}

type incompleteRunWatchWriter struct{}

func (incompleteRunWatchWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestRunWatchCommandKeepsCursorOnOutputFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, runWatchCommandFrame(9, "run-1", `{}`))
	}))
	defer server.Close()
	var stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "--project", "project-one",
		"run", "watch", "run-1", "--after", "7", "--max-events", "1"}, nil, incompleteRunWatchWriter{}, &stderr)
	if code != 1 || calls.Load() != 1 || !strings.Contains(stderr.String(), `"code": "run_watch_output"`) ||
		!strings.Contains(stderr.String(), `"after": 7`) {
		t.Fatalf("exit=%d calls=%d stderr=%q", code, calls.Load(), stderr.String())
	}
}
