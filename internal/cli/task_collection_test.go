package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTaskCollectionCLIProjectFiltersAndSinglePage(t *testing.T) {
	for _, action := range []string{"list", "groups"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				wantPath := "/api/v1/task-collection/summary"
				if action == "list" {
					wantPath = "/api/v1/task-workspace"
				}
				if r.Method != http.MethodGet || r.URL.Path != wantPath || r.Header.Get("Authorization") != "Bearer current-project-token" || r.Header.Get("Accept-Language") != "en-US" {
					t.Error("read transport changed", r.Method, r.URL, r.Header.Get("Accept-Language"))
				}
				if q.Get("project_id") != "project-one" || q.Get("q") != " 字面 %_! 中文 " || q.Get("owner_id") != "owner:one" || q.Get("priority") != "custom priority" || q.Get("requirement_id") != "req%one" || q.Has("unassigned") {
					t.Error("original project and literal filters changed", q)
				}
				if action == "groups" {
					if q.Get("status") != "waiting_input" || q.Get("group_by") != "owner" || q.Get("group_limit") != "100" || q.Get("group_cursor") != "original_group_cursor-1" || q.Has("sort") || q.Has("limit") || q.Has("cursor") {
						t.Error("summary received list-only parameters", q)
					}
					_, _ = w.Write([]byte(`{"project_id":"project-one","total":71,"group_by":"owner","groups":[{"key":"owner:one","total":71}],"groups_total":300,"groups_limit":100,"next_group_cursor":"continue-groups-explicitly","as_of":"2026-10-29T00:00:00Z"}`))
					return
				}
				if q.Get("status") != "waiting_input" || q.Get("limit") != "100" || q.Get("sort") != "title_asc" || q.Get("cursor") != "original_cursor-1" || q.Has("group_by") || q.Has("group_limit") || q.Has("group_cursor") {
					t.Error("original list scope changed", q)
				}
				_, _ = w.Write([]byte(`{"project_id":"project-one","items":[],"total":71,"limit":100,"next_cursor":"continue-explicitly","as_of":"2026-10-29T00:00:00Z"}`))
			}))
			defer server.Close()
			args := []string{"--language", "en-US", "--control", server.URL, "--project", "project-one", "--token", "current-project-token", "tasks", action, "--q", " 字面 %_! 中文 ", "--owner", "owner:one", "--priority", "custom priority", "--requirement", "req%one", "--status", "waiting_input"}
			if action == "list" {
				args = append(args, "--sort", "title_asc", "--limit", "100", "--cursor", "original_cursor-1")
			} else {
				args = append(args, "--group-by", "owner", "--group-limit", "100", "--group-cursor", "original_group_cursor-1")
			}
			var out, stderr strings.Builder
			if code := Execute(context.Background(), args, strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
				t.Fatal("explicit scoped read failed or followed the cursor", code, calls, stderr.String())
			}
			var value map[string]json.RawMessage
			if json.Unmarshal([]byte(out.String()), &value) != nil || string(value["total"]) != "71" {
				t.Fatal("machine response changed", out.String())
			}
			if action == "groups" && (string(value["groups_total"]) != "300" || string(value["groups_limit"]) != "100" || string(value["next_group_cursor"]) != `"continue-groups-explicitly"`) {
				t.Fatal("bounded group metadata changed", out.String())
			}
		})
	}
}

func TestTaskCollectionCLIUnassignedDefaultAndCurrentStatus(t *testing.T) {
	for _, status := range []string{"", "all", "archived"} {
		t.Run("status_"+status, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				if r.URL.Path != "/api/v1/task-workspace" || q.Get("project_id") != "p" || q.Get("unassigned") != "true" || q.Get("limit") != "20" || q.Get("sort") != "updated_desc" || q.Has("owner_id") || q.Has("cursor") || q.Has("status") != (status != "") || q.Get("status") != status {
					t.Error("default page, unassigned scope or literal status changed", q)
				}
				_, _ = w.Write([]byte(`{"items":[],"next_cursor":"more","total":1,"limit":20}`))
			}))
			defer server.Close()
			var out, stderr strings.Builder
			args := []string{"--control", server.URL, "--project", "p", "tasks", "list", "--unassigned"}
			if status != "" {
				args = append(args, "--status", status)
			}
			if code := Execute(context.Background(), args, strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
				t.Fatal(code, calls, stderr.String())
			}
		})
	}
}

