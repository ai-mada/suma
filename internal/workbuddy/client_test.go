package workbuddy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureToken = "secret-access-token"

func clientFixture(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{BaseURL: server.URL, Token: fixtureToken, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}
func requireError(t *testing.T, err error, code string, unknown bool) *Error {
	t.Helper()
	var detail *Error
	if !errors.As(err, &detail) || detail.Code != code || detail.OutcomeUnknown != unknown {
		t.Fatalf("unexpected safe error: %v", err)
	}
	for _, text := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
		for _, secret := range []string{fixtureToken, "sandbox-secret", "private-prompt", "sensitive-server-error"} {
			if strings.Contains(text, secret) {
				t.Fatal("error leaked secret")
			}
		}
	}
	return detail
}
func TestClientRoutesAndSafeTaskProjection(t *testing.T) {
	var calls int
	request := CreateRequest{Prompt: "实现中文需求\n保留上下文", Name: "本次任务"}
	client, server := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+fixtureToken || r.Header.Get("Accept") != "application/json" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("unexpected authentication or synthetic idempotency")
		}
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			if r.Method != "POST" || r.URL.RequestURI() != "/openapi/v2/tasks" || r.Header.Get("Content-Type") != "application/json" {
				t.Error("wrong create route")
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			var got map[string]any
			if json.Unmarshal(raw, &got) != nil || len(got) != 2 || got["prompt"] != request.Prompt || got["name"] != request.Name {
				t.Error("wrong typed create body")
			}
			hash, _ := CreateRequestDigest(request)
			sum := sha256.Sum256(raw)
			if hash != hex.EncodeToString(sum[:]) {
				t.Error("digest differs from actual wire")
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"task_id":"2076261663968759808","status":"working","name":"任务","link":"https://acp.workbuddy.cn/secret","token":"sandbox-secret","sandboxLink":"https://private/secret","expire_at":123}`)
		case 2:
			if r.Method != "GET" || r.URL.RequestURI() != "/openapi/v2/tasks/2076261663968759808" {
				t.Error("wrong exact GET route")
			}
			io.WriteString(w, `{"task_id":"2076261663968759808","status":"future_vendor_state","name":"任务","token":"sandbox-secret"}`)
		case 3:
			if r.Method != "GET" || r.URL.Path != "/openapi/v2/tasks" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("size") != "1" || len(r.URL.Query()) != 2 {
				t.Error("wrong list route")
			}
			io.WriteString(w, `{"tasks":[{"task_id":"two","status":"completed","created_at":"2026-08-04T10:30:00Z"}],"total":2,"pagination":{"page":2,"size":1,"total":2}}`)
		default:
			t.Error("unexpected retry")
		}
	})
	if client.Endpoint() != server.URL+"/openapi/v2/tasks" {
		t.Fatal("wrong endpoint identity")
	}
	task, err := client.CreateTask(context.Background(), request)
	if err != nil || task.ID != "2076261663968759808" || task.Status != "working" {
		t.Fatal(task, err)
	}
	raw, _ := json.Marshal(task)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "link") || strings.Contains(string(raw), "token") {
		t.Fatal("vendor credential escaped projection")
	}
	task, err = client.GetTask(context.Background(), task.ID)
	if err != nil || task.Status != "future_vendor_state" {
		t.Fatal("unknown vendor status not preserved", err)
	}
	page, err := client.ListTasks(context.Background(), 2, 1)
	if err != nil || page.Total != 2 || page.Pagination.Page != 2 || len(page.Tasks) != 1 || page.Tasks[0].CreatedAt == "" {
		t.Fatal(page, err)
	}
	if calls != 3 {
		t.Fatal("request was retried")
	}
}
func TestClientRejectsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, request := range []CreateRequest{{}, {Prompt: " \n"}, {Prompt: string([]byte{255})}, {Prompt: strings.Repeat("a", 256*1024+1)}, {Prompt: "ok", Name: strings.Repeat("界", 513)}} {
		_, err := client.CreateTask(context.Background(), request)
		requireError(t, err, "invalid_request", false)
		if _, err = CreateRequestDigest(request); err == nil {
			t.Fatal("digest accepted request create rejects")
		}
	}
	for _, id := range []string{"", "../x", "a/b", "a%2fb", "a?x", "a#x", "a\\b", "a\n", ".abc", strings.Repeat("a", 201)} {
		_, err := client.GetTask(context.Background(), id)
		requireError(t, err, "invalid_request", false)
	}
	for _, paging := range [][2]int{{0, 20}, {1, 0}, {1, 101}, {-1, 20}} {
		_, err := client.ListTasks(context.Background(), paging[0], paging[1])
		requireError(t, err, "invalid_request", false)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.CreateTask(ctx, CreateRequest{Prompt: "ok"})
	requireError(t, err, "cancelled", false)
	if calls.Load() != 0 {
		t.Fatal("local invalid input or canceled context reached network")
	}
}
func TestClientResponseValidationAndSingleSubmission(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"http_error", `sensitive-server-error ` + fixtureToken, 500},
		{"wrong_success_code", `{"task_id":"one","status":"working"}`, 200},
		{"missing_status", `{"task_id":"one"}`, 201},
		{"null_status", `{"task_id":"one","status":null}`, 201},
		{"empty_status", `{"task_id":"one","status":""}`, 201},
		{"wrong_id_type", `{"task_id":1,"status":"working"}`, 201},
		{"path_id", `{"task_id":"a/b","status":"working"}`, 201},
		{"duplicate_id", `{"task_id":"one","task_id":"two","status":"working"}`, 201},
		{"escaped_duplicate", `{"task_id":"one","\u0074ask_id":"two","status":"working"}`, 201},
		{"duplicate_status", `{"task_id":"one","status":"working","status":"completed"}`, 201},
		{"trailing_json", `{"task_id":"one","status":"working"}{}`, 201},
		{"invalid_utf8", "{\"task_id\":\"one\",\"status\":\"" + string([]byte{255}) + "\"}", 201},
		{"unpaired_surrogate", `{"task_id":"one","status":"working","name":"\ud800"}`, 201},
		{"too_large", `{"task_id":"one","status":"working","unused":"` + strings.Repeat("a", 1024*1024) + `"}`, 201},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, err := client.CreateTask(context.Background(), CreateRequest{Prompt: "private-prompt"})
			code := "invalid_response"
			if tc.status != 201 {
				code = "http_status"
			}
			requireError(t, err, code, true)
			if calls != 1 {
				t.Fatal("unsafe submission retry")
			}
		})
	}
}
func TestClientGETIdentityAndPaginationValidation(t *testing.T) {
	cases := []struct {
		name, body string
		list       bool
	}{
		{"id_mismatch", `{"task_id":"other","status":"working"}`, false},
		{"missing_tasks", `{"total":0,"pagination":{"page":1,"size":2,"total":0}}`, true},
		{"null_tasks", `{"tasks":null,"total":0,"pagination":{"page":1,"size":2,"total":0}}`, true},
		{"wrong_page", `{"tasks":[],"total":0,"pagination":{"page":2,"size":2,"total":0}}`, true},
		{"wrong_size", `{"tasks":[],"total":0,"pagination":{"page":1,"size":3,"total":0}}`, true},
		{"wrong_total", `{"tasks":[],"total":2,"pagination":{"page":1,"size":2,"total":1}}`, true},
		{"negative_total", `{"tasks":[],"total":-1,"pagination":{"page":1,"size":2,"total":-1}}`, true},
		{"duplicate_paging", `{"tasks":[],"total":0,"pagination":{"page":1,"page":2,"size":2,"total":0}}`, true},
		{"duplicate_task", `{"tasks":[{"task_id":"one","status":"working"},{"task_id":"one","status":"working"}],"total":2,"pagination":{"page":1,"size":2,"total":2}}`, true},
		{"over_total", `{"tasks":[{"task_id":"one","status":"working"}],"total":0,"pagination":{"page":1,"size":2,"total":0}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			})
			var err error
			if tc.list {
				_, err = client.ListTasks(context.Background(), 1, 2)
			} else {
				_, err = client.GetTask(context.Background(), "one")
			}
			requireError(t, err, "invalid_response", false)
		})
	}
}
func TestClientOriginRedirectAndSecretBoundaries(t *testing.T) {
	for _, base := range []string{"http://www.workbuddy.cn", "https://www.workbuddy.cn.evil.test", "https://www.workbuddy.cn@evil.test", "https://www.workbuddy.cn:444", "https://www.workbuddy.cn/anything", "https://www.workbuddy.cn?token=secret", "https://www.workbuddy.cn#fragment", "http://192.168.1.10", "http://localhost.evil.test", "file:///secret"} {
		_, err := NewClient(Config{BaseURL: base, Token: fixtureToken})
		requireError(t, err, "invalid_config", false)
	}
	for _, token := range []string{"", "bad\r\ntoken", "bad token"} {
		_, err := NewClient(Config{Token: token})
		requireError(t, err, "invalid_config", false)
	}
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	var sourceCalls atomic.Int32
	client, source := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		http.Redirect(w, r, target.URL+"/stolen", 307)
	})
	_, err := client.CreateTask(context.Background(), CreateRequest{Prompt: "private-prompt"})
	requireError(t, err, "http_status", true)
	if targetCalls.Load() != 0 || sourceCalls.Load() != 1 {
		t.Fatal("followed redirect or retried")
	}
	cfg := Config{BaseURL: source.URL, Token: fixtureToken, HTTPClient: source.Client()}
	for _, value := range []any{cfg, &cfg, client} {
		raw, _ := json.Marshal(value)
		for _, formatted := range []string{string(raw), fmt.Sprintf("%v", value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if strings.Contains(formatted, fixtureToken) {
				t.Fatal("formatted client/config leaked token")
			}
		}
	}
}
func TestClientLostResponseIsUnknownWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	_, err := client.CreateTask(context.Background(), CreateRequest{Prompt: "ok"})
	requireError(t, err, "transport", true)
	if calls.Load() != 1 {
		t.Fatal("lost response caused repeated external effect")
	}
}

