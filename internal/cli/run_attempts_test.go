package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	runAttemptFixture     = `{"run_id":"run-one","node_id":"node-one","attempt":2,"fence":9,"worker_id":"worker-one","state":"failed","started_at":"2026-11-06T01:02:03Z","completed_at":"2026-11-06T01:03:00Z","result_source":"not_loaded"}`
	runAttemptFixturePath = "/api/v1/runs/run-one/nodes/node-one/attempts"
	runAttemptFixtureRaw  = `{ "attempt":2, "worker_id":"worker-one", "fence":9, "elapsed_ms":1e1 }`
)

func runAttemptFixtureEventPage(data, raw string) string {
	return fmt.Sprintf(`{"schema_version":1,"project_id":"project-one","run_id":"run-one","node_id":"node-one","attempt":2,"fence":9,"worker_id":"worker-one","items":[{"sequence":12,"run_id":"run-one","node_id":"node-one","type":"tool_progress","at":"2026-11-06T01:02:04Z","data":%s,"raw_data":%s}],"watermark":20,"scanned_through":12,"has_more":true,"page_cursor":"b3JpZ2luYWw","next_cursor":"bmV4dA"}`, data, raw)
}

func invokeRunAttemptRead(t *testing.T, control string, args []string) (int, string, string) {
	t.Helper()
	var out, diagnostic bytes.Buffer
	base := []string{"--control", control, "--project", "project-one", "--token", "test-secret",
		"--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "--language", "en-US"}
	code := Execute(context.Background(), append(base, args...), nil, &out, &diagnostic)
	return code, out.String(), diagnostic.String()
}

func TestRunAttemptReadsKeepExactScopeAndEvidence(t *testing.T) {
	quotedRaw, err := json.Marshal(runAttemptFixtureRaw)
	if err != nil {
		t.Fatal(err)
	}
	result := strings.Replace(runAttemptFixture, `"result_source":"not_loaded"`, `"result_source":"manual_reconciliation","reconciliation":{"outcome":"failed","reason":"verified by operator"}`, 1)
	// The engine keeps this durable raw JSON verbatim and uses Go's decoded
	// value as the separate display projection.
	const rawToolPayload = `{ "attempt":2, "worker_id":"worker-one", "fence":9, "tool_event":{"value":1,"value":9007199254740993,"name":"\ud800"} }`
	const projectedToolPayload = `{"attempt":2,"worker_id":"worker-one","fence":9,"tool_event":{"value":9007199254740993,"name":"\ufffd"}}`
	quotedToolPayload, err := json.Marshal(rawToolPayload)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, action, suffix string
		flags                []string
		query                url.Values
		body                 string
		want                 []string
	}{
		{"list", "attempts", "", []string{"--limit", "1", "--cursor", "cHJldmlvdXM"}, url.Values{"limit": {"1"}, "cursor": {"cHJldmlvdXM"}},
			`{"items":[` + runAttemptFixture + `],"total":3,"next_cursor":"bmV4dA"}`, []string{`"next_cursor": "bmV4dA"`, `"result_source": "not_loaded"`}},
		{"detail", "attempt", "/2", []string{"--attempt", "2"}, url.Values{}, result, []string{`"result_source": "manual_reconciliation"`, `"reason": "verified by operator"`}},
		{"tool-events", "tool-events", "/2/tool-events", []string{"--attempt", "2", "--limit", "1", "--cursor", "b3JpZ2luYWw"}, url.Values{"limit": {"1"}, "cursor": {"b3JpZ2luYWw"}},
			runAttemptFixtureEventPage(runAttemptFixtureRaw, string(quotedRaw)), []string{`"raw_data": ` + string(quotedRaw), `"next_cursor": "bmV4dA"`}},
		{"tool-events-original-payload", "tool-events", "/2/tool-events", []string{"--attempt", "2"}, url.Values{"limit": {"50"}},
			runAttemptFixtureEventPage(projectedToolPayload, string(quotedToolPayload)), []string{`"raw_data": ` + string(quotedToolPayload), `"value": 9007199254740993`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			query := tc.query
			query.Set("project_id", "project-one")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil || len(body) != 0 || r.Method != http.MethodGet ||
					r.URL.Path != runAttemptFixturePath+tc.suffix || !reflect.DeepEqual(r.URL.Query(), query) {
					t.Errorf("changed scoped read: %s %s body bytes=%d (%v)", r.Method, r.URL, len(body), err)
				}
				if r.Header.Get("Authorization") != "Bearer test-secret" ||
					r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) ||
					r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("original authorization view was not preserved")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			args := append([]string{"run", tc.action, "run-one", "--node", "node-one"}, tc.flags...)
			code, out, diagnostic := invokeRunAttemptRead(t, server.URL, args)
			if code != 0 || diagnostic != "" || calls.Load() != 1 {
				t.Fatalf("exit=%d reads=%d stdout=%q stderr=%q", code, calls.Load(), out, diagnostic)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("stdout lost %s: %s", want, out)
				}
			}
		})
	}
}

