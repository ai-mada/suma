package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Opt-in actual HTTP/storage/browser journey. The fresh task and source file
// are local development data, not a Runner, model, test or approval simulation.
func TestTaskConversationActualHTTPAndOriginalReceipt(t *testing.T) {
	if os.Getenv("SUMA_CONVERSATION_UI_DIR") == "" {
		t.Skip("explicit fresh conversation UI directory and final web build are required")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(repository, ".cache", "e2e", "collab-127")
	web := workbenchRetainedDirectory(t, os.Getenv("SUMA_CONVERSATION_WEB_DIR"), owned, false)
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("build the final conversation interface before starting its HTTP journey", err)
	}
	dir := workbenchRetainedDirectory(t, os.Getenv("SUMA_CONVERSATION_UI_DIR"), owned, true)
	if under(web, dir) || under(dir, web) {
		t.Fatal("isolated receipts and final web assets must be separate")
	}
	code := filepath.Join(dir, "code")
	if err = os.Mkdir(code, 0700); err != nil {
		t.Fatal(err)
	}
	source := []byte("module.exports=(a,b)=>a+b;\n")
	if err = os.WriteFile(filepath.Join(code, "sum.cjs"), source, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUMA_WORKSPACE_ROOT", code)
	f := newFixture(t) // A new initial SQLite schema; never reopen an older DB.
	server, origin, port := modelSelectionGUIServer(t, f, web, "collab127.localhost")
	t.Cleanup(server.Close)
	const marker = "T127 browser receipt recovery"
	type intercepted struct {
		Key, RequestSHA256, MessageSHA256 string
		Original                          TaskConversationMessage
		Dropped                           bool
		Replays                           int
		Error                             string
	}
	var mu sync.Mutex
	var lost intercepted
	originalHandler := server.Config.Handler
	// The only fault is a transport ACK lost after the original production
	// handler has committed201. No business row or response is fabricated.
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/task-conversations" {
			originalHandler.ServeHTTP(w, r)
			return
		}
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, (128<<10)+1))
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var request TaskConversationAppendRequest
		if readErr != nil || json.Unmarshal(raw, &request) != nil || request.Body != marker {
			originalHandler.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		response := httptest.NewRecorder()
		originalHandler.ServeHTTP(response, r)
		var message TaskConversationMessage
		if response.Code == http.StatusCreated && json.Unmarshal(response.Body.Bytes(), &message) == nil {
			if !lost.Dropped {
				lost.Key, lost.RequestSHA256, lost.MessageSHA256 = request.IdempotencyKey, fmt.Sprintf("%x", sha256.Sum256(raw)), fmt.Sprintf("%x", sha256.Sum256([]byte(request.Body)))
				lost.Original = message
				if hijacker, ok := w.(http.Hijacker); ok {
					connection, _, e := hijacker.Hijack()
					if e == nil {
						lost.Dropped = true
						_ = connection.Close()
						return
					}
					lost.Error = e.Error()
				} else {
					lost.Error = "local HTTP transport lacks a real response-loss mechanism"
				}
			} else if request.IdempotencyKey != lost.Key || stringify(message) != stringify(lost.Original) {
				lost.Error = "original browser operation was replaced instead of recovered"
			} else {
				lost.Replays++
			}
		}
		for key, values := range response.Header() {
			w.Header()[key] = append([]string{}, values...)
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
	client := &http.Client{Timeout: 15 * time.Second}
	call := func(method, path string, body any, want int, target any) {
		t.Helper()
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		request, e := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		request.Host = strings.TrimPrefix(origin, "http://")
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Suma-Expected-User", f.user.ID)
		request.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		request.AddCookie(f.cookie)
		response, e := client.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		payload, e := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if e != nil || response.StatusCode != want {
			t.Fatalf("real conversation HTTP %s %s => %d; want%d (%v)", method, path, response.StatusCode, want, e)
		}
		if target != nil {
			if e = json.Unmarshal(payload, target); e != nil {
				t.Fatal(e)
			}
		}
	}
	var requirement, task Record
	call("POST", "/requirements", f.body(map[string]any{"title": "可靠的任务协作 / Reliable task collaboration", "acceptance_criteria": []string{"Keep the original note once after a lost response", "Discussion must not dispatch or accept work"}}), 201, &requirement)
	call("POST", "/tasks", f.body(map[string]any{"title": "任务讨论与代码工作台 / Task discussion and workspace", "requirement_id": requirement.ID, "description": "Discuss the exact task context beside real project files."}), 201, &task)
	call("POST", "/workspace/binding", f.body(map[string]any{"path": code}), 200, &WorkspaceMetadata{})
	seed, seedSHA := workbenchRetainedProtected(t, f)
	first := taskConversationRequest(f, task.ID, task.Version, "中文协作记录：先确认需求依据，再明确派发。")
	var message, replay TaskConversationMessage
	call("POST", "/task-conversations", first, 201, &message)
	call("POST", "/task-conversations", first, 201, &replay)
	if stringify(message) != stringify(replay) || message.Sequence != 1 || message.RequirementID != requirement.ID {
		t.Fatal("real HTTP replay did not recover the original immutable note")
	}
	call("POST", "/task-conversations", taskConversationRequest(f, task.ID, task.Version, "English collaboration note: posting is separate from execution and acceptance."), 201, &replay)
	query := "?project_id=" + url.QueryEscape(f.project) + "&task_id=" + url.QueryEscape(task.ID)
	var page, older TaskConversationPage
	call("GET", "/task-conversations"+query+"&limit=1", nil, 200, &page)
	call("GET", "/task-conversations"+query+fmt.Sprintf("&limit=1&before=%d", page.NextBefore), nil, 200, &older)
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Sequence != 2 || len(older.Items) != 1 || stringify(older.Items[0]) != stringify(message) || older.NextBefore != 0 {
		t.Fatal("actual HTTP conversation pagination or original replay changed")
	}
	state := map[string]any{"stage": "T-COLLAB-127", "kind": "actual_http_task_collaboration", "status": "ready", "origin": origin, "project_id": f.project, "task_id": task.ID, "task_version": task.Version, "requirement_id": requirement.ID, "seed_counts": seed, "seed_protected_sha256": seedSHA, "source_sha256": fmt.Sprintf("%x", sha256.Sum256(source)), "new_executions": 0, "model_calls": 0, "customer_acceptance": false, "stop_reason": "timeout"}
	t.Cleanup(func() {
		server.Close() // Finish all original writes before observing final facts.
		final, finalSHA := workbenchRetainedProtected(t, f)
		state["final_counts"], state["final_protected_sha256"] = final, finalSHA
		if !reflect.DeepEqual(seed, final) || seedSHA != finalSHA {
			state["status"] = "protected_state_changed"
			t.Error("conversation changed a task, run, quality, acceptance or release fact")
		}
		all, e := f.s.ListTaskConversation(context.Background(), f.actor, f.project, task.ID, 50, 0)
		mu.Lock()
		state["lost_response"] = map[string]any{"idempotency_key": lost.Key, "request_sha256": lost.RequestSHA256, "message_sha256": lost.MessageSHA256, "original": lost.Original, "dropped_after_commit": lost.Dropped, "original_receipt_replays": lost.Replays, "error": lost.Error}
		if e != nil || all.Total != 3 || len(all.Items) != 3 || !lost.Dropped || lost.Replays < 1 || lost.Error != "" || stringify(all.Items[2]) != stringify(lost.Original) {
			state["status"] = "original_receipt_not_verified"
			t.Error("browser must recover the same persisted third note exactly once", e)
		}
		mu.Unlock()
		state["conversation"] = all
		if actual, e := os.ReadFile(filepath.Join(code, "sum.cjs")); e != nil || !bytes.Equal(actual, source) {
			state["status"] = "source_changed"
			t.Error("conversation interface changed the original source bytes", e)
		}
		retained := filepath.Join(dir, "fixture.sqlite")
		if _, e = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(retained, "'", "''") + "'"); e != nil {
			t.Error(e)
		} else if raw, e := os.ReadFile(retained); e != nil {
			t.Error(e)
		} else {
			state["database_bytes"], state["database_sha256"] = len(raw), fmt.Sprintf("%x", sha256.Sum256(raw))
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"origin": origin, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "task_id": task.ID, "browser_note": marker, "allowed_actions": []string{"read", "post_exact_browser_note_once", "recover_original_receipt", "refresh"}})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "origin": origin, "port": port, "project_id": f.project, "task_id": task.ID, "browser_note": marker, "new_executions": 0, "model_calls": 0})
	t.Logf("actual conversation HTTP ready at %s; post the specified note once, then recover its original receipt", origin)
	deadline, stop := time.NewTimer(5*time.Minute), time.NewTicker(200*time.Millisecond)
	defer deadline.Stop()
	defer stop.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("conversation browser verification must close explicitly before timeout")
		case <-stop.C:
			if _, e := os.Lstat(filepath.Join(dir, "stop.request")); e == nil {
				state["stop_reason"], state["status"] = "explicit_stop", "closed_after_original_receipt_recovery"
				return
			} else if !os.IsNotExist(e) {
				t.Fatal(e)
			}
		}
	}
}