func TestClientSentRequestTimeoutAndReadBoundaries(t *testing.T) {
	var calls atomic.Int32
	client, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	client.http.Timeout = 25 * time.Millisecond
	_, err := client.CreateTask(context.Background(), CreateRequest{Prompt: "ok"})
	requireError(t, err, "transport", true)
	if calls.Load() != 1 {
		t.Fatal("timeout request was not sent exactly once")
	}
	for _, tc := range []struct{ name, contentType, body string }{
		{"wrong_media_type", "text/html", `{"task_id":"one","status":"working"}`},
		{"invalid_date", "application/json", `{"task_id":"one","status":"working","created_at":"yesterday"}`},
		{"null_name", "application/json", `{"task_id":"one","status":"working","name":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				io.WriteString(w, tc.body)
			})
			_, err := c.GetTask(context.Background(), "one")
			requireError(t, err, "invalid_response", false)
		})
	}
}
func TestClientPageRemainderAndCanonicalOrigins(t *testing.T) {
	for _, origin := range []string{"", "https://www.workbuddy.cn/", "https://www.workbuddy.cn:443"} {
		c, err := NewClient(Config{BaseURL: origin, Token: fixtureToken})
		if err != nil || c.Endpoint() != "https://www.workbuddy.cn/openapi/v2/tasks" {
			t.Fatal("official origin not canonical", err)
		}
	}
	if _, err := NewClient(Config{BaseURL: "http://127.0.0.1:", Token: fixtureToken}); err == nil {
		t.Fatal("empty port accepted")
	}
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"empty_beyond_total", `{"tasks":[],"total":1,"pagination":{"page":2,"size":2,"total":1}}`, true},
		{"item_beyond_total", `{"tasks":[{"task_id":"one","status":"idle"}],"total":1,"pagination":{"page":2,"size":2,"total":1}}`, false},
		{"larger_than_size", `{"tasks":[{"task_id":"one","status":"idle"},{"task_id":"two","status":"idle"},{"task_id":"three","status":"idle"}],"total":10,"pagination":{"page":2,"size":2,"total":10}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := clientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			})
			page, err := c.ListTasks(context.Background(), 2, 2)
			if tc.valid {
				if err != nil || page.Tasks == nil || len(page.Tasks) != 0 {
					t.Fatal(page, err)
				}
			} else {
				requireError(t, err, "invalid_response", false)
			}
		})
	}
}