func TestRunAttemptReadsRejectMismatchedOrMalformedResponses(t *testing.T) {
	quotedRaw, err := json.Marshal(runAttemptFixtureRaw)
	if err != nil {
		t.Fatal(err)
	}
	history := runAttemptFixtureEventPage(runAttemptFixtureRaw, string(quotedRaw))
	const emptyHistory = `{"schema_version":1,"project_id":"project-one","run_id":"run-one","node_id":"node-one","attempt":2,"fence":9,"worker_id":"worker-one","items":null,"watermark":0,"scanned_through":0,"has_more":false,"page_cursor":"b3JpZ2luYWw"}`
	result := strings.Replace(runAttemptFixture, "not_loaded", "unreported", 1)
	cases := []struct {
		name, action, body string
	}{
		{"omitted-page-items", "attempts", `{"total":0}`},
		{"missing-page-items", "attempts", `{"items":null,"total":0}`},
		{"foreign-page-node", "attempts", `{"items":[` + strings.Replace(runAttemptFixture, "node-one", "node-other", 1) + `],"total":1}`},
		{"duplicate-attempt", "attempts", `{"items":[` + runAttemptFixture + "," + runAttemptFixture + `],"total":2}`},
		{"foreign-detail-attempt", "attempt", strings.Replace(result, `"attempt":2`, `"attempt":3`, 1)},
		{"duplicate-detail-identity", "attempt", strings.Replace(result, `"run_id":"run-one"`, `"run_id":"foreign","run_id":"run-one"`, 1)},
		{"foreign-tool-project", "tool-events", strings.Replace(history, `"project_id":"project-one"`, `"project_id":"project-other"`, 1)},
		{"missing-tool-items", "tool-events", emptyHistory},
		{"tool-event-after-scan", "tool-events", strings.Replace(history, `"sequence":12`, `"sequence":13`, 1)},
		{"tool-event-wrong-fence", "tool-events", runAttemptFixtureEventPage(strings.Replace(runAttemptFixtureRaw, `"fence":9`, `"fence":10`, 1), string(quotedRaw))},
		{"tool-event-missing-raw", "tool-events", strings.Replace(history, `,"raw_data":`+string(quotedRaw), "", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet {
					t.Error("expected only a read")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			args := []string{"run", tc.action, "run-one", "--node", "node-one"}
			if tc.action != "attempts" {
				args = append(args, "--attempt", "2")
			}
			code, out, diagnostic := invokeRunAttemptRead(t, server.URL, args)
			if code != 1 || out != "" || !strings.Contains(diagnostic, `"code": "run_attempt_response_invalid"`) || calls.Load() != 1 {
				t.Fatalf("exit=%d reads=%d stdout=%q stderr=%q", code, calls.Load(), out, diagnostic)
			}
		})
	}
}

func TestRunAttemptReadsRejectBadArgumentsAndPreserveRefusal(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != runAttemptFixturePath+"/2/tool-events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":"forbidden","message":"project access revoked"}}`)
	}))
	defer server.Close()
	for _, args := range [][]string{
		{"run", "attempts", "run-one"},
		{"run", "attempts", "run-one", "--node", "node-one", "--node", "node-two"},
		{"run", "attempts", "run-one", "--node", "node-one", "--limit", "101"},
		{"run", "attempts", "run-one", "--node", "node-one", "--cursor", "bad\ncursor"},
		{"run", "attempt", "run-one", "--node", "node-one"},
		{"run", "attempt", "run-one", "--node", "node-one", "--attempt", "02"},
		{"run", "attempt", "run-one", "--node", "node-one", "--attempt", "2", "--limit", "1"},
		{"run", "tool-events", "run-one", "--node", "node-one", "--attempt", "9007199254740992"},
	} {
		code, out, diagnostic := invokeRunAttemptRead(t, server.URL, args)
		if code != 2 || out != "" || !strings.Contains(diagnostic, `"code": "usage"`) || calls.Load() != 0 {
			t.Fatalf("%v exit=%d reads=%d stdout=%q stderr=%q", args, code, calls.Load(), out, diagnostic)
		}
	}
	code, out, diagnostic := invokeRunAttemptRead(t, server.URL, []string{"run", "tool-events", "run-one", "--node", "node-one", "--attempt", "2"})
	if code != 1 || out != "" || !strings.Contains(diagnostic, `"code": "forbidden"`) || calls.Load() != 1 {
		t.Fatalf("server refusal changed: exit=%d reads=%d stdout=%q stderr=%q", code, calls.Load(), out, diagnostic)
	}
}