func TestTaskCollectionCLIUsesCanonicalServerSorts(t *testing.T) {
	for _, sort := range []string{"updated_desc", "created_desc", "title_asc", "priority_desc"} {
		t.Run(sort, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/v1/task-workspace" || r.URL.Query().Get("sort") != sort || r.URL.Query().Get("project_id") != "p" {
					t.Error("canonical server sort changed", r.URL)
				}
				_, _ = w.Write([]byte(`{"items":[{"id":"second"},{"id":"first"}],"total":2,"next_cursor":"original"}`))
			}))
			defer server.Close()
			var out, stderr strings.Builder
			if code := Execute(context.Background(), []string{"--control", server.URL, "--project", "p", "tasks", "list", "--sort", sort}, strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
				t.Fatal(code, calls, stderr.String())
			}
			var page struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if json.Unmarshal([]byte(out.String()), &page) != nil || len(page.Items) != 2 || page.Items[0].ID != "second" || page.Items[1].ID != "first" {
				t.Fatal("client changed server page ordering", out.String())
			}
		})
	}
}

func TestTaskCollectionCLIEmptyStatusFilterOnRowsAndGroups(t *testing.T) {
	for _, action := range []string{"list", "groups"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				if q.Get("project_id") != "p" || q.Get("status_unset") != "true" || q.Has("status") {
					t.Error("empty status filter changed", q)
				}
				if action == "groups" && (q.Get("group_by") != "status" || q.Get("group_limit") != "20" || q.Has("limit")) {
					t.Error("default bounded status group changed", q)
				}
				_, _ = w.Write([]byte(`{"total":3,"items":[],"groups":[{"key":"","total":3}]}`))
			}))
			defer server.Close()
			var out, stderr strings.Builder
			if code := Execute(context.Background(), []string{"--control", server.URL, "--project", "p", "tasks", action, "--status-unset"}, strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
				t.Fatal(code, calls, stderr.String())
			}
		})
	}
}

func TestTaskCollectionCLIGroupKeysDistinguishEmptyFromLiteralIDs(t *testing.T) {
	for _, tc := range []struct {
		name, group, flag, value, wire, literal string
	}{
		{"literal_owner", "owner", "--owner", "__unassigned__", "owner_id", "__unassigned__"},
		{"empty_owner", "owner", "--unassigned", "", "unassigned", "__unassigned__"},
		{"literal_priority", "priority", "--priority", "__unset__", "priority", "__unset__"},
		{"empty_priority", "priority", "--priority-unset", "", "priority_unset", "__unset__"},
		{"literal_status", "status", "--status", "all", "status", "all"},
		{"empty_status", "status", "--status-unset", "", "status_unset", "all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				wantValue := tc.value
				if wantValue == "" {
					wantValue = "true"
				}
				if r.URL.Path != "/api/v1/task-collection/summary" || q.Get("project_id") != "p" || q.Get("group_by") != tc.group || q.Get("group_limit") != "20" || q.Has("group_cursor") || tc.group != "status" && q.Get("status") != "archived" || q.Get(tc.wire) != wantValue || q.Has("limit") || q.Has("sort") || q.Has("cursor") {
					t.Error("group filter or literal value changed", q)
				}
				if tc.group == "owner" && q.Has("owner_id") == q.Has("unassigned") || tc.group == "priority" && q.Has("priority") == q.Has("priority_unset") || tc.group == "status" && q.Has("status") == q.Has("status_unset") {
					t.Error("empty group and literal filter were conflated", q)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"group_by": tc.group, "groups": []map[string]any{{"key": "", "total": 1}, {"key": tc.literal, "total": 2}}, "total": 3})
			}))
			defer server.Close()
			args := []string{"--control", server.URL, "--project", "p", "tasks", "groups", "--group-by", tc.group}
			if tc.group != "status" {
				args = append(args, "--status", "archived")
			}
			args = append(args, tc.flag)
			if tc.value != "" {
				args = append(args, tc.value)
			}
			var out, stderr strings.Builder
			if code := Execute(context.Background(), args, strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
				t.Fatal(code, calls, stderr.String())
			}
			var summary struct {
				Groups []struct {
					Key string `json:"key"`
				} `json:"groups"`
			}
			if json.Unmarshal([]byte(out.String()), &summary) != nil || len(summary.Groups) != 2 || summary.Groups[0].Key != "" || summary.Groups[1].Key != tc.literal {
				t.Fatal("machine group keys changed", out.String())
			}
		})
	}
}

