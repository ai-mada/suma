package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func discussionCLIInvoke(t *testing.T, origin, locale, input string, command ...string) (int, string, string) {
	t.Helper()
	args := append([]string{"--control", origin, "--language", locale, "--project", "project-one", "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "discussion"}, command...)
	var out, stderr strings.Builder
	code := Execute(context.Background(), args, strings.NewReader(input), &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestTaskDiscussionsCLIExactScopedReadParameters(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		args       []string
		query      map[string]string
	}{
		{"list", "/api/v1/task-conversations", []string{"list", "--task", "task-one", "--limit", "50", "--before", "9007199254740991"}, map[string]string{"task_id": "task-one", "limit": "50", "before": "9007199254740991"}},
		{"people", "/api/v1/task-conversation-mention-targets", []string{"people", "--task", "task-one", "--query", " 原名 %_! ", "--limit", "50", "--cursor", "original_cursor-1"}, map[string]string{"task_id": "task-one", "q": " 原名 %_! ", "limit": "50", "cursor": "original_cursor-1"}},
		{"message", "/api/v1/task-conversation-messages/message-one", []string{"message", "message-one", "--task", "task-one"}, map[string]string{"task_id": "task-one"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				query := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != tc.path || query.Get("project_id") != "project-one" || len(query) != len(tc.query)+1 {
					t.Error("discussion read target or scope changed", r.Method, r.URL)
				}
				for key, value := range tc.query {
					if query.Get(key) != value {
						t.Error("original read parameter changed", key, query.Get(key), value)
					}
				}
				_, _ = w.Write([]byte(`{"sequence":9007199254740991,"next_cursor":"explicit-only","body":"原文 unchanged","can_post":false}`))
			}))
			defer server.Close()
			code, out, stderr := discussionCLIInvoke(t, server.URL, "en-US", "", tc.args...)
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, "9007199254740991") || !strings.Contains(out, `"can_post": false`) || !strings.Contains(out, "原文 unchanged") {
				t.Fatal("read changed JSON or followed history automatically", code, calls, out, stderr)
			}
		})
	}
}

func TestTaskDiscussionsCLIPostKeepsOriginalPayloadAndHasNoQuery(t *testing.T) {
	const input = `{"project_id":"project-one","task_id":"task-one","expected_task_version":2,"body":"  原正文\nline two  ","idempotency_key":"original-discussion-post","mention_resource_ids":["human-z","human-a"],"knowledge":[{"id":"reviewed-source","revision":2,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/task-conversations" || r.URL.RawQuery != "" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
			t.Error("closed POST or original caller changed", r.Method, r.URL)
		}
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		var body map[string]any
		if decoder.Decode(&body) != nil || !reflect.DeepEqual(body, map[string]any{"project_id": "project-one", "task_id": "task-one", "expected_task_version": json.Number("2"), "body": "  原正文\nline two  ", "idempotency_key": "original-discussion-post", "mention_resource_ids": []any{"human-z", "human-a"}, "knowledge": []any{map[string]any{"id": "reviewed-source", "revision": json.Number("2"), "sha256": strings.Repeat("a", 64)}}}) {
			t.Error("post modified original body, version, operation or ordered resource IDs", body)
		}
		_, _ = w.Write([]byte(`{"id":"message-one","sequence":17,"body":"  原正文\nline two  ","actor_kind":"human","mentions":[{"resource_id":"human-z","user_id":"z","name_at_post":"Z原名"},{"resource_id":"human-a","user_id":"a","name_at_post":"A原名"}]}`))
	}))
	defer server.Close()
	code, out, stderr := discussionCLIInvoke(t, server.URL, "zh-CN", input, "post", "--file", "-")
	if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, "Z原名") || !strings.Contains(out, "A原名") {
		t.Fatal(code, calls, out, stderr)
	}
}

func TestTaskDiscussionsCLIUnconfirmedPostDoesNotRetryOrReplaceOperation(t *testing.T) {
	const input = `{"project_id":"project-one","task_id":"task-one","expected_task_version":2,"body":"Original recovery body","idempotency_key":"original-discussion-recovery","mention_resource_ids":["human-z","human-a"]}`
	var posted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		posted = append(posted, string(raw))
		if r.URL.RawQuery != "" {
			t.Error("POST recovery gained a project query", r.URL)
		}
		if len(posted) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = w.Write([]byte(`{"id":"original-message","body":"Original recovery body","sequence":1}`))
	}))
	defer server.Close()
	code, out, stderr := discussionCLIInvoke(t, server.URL, "en-US", input, "post", "--file", "-")
	if code != 1 || out != "" || len(posted) != 1 || !strings.Contains(stderr, "outcome may be unknown") {
		t.Fatal("unconfirmed post retried or pretended success", code, len(posted), out, stderr)
	}
	code, out, stderr = discussionCLIInvoke(t, server.URL, "en-US", input, "post", "--file", "-")
	if code != 0 || stderr != "" || len(posted) != 2 || posted[0] != posted[1] {
		t.Fatal("explicit retry changed the original frozen request", code, posted, out, stderr)
	}
}

func TestTaskDiscussionsCLIInvalidLocalInputNeverReachesTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, args := range [][]string{{"list"}, {"list", "--task", "task-one", "--limit", "51"}, {"list", "--task", "task-one", "--before", "0"}, {"list", "--task", "task-one", "--before", "9007199254740992"}, {"people", "--task", "task-one", "--query", "bad\u0085name"}, {"people", "--task", "task-one", "--cursor", strings.Repeat("x", 4097)}, {"message", "../message", "--task", "task-one"}, {"post"}, {"post", "--file", "-", "--operation", "replacement"}} {
		code, out, stderr := discussionCLIInvoke(t, server.URL, "en-US", "{}", args...)
		if code != 2 || out != "" || stderr == "" {
			t.Error("invalid discussion flags accepted", args, code, out, stderr)
		}
	}
	for _, input := range []string{
		`{"project_id":"another-project","task_id":"task-one","expected_task_version":1,"body":"Text","idempotency_key":"original-key"}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1,"body":"Text"}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1e0,"body":"Text","idempotency_key":"original-key"}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1,"body":"Text","idempotency_key":"original-key","actor_kind":"assistant"}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1,"body":"Text","idempotency_key":"original-key","mention_resource_ids":["same","same"]}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1,"body":"Text","idempotency_key":"original-key","knowledge":null}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1,"body":"Text","idempotency_key":"original-key","knowledge":[{"id":"k","revision":2e0,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
		`{"project_id":"project-one","task_id":"task-one","expected_task_version":1,"body":"Text","idempotency_key":"original-key","knowledge":[{"id":"k","revision":2,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","content":"unreviewed text"}]}`,
	} {
		code, out, stderr := discussionCLIInvoke(t, server.URL, "en-US", input, "post", "--file", "-")
		if code != 2 || out != "" || stderr == "" {
			t.Error("invalid original post accepted", code, out, stderr)
		}
	}
	if calls != 0 {
		t.Fatal("invalid local input reached transport", calls)
	}
}

func TestTaskDiscussionsCLIHelpLanguages(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		code, out, stderr := discussionCLIInvoke(t, "http://127.0.0.1:1", locale, "", "--help")
		if code != 0 || stderr != "" || !strings.Contains(out, "discussion post --file") || !strings.Contains(out, "discussion people") || locale == "zh-CN" && !strings.Contains(out, "任务讨论") || locale == "en-US" && !strings.Contains(out, "Task discussions") {
			t.Fatal(locale, code, out, stderr)
		}
	}
}
