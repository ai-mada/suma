package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunWatchStreamDeliversOnlyCompleteScopedEvents(t *testing.T) {
	user, organization := strings.Repeat("a", 32), strings.Repeat("b", 32)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/runs/run-1/events/stream" ||
			len(q) != 2 || q.Get("project_id") != "project-one" || q.Get("after") != "7" ||
			r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Accept-Language") != "en-US" ||
			r.Header.Get("Authorization") != "Bearer stream-test-token" ||
			r.Header.Get("X-Suma-Expected-User") != user || r.Header.Get("X-Suma-Expected-Organization") != organization {
			t.Error("stream did not retain the exact request scope, credential and view")
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		partial := runWatchTestFrame("15", runWatchTestEvent("run-1", "15", "{}"))
		for _, chunk := range []string{
			"\xef", "\xbb\xbf: connected\r", "\n\r\n: heartbeat\r\n\r\n",
			"id: 11\r\nevent: run_event\r\n",
			"data: {\"sequence\":11,\"run_id\":\"run-1\",\"node_id\":\"\",",
			"\r\ndata: \"type\":\"run_created\",\"at\":\"2026-11-06T00:00:00Z\",\"data\":{\"name\":\"\xe4\xb8",
			"\xad文\"}}\r\n\r", "\n",
			partial[:len(partial)-1],
		} {
			_, _ = io.WriteString(w, chunk)
			w.(http.Flusher).Flush()
		}
	}))
	defer host.Close()
	c, err := newClient(host.URL, "project-one", "stream-test-token", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.setOriginalView(user, organization); err != nil {
		t.Fatal(err)
	}
	c.locale = "en-US"
	var sequences []int64
	var event struct{ Data map[string]string }
	active, err := c.readRunEventStream(context.Background(), "run-1", 7, func(raw json.RawMessage, seq int64) error {
		sequences = append(sequences, seq)
		return json.Unmarshal(raw, &event)
	})
	if err != nil || !active || len(sequences) != 1 || sequences[0] != 11 || event.Data["name"] != "中文" {
		t.Fatalf("unexpected stream result: activity=%t, sequences=%v, data=%v, error=%v", active, sequences, event.Data, err)
	}
}

func TestRunWatchStreamRejectsInvalidFramesAndPreservesFailures(t *testing.T) {
	outputFailure := errors.New("test output closed")
	for _, tc := range []struct {
		name, body, contentType, code string
		status, wantStatus            int
		active, callbackFailure       bool
	}{
		{name: "connected-only", body: ": connected\n\n"},
		{name: "incomplete-heartbeat", body: ": heartbeat\n"},
		{name: "complete-heartbeat", body: ": heartbeat\n\n", active: true},
		{name: "no-forward-event", body: runWatchTestFrame("7", runWatchTestEvent("run-1", "7", "{}"))},
		{name: "cross-run", body: runWatchTestFrame("9", runWatchTestEvent("another", "9", "{}")), code: "run_watch_protocol"},
		{name: "mismatched-sequence", body: runWatchTestFrame("9", runWatchTestEvent("run-1", "10", "{}")), code: "run_watch_protocol"},
		{name: "out-of-range", body: runWatchTestFrame("9007199254740992", runWatchTestEvent("run-1", "9007199254740992", "{}")), code: "run_watch_protocol"},
		{name: "non-object-metadata", body: runWatchTestFrame("9", runWatchTestEvent("run-1", "9", "null")), code: "run_watch_protocol"},
		{name: "invalid-utf8", body: "event: run_event\ndata: \xff\n\n", code: "run_watch_protocol"},
		{name: "unknown-data-event", body: "event: future_persisted_event\ndata: {}\n\n", code: "run_watch_protocol"},
		{name: "unexpected-content", contentType: "application/json", body: "{}", code: "run_watch_protocol"},
		{name: "http-denied", status: 403, body: "{\"error\":{\"code\":\"forbidden\",\"message\":\"current access denied\",\"localized\":false}}", code: "forbidden", wantStatus: 403},
		{name: "stream-denied", body: "event: stream_error\ndata: {\"status\":403,\"code\":\"forbidden\"}\n\n", code: "forbidden", wantStatus: 403},
		{name: "callback-failure", body: runWatchTestFrame("9", runWatchTestEvent("run-1", "9", "{}")), callbackFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				contentType := tc.contentType
				if contentType == "" {
					contentType = "text/event-stream"
				}
				w.Header().Set("Content-Type", contentType)
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer host.Close()
			c, err := newClient(host.URL, "project-one", "", 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			active, err := c.readRunEventStream(context.Background(), "run-1", 7, func(_ json.RawMessage, seq int64) error {
				if tc.callbackFailure {
					return outputFailure
				}
				if tc.name != "no-forward-event" || seq != 7 {
					t.Fatalf("unexpected event %d", seq)
				}
				return nil
			})
			if active != tc.active {
				t.Fatalf("activity=%t, want %t", active, tc.active)
			}
			if tc.callbackFailure {
				if err != outputFailure {
					t.Fatalf("callback error changed: %v", err)
				}
				return
			}
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var failure *failure
			if !errors.As(err, &failure) || failure.Code != tc.code || failure.Status != tc.wantStatus {
				t.Fatalf("unexpected failure: %#v (%v)", failure, err)
			}
			if tc.name == "http-denied" && (failure.Message != "current access denied" || failure.Localized == nil || *failure.Localized || !failure.remote) {
				t.Fatalf("HTTP diagnostic changed: %#v", failure)
			}
		})
	}
}

func TestRunWatchStreamRejectsOversizedFrame(t *testing.T) {
	// Use individually short comments to exercise the aggregate frame bound.
	body := strings.NewReader(strings.Repeat(": "+strings.Repeat("x", 1021)+"\n", (16*1024*1024)/1024+1))
	active, err := readRunWatchFrames(context.Background(), body, "run-1", 0, func(json.RawMessage, int64) error {
		t.Fatal("oversized frame was delivered")
		return nil
	})
	var protocol *failure
	if active || !errors.As(err, &protocol) || protocol.Code != "run_watch_protocol" || protocol.Message != "live run event frame exceeds 16 MiB" {
		t.Fatalf("oversized frame: activity=%t, error=%v", active, err)
	}
}

func runWatchTestFrame(id, payload string) string {
	return "id: " + id + "\nevent: run_event\ndata: " + payload + "\n\n"
}

func runWatchTestEvent(run, sequence, data string) string {
	return "{\"sequence\":" + sequence + ",\"run_id\":" + strconv.Quote(run) +
		",\"node_id\":\"\",\"type\":\"run_created\",\"at\":\"2026-11-06T00:00:00Z\",\"data\":" + data + "}"
}
