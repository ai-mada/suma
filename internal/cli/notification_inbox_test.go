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

func inboxCLIInvoke(t *testing.T, origin, locale string, command ...string) (int, string, string) {
	t.Helper()
	args := []string{"--control", origin, "--language", locale, "--project", "project-one", "--token", "original-user-api-token", "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "inbox"}
	args = append(args, command...)
	var out, stderr strings.Builder
	code := Execute(context.Background(), args, strings.NewReader(""), &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestNotificationInboxCLIListScopeAndOpaqueSequence(t *testing.T) {
	for _, filter := range []string{"", "inbox", "unread", "archived"} {
		t.Run("filter_"+filter, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				wantFilter, wantLimit := filter, "50"
				if wantFilter == "" {
					wantFilter, wantLimit = "inbox", "20"
				}
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/notification-inbox" || q.Get("project_id") != "project-one" || q.Get("filter") != wantFilter || q.Get("limit") != wantLimit || q.Has("user_id") || q.Has("recipient_id") || r.Header.Get("Authorization") != "Bearer original-user-api-token" {
					t.Error("personal authorized list scope changed", r.Method, r.URL)
				}
				if filter != "" && q.Get("cursor") != "original_cursor-1" || filter == "" && q.Has("cursor") {
					t.Error("original page cursor changed", q)
				}
				_, _ = w.Write([]byte(`{"project_id":"project-one","user_id":"current-user","items":[{"id":"notice-1","version":1,"sequence":"9007199254740993","read":false,"archived":false,"source_available":false,"assignment_current":false}],"total":3,"limit":20,"counts":{"inbox":3,"unread":2,"archived":1},"next_cursor":"continue-explicitly","as_of":"2026-10-29T00:00:00Z"}`))
			}))
			defer server.Close()
			args := []string{"list"}
			if filter != "" {
				args = append(args, "--filter", filter, "--limit", "50", "--cursor", "original_cursor-1")
			}
			code, out, stderr := inboxCLIInvoke(t, server.URL, "en-US", args...)
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, `"sequence": "9007199254740993"`) || !strings.Contains(out, `"read": false`) || !strings.Contains(out, `"next_cursor": "continue-explicitly"`) {
				t.Fatal("list changed machine facts, wrote read state or followed a cursor", code, calls, out, stderr)
			}
		})
	}
}

func TestNotificationInboxCLICommandsPreserveExplicitOriginalRequest(t *testing.T) {
	for _, action := range []string{"read", "unread", "archive", "restore"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				var body map[string]any
				if decoder.Decode(&body) != nil || !reflect.DeepEqual(body, map[string]any{"project_id": "project-one", "expected_version": json.Number("2147483646"), "action": action, "idempotency_key": "原通知操作-0001"}) || r.Method != http.MethodPost || r.URL.Path != "/api/v1/notification-inbox/notice-1/commands" {
					t.Error("explicit original notification command changed", r.Method, r.URL, body)
				}
				if r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) || r.Header.Get("Accept-Language") != "zh-CN" {
					t.Error("original account view or request language changed")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"idempotency_key": "原通知操作-0001", "notification_id": "notice-1", "version": 2147483646, "action": action, "read": false, "archived": true, "read_at": "", "archived_at": "2026-10-29T00:00:00Z", "updated_at": "2026-10-29T00:00:00Z"})
			}))
			defer server.Close()
			code, out, stderr := inboxCLIInvoke(t, server.URL, "zh-CN", action, "notice-1", "--version", "2147483646", "--operation", "原通知操作-0001")
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, `"read": false`) || !strings.Contains(out, `"archived": true`) || !strings.Contains(out, "原通知操作-0001") {
				t.Fatal(code, calls, out, stderr)
			}
		})
	}
}

func TestNotificationInboxCLIRecoveryRequiresSameCallerVersionAndOperation(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, string(raw))
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notification-inbox/notice-1/commands" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
			t.Error("original recovery identity or target changed")
		}
		if len(bodies) == 1 {
			connection, _, hijackErr := w.(http.Hijacker).Hijack()
			if hijackErr != nil {
				t.Error(hijackErr)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = w.Write([]byte(`{"idempotency_key":"original-notice-operation","notification_id":"notice-1","version":4,"action":"archive","read":false,"archived":true,"read_at":"","archived_at":"2026-10-29T00:00:00Z","updated_at":"2026-10-29T00:00:00Z"}`))
	}))
	defer server.Close()
	args := []string{"archive", "notice-1", "--version", "3", "--operation", "original-notice-operation"}
	code, out, stderr := inboxCLIInvoke(t, server.URL, "en-US", args...)
	if code != 1 || out != "" || len(bodies) != 1 || !strings.Contains(stderr, "outcome may be unknown") {
		t.Fatal("lost response was retried or reported as success", code, len(bodies), out, stderr)
	}
	code, recovered, stderr := inboxCLIInvoke(t, server.URL, "en-US", args...)
	if code != 0 || stderr != "" || len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatal("explicit recovery changed original request", code, bodies, recovered, stderr)
	}
	code, replay, stderr := inboxCLIInvoke(t, server.URL, "en-US", args...)
	if code != 0 || stderr != "" || len(bodies) != 3 || bodies[1] != bodies[2] || replay != recovered {
		t.Fatal("receipt replay changed request or response", code, bodies, replay, stderr)
	}
}