func TestTaskCollectionCLIInvalidOptionsNeverReachTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	for _, args := range [][]string{
		{"groups", "--sort", "updated_desc"}, {"groups", "--limit", "20"}, {"groups", "--cursor", "a"},
		{"groups", "--group-by", "unknown"}, {"groups", "--group-by", ""}, {"list", "--group-by", "status"}, {"list", "--sort", "unknown"},
		{"groups", "--group-limit", "0"}, {"groups", "--group-limit", "101"}, {"groups", "--group-limit", "01"},
		{"groups", "--group-cursor", ""}, {"groups", "--group-cursor", strings.Repeat("x", 4097)}, {"groups", "--group-cursor", "bad\ncursor"},
		{"list", "--group-limit", "20"}, {"list", "--group-cursor", "original"},
		{"list", "--owner", "one", "--unassigned"}, {"list", "--owner", "bad/id"},
		{"list", "--requirement", strings.Repeat("x", 65)}, {"list", "--owner", "owner one"},
		{"list", "--q", "bad\x00query"}, {"list", "--q", strings.Repeat("中", 201)}, {"list", "--q", string([]byte{0xff})},
		{"list", "--q", "unicode\u0085control"}, {"list", "--status", "all", "--status-unset"}, {"groups", "--status", "archived", "--status-unset"},
		{"list", "--status", ""}, {"list", "--status", " "}, {"list", "--priority", "bad\npriority"}, {"list", "--priority", "high", "--priority-unset"},
		{"list", "--limit", "0"}, {"list", "--limit", "101"}, {"list", "--limit", "01"}, {"list", "--limit", "1e1"},
		{"list", "--cursor", strings.Repeat("x", 4097)}, {"list", "--cursor", "bad\ncursor"},
		{"list", "--confirm"}, {"list", "--operation", "write-key"}, {"list", "unexpected"},
	} {
		var out, stderr strings.Builder
		command := append([]string{"--language", "en-US", "--control", server.URL, "--project", "p", "tasks"}, args...)
		if code := Execute(context.Background(), command, strings.NewReader(""), &out, &stderr); code != 2 || out.Len() != 0 {
			t.Error("invalid command was accepted", args, code, out.String(), stderr.String())
		}
	}
	var out, stderr strings.Builder
	if code := Execute(context.Background(), []string{"--control", server.URL, "tasks", "list"}, strings.NewReader(""), &out, &stderr); code != 2 {
		t.Error("missing project was accepted", code, stderr.String())
	}
	if calls != 0 {
		t.Fatal("invalid read reached transport", calls)
	}
}

func TestTaskCollectionCLIAuthorityFailurePassesThroughWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"当前授权已收回","message_key":"task_collection.denied","locale":"zh-CN","localized":false,"details":{"scope":"original"}}}`))
	}))
	defer server.Close()
	var out, stderr strings.Builder
	code := Execute(context.Background(), []string{"--language", "en-US", "--control", server.URL, "--project", "p", "tasks", "groups"}, strings.NewReader(""), &out, &stderr)
	var envelope struct {
		Error failure `json:"error"`
	}
	if code != 1 || calls != 1 || out.Len() != 0 || json.Unmarshal([]byte(stderr.String()), &envelope) != nil || envelope.Error.Status != 403 || envelope.Error.Code != "forbidden" || envelope.Error.Message != "当前授权已收回" || envelope.Error.Localized == nil || *envelope.Error.Localized {
		t.Fatal("current authorization failure changed", code, calls, stderr.String())
	}
}

func TestTaskCollectionCLIHelpAndUsageLanguages(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		var out, stderr strings.Builder
		if code := Execute(context.Background(), []string{"--language", locale, "tasks", "list", "--help"}, strings.NewReader(""), &out, &stderr); code != 0 || !strings.Contains(out.String(), "tasks groups") || !strings.Contains(out.String(), "--cursor") {
			t.Fatal("scoped command help missing", locale, code, out.String(), stderr.String())
		}
		out.Reset()
		stderr.Reset()
		code := Execute(context.Background(), []string{"--language", locale, "tasks", "list", "--limit", "101"}, strings.NewReader(""), &out, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "cli.task_collection.usage") || locale == "zh-CN" && !strings.Contains(stderr.String(), "任务集合") || locale == "en-US" && !strings.Contains(stderr.String(), "tasks list") {
			t.Fatal("local usage language changed", locale, code, stderr.String())
		}
	}
}
