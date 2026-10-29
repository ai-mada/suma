package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSharedTaskViewsCLIPreservesWriteIdentityAndVersion(t *testing.T) {
	for _, action := range []string{"create", "update", "archive"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				if decoder.Decode(&body) != nil {
					t.Fatal("body")
				}
				if r.URL.RawQuery != "" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if body["project_id"] != "project-one" || body["idempotency_key"] != "original-operation" {
					t.Error("identity changed", body, r.URL)
				}
				method, path := http.MethodPost, "/api/v1/task-views"
				if action != "create" {
					path += "/view-one"
					if body["expected_version"] != json.Number("7") {
						t.Error("version changed")
					}
				}
				if action == "update" {
					method = http.MethodPatch
				}
				if action == "archive" {
					path += "/archive"
				}
				if r.Method != method || r.URL.Path != path {
					t.Error("wrong route", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":"view-one","version":8}`))
			}))
			defer server.Close()
			c, _ := newClient(server.URL, "project-one", "", time.Second)
			args := []string{action}
			body := `{"name":"My view","visibility":"private","preferences":{"owner_id":"__unassigned__"},"idempotency_key":"original-operation"}`
			if action != "create" {
				args = append(args, "view-one")
				body = `{"name":"My view","visibility":"private","expected_version":7,"idempotency_key":"original-operation"}`
			}
			if action == "archive" {
				body = `{"expected_version":7,"idempotency_key":"original-operation"}`
			}
			args = append(args, "--file", "-")
			var out strings.Builder
			if err := sharedTaskViewsCommand(context.Background(), c, args, strings.NewReader(body), &out); err != nil || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
}

func TestSharedTaskViewsCLIRejectsAmbiguousOrUnscopedWritesBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	c, _ := newClient(server.URL, "project-one", "", time.Second)
	for _, body := range []string{
		`{"name":"v","visibility":"private"}`,
		`{"name":"v","visibility":"private","idempotency_key":"operation-one","expected_version":1}`,
		`{"name":"v","visibility":"private","idempotency_key":"operation-one","owner_id":"other"}`,
		`{"name":"v","visibility":"private","idempotency_key":"operation-one","preferences":{"q":"a","q":"b"}}`,
		`{"name":"v","visibility":"private","idempotency_key":"operation-one","preferences":null}`,
		`{"name":"v","visibility":"private","idempotency_key":"operation-one","project_id":"other"}`,
	} {
		if err := sharedTaskViewsCommand(context.Background(), c, []string{"create", "--file", "-"}, strings.NewReader(body), &strings.Builder{}); err == nil {
			t.Fatal("accepted invalid body", body)
		}
	}
	for _, version := range []string{"0", "-1", "1.0", "1e1", "null", "\"7\""} {
		body := `{"expected_version":` + version + `,"idempotency_key":"operation-one"}`
		if err := sharedTaskViewsCommand(context.Background(), c, []string{"archive", "view-one", "--file", "-"}, strings.NewReader(body), &strings.Builder{}); err == nil {
			t.Fatal("accepted invalid version", version)
		}
	}
	c.project = ""
	if err := sharedTaskViewsCommand(context.Background(), c, []string{"list"}, nil, &strings.Builder{}); err == nil {
		t.Fatal("missing project accepted")
	}
	if calls != 0 {
		t.Fatal("invalid write reached network", calls)
	}
}

func TestSharedTaskViewsCLIReadsOneBoundedPage(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v1/task-views" || r.URL.Query().Get("limit") != "17" || r.URL.Query().Get("cursor") != "original+cursor" || r.URL.Query().Get("project_id") != "project-one" {
			t.Error(r.URL)
		}
		w.Write([]byte(`{"items":[],"total":45,"next_cursor":"next"}`))
	}))
	defer server.Close()
	c, _ := newClient(server.URL, "project-one", "", time.Second)
	var out strings.Builder
	if err := sharedTaskViewsCommand(context.Background(), c, []string{"list", "--limit", "17", "--cursor", "original+cursor"}, nil, &out); err != nil || calls != 1 || !strings.Contains(out.String(), `"total": 45`) {
		t.Fatal(err, calls, out.String())
	}
}