func TestNotificationInboxCLIInvalidParametersNeverReachTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, args := range [][]string{
		{}, {"list", "--filter", "all"}, {"list", "--filter", ""}, {"list", "--user", "another-user"}, {"list", "--mark-read"},
		{"list", "--limit", "0"}, {"list", "--limit", "51"}, {"list", "--limit", "01"}, {"list", "--limit", "1e1"},
		{"list", "--cursor", strings.Repeat("x", 4097)}, {"list", "--cursor", "bad\ncursor"}, {"list", "extra"},
		{"read", "../notice", "--version", "1", "--operation", "original-key"}, {"read", "notice-1", "--version", "0", "--operation", "original-key"},
		{"read", "notice-1", "--version", "2147483647", "--operation", "original-key"}, {"read", "notice-1", "--version", "01", "--operation", "original-key"},
		{"archive", "notice-1", "--version", "1"}, {"restore", "notice-1", "--version", "1", "--operation", ""},
		{"unread", "notice-1", "--version", "1", "--operation", "short"}, {"unread", "notice-1", "--version", "1", "--operation", strings.Repeat("x", 129)},
		{"unread", "notice-1", "--version", "1", "--operation", "        "}, {"unread", "notice-1", "--version", "1", "--operation", "bad\u0085operation"},
		{"read", "notice-1", "--version", "1", "--operation", "original-key", "--user", "another-user"}, {"create", "notice-1"},
	} {
		code, out, stderr := inboxCLIInvoke(t, server.URL, "en-US", args...)
		if code != 2 || out != "" || stderr == "" {
			t.Error("invalid personal inbox command accepted", args, code, out, stderr)
		}
	}
	for _, command := range [][]string{{"list"}, {"read", "notice-1", "--version", "1", "--operation", "original-key"}} {
		var out, stderr strings.Builder
		args := append([]string{"--control", server.URL, "inbox"}, command...)
		if code := Execute(context.Background(), args, strings.NewReader(""), &out, &stderr); code != 2 {
			t.Error("missing project accepted", code, stderr.String())
		}
	}
	if calls != 0 {
		t.Fatal("invalid command reached transport", calls)
	}
}

func TestNotificationInboxCLICurrentFailureIsNotRetriedOrTranslated(t *testing.T) {
	for _, command := range [][]string{{"list"}, {"read", "notice-1", "--version", "1", "--operation", "original-key"}} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"原接收人当前授权已收回","message_key":"inbox.current_denied","locale":"zh-CN","localized":false,"details":{"scope":"current"}}}`))
		}))
		code, out, stderr := inboxCLIInvoke(t, server.URL, "en-US", command...)
		server.Close()
		var envelope struct {
			Error failure `json:"error"`
		}
		if code != 1 || calls != 1 || out != "" || json.Unmarshal([]byte(stderr), &envelope) != nil || envelope.Error.Status != 403 || envelope.Error.Code != "forbidden" || envelope.Error.Message != "原接收人当前授权已收回" || envelope.Error.Locale != "zh-CN" || envelope.Error.Localized == nil || *envelope.Error.Localized {
			t.Fatal("current server authorization or diagnostics changed", code, calls, out, stderr)
		}
	}
}

func TestNotificationInboxCLIHelpAndLocalUsageLanguages(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		code, out, stderr := inboxCLIInvoke(t, "http://127.0.0.1:1", locale, "--help")
		if code != 0 || stderr != "" || !strings.Contains(out, "inbox list") || !strings.Contains(out, "--operation") || !strings.Contains(out, "--version") {
			t.Fatal("inbox help omitted recovery flags", code, out, stderr)
		}
		code, out, stderr = inboxCLIInvoke(t, "http://127.0.0.1:1", locale, "archive", "notice-1", "--version", "1")
		if code != 2 || out != "" || !strings.Contains(stderr, "cli.notification_inbox.usage") || locale == "zh-CN" && !strings.Contains(stderr, "个人收件箱") || locale == "en-US" && !strings.Contains(stderr, "inbox list") {
			t.Fatal("local inbox usage language changed", locale, code, out, stderr)
		}
	}
}
