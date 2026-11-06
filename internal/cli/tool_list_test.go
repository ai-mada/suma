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

	"github.com/ai-mada/suma/internal/platform"
)

func TestToolProfileListCLIReadFiltersScopeAndMachineShape(t *testing.T) {
	profile := toolDoctorReport181(t).Tool
	profile.ID, profile.Name = "profile193", "研发工具 SDK %_ &"
	var calls atomic.Int32
	query, cursor := "研发 CLI%_+", "opaque+/=cursor193"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-profiles" || len(body) != 0 || r.Header.Get("Authorization") != "Bearer session193" || r.Header.Get("Accept-Language") != "en-US" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
			t.Error("read changed original caller or became a write")
		}
		q := r.URL.Query()
		if q.Get("project_id") != "project-one" || q.Get("state") != "active" || q.Get("limit") != "2" || q.Get("cursor") != cursor || len(q) != 5 {
			t.Error("scoped filters or opaque cursor changed", q)
		}
		item := profile
		if q.Get("query") == "cross-project" {
			item.ProjectID = "other-project"
		} else if q.Get("query") != query {
			t.Error("query text was rewritten", q.Get("query"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(platform.ToolProfileList{Items: []platform.ToolProfile{item}, Total: 7, NextCursor: "next193"})
	}))
	defer server.Close()
	run := func(text string) (int, string, string) {
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", server.URL, "--project", "project-one", "--token", "session193", "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "--language", "en-US", "tools", "list", "--query", text, "--limit", "2", "--cursor", cursor}, nil, &out, &stderr)
		return code, out.String(), stderr.String()
	}
	code, out, stderr := run(query)
	if code != 0 {
		t.Fatal("valid current tool list rejected", code, out, stderr)
	}
	var got platform.ToolProfileList
	if json.Unmarshal([]byte(out), &got) != nil || !reflect.DeepEqual(got, platform.ToolProfileList{Items: []platform.ToolProfile{profile}, Total: 7, NextCursor: "next193"}) {
		t.Fatal("machine response was reshaped, inferred or truncated", out)
	}
	if code, out, _ := run("cross-project"); code != 1 || out != "" {
		t.Fatal("wrong-project result was published", code, out)
	}
	if calls.Load() != 2 {
		t.Fatal("list fetched a new identity or followed another action", calls.Load())
	}
}

func TestToolProfileListCLIRejectsInvalidFlagsAndPreservesServerDenial(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-profiles" || r.URL.Query().Get("project_id") != "project-one" {
			t.Error("list left the original read path")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("query") == "forbidden" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"forbidden","message":"server-owned refusal","details":{"next_action":"enable"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"items":[],"total":0}`)
	}))
	defer server.Close()
	run := func(args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		base := []string{"--control", server.URL, "--project", "project-one", "--token", "session193", "--language", "en-US", "tools", "list"}
		code := Execute(context.Background(), append(base, args...), nil, &out, &stderr)
		return code, out.String(), stderr.String()
	}
	for _, args := range [][]string{{"--state", "ready"}, {"--state", ""}, {"--limit", "0"}, {"--limit", "201"}, {"--query", strings.Repeat("码", 41)}, {"--query", string([]byte{255})}, {"--query", "line\nbreak"}, {"--worker", ""}, {"--worker", "node/a"}, {"--worker", " node-a"}, {"--worker", strings.Repeat("n", 101)}, {"--cursor", strings.Repeat("x", 1025)}, {"--cursor", "bad\ncursor"}, {"--confirm"}, {"--operation", "not-a-read"}, {"unexpected"}} {
		if code, out, _ := run(args...); code != 2 || out != "" {
			t.Fatal("invalid or write-like list options accepted", args, code, out)
		}
	}
	for _, language := range []string{"zh-CN", "en-US"} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), []string{"--control", server.URL, "--language", language, "tools", "list", "--help"}, nil, &out, &stderr); code != 0 || !strings.Contains(out.String(), "--state") || !strings.Contains(out.String(), "120") || !strings.Contains(out.String(), "--cursor") || !strings.Contains(strings.SplitN(out.String(), "tools catalog", 2)[0], "--worker") {
			t.Fatal("bilingual list help unavailable without a project", code, out.String(), stderr.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid options or help reached the API", calls.Load())
	}
	if code, _, stderr := run("--query", strings.Repeat("码", 40), "--state", "all"); code != 0 {
		t.Fatal("valid UTF-8 byte boundary rejected", code, stderr)
	}
	code, out, stderr := run("--query", "forbidden", "--state", "revoked")
	if code != 1 || out != "" || !strings.Contains(stderr, "server-owned refusal") || !strings.Contains(stderr, `"status": 403`) || !strings.Contains(stderr, `"next_action": "enable"`) {
		t.Fatal("server denial or its diagnostics changed", code, out, stderr)
	}
	if calls.Load() != 2 {
		t.Fatal("denial triggered identity refresh, retry or repair", calls.Load())
	}
}

func TestToolProfileListCLIExactWorkerComposesWithOtherFilters(t *testing.T) {
	const worker, cursor = "Team-Runner_1.0", "worker-page+/="
	profile := toolDoctorReport181(t).Tool
	profile.WorkerID, profile.Name, profile.State = worker, "Team development CLI", "revoked"
	for _, test := range []struct{ name, returnedWorker string }{
		{"exact worker", worker},
		{"case variant", "team-runner_1.0"},
		{"same prefix", "Team-Runner_1.0-cli"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := profile
			item.WorkerID = test.returnedWorker
			page := platform.ToolProfileList{Items: []platform.ToolProfile{item}, Total: 3, NextCursor: "next-worker-page"}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-profiles" || len(q) != 6 || q.Get("project_id") != "project-one" || q.Get("worker_id") != worker || q.Get("state") != "revoked" || q.Get("query") != "Team development" || q.Get("limit") != "1" || q.Get("cursor") != cursor {
					t.Error("worker filter changed the current read or another filter", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(page)
			}))
			defer server.Close()
			var out, stderr bytes.Buffer
			code := Execute(context.Background(), []string{"--control", server.URL, "--project", "project-one", "--token", "session193", "--language", "en-US", "tools", "list", "--worker", worker, "--state", "revoked", "--query", "Team development", "--limit", "1", "--cursor", cursor}, nil, &out, &stderr)
			if test.returnedWorker == worker {
				var got platform.ToolProfileList
				if code != 0 || json.Unmarshal(out.Bytes(), &got) != nil || !reflect.DeepEqual(got, page) {
					t.Fatal("valid filtered page rejected or reshaped", code, out.String(), stderr.String())
				}
			} else if code != 1 || out.Len() != 0 {
				t.Fatal("wrong-worker record returned as output", code, out.String(), stderr.String())
			}
			if calls.Load() != 1 {
				t.Fatal("expected one GET", calls.Load())
			}
		})
	}
}
