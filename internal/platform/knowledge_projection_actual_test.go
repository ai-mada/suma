package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ic "github.com/ai-mada/suma/internal/indexcontract"
)

// Native indexing is deliberately opt-in: ordinary domain checks do not start
// Node, a native database or an embedding transport. The embedding endpoint in
// this journey is an owned deterministic fixture, never a commercial model.
func TestKnowledgeProjectionActualNativeLifecycleAndExactTaskContext(t *testing.T) {
	if os.Getenv("SUMA_KNOWLEDGE_PROJECTION_197_ACTUAL") != "1" {
		t.Skip("explicit isolated native index journey")
	}
	node := os.Getenv("SUMA_KNOWLEDGE_PROJECTION_197_NODE")
	if !filepath.IsAbs(node) {
		t.Fatal("an explicit installed Node executable is required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	evidenceName := "actual"
	if suffix := os.Getenv("SUMA_KNOWLEDGE_PROJECTION_197_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("only an independent final evidence directory is accepted")
		}
		evidenceName += "-" + suffix
	}
	dir := filepath.Join(root, ".cache", "knowledge-projection-197", evidenceName)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "receipt.json")); !os.IsNotExist(err) {
		t.Fatal("retain previous actual evidence instead of overwriting it")
	}
	var embeddingCalls atomic.Int32
	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Model != "owned-fixture-embedding" || len(input.Input) < 1 {
			http.Error(w, "owned embedding input", 400)
			return
		}
		embeddingCalls.Add(1)
		data := []map[string]any{}
		for i, text := range input.Input {
			vector := []float64{0, 1}
			if strings.Contains(text, "rollback") || strings.Contains(text, "rescue") {
				vector = []float64{1, 0}
			}
			data = append(data, map[string]any{"index": i, "embedding": vector})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer embedding.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 170*time.Second)
	defer cancel()
	token := newID() + newID()
	var lifetimes []map[string]any
	start := func(number int, version, operation string) (string, func()) {
		t.Helper()
		label := fmt.Sprintf("lifetime%d", number)
		cmd := exec.CommandContext(ctx, node, filepath.Join(root, ".cache", "knowledge-projection-197", "actual-server.mjs"))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "SUMA_KNOWLEDGE_197_MODULE_DIR="+filepath.Join(root, ".cache", "knowledge-projection-197", "projection", "out", "src"), "SUMA_KNOWLEDGE_197_NATIVE_DIR="+filepath.Join(dir, "native"), "SUMA_KNOWLEDGE_197_INTERNAL_TOKEN="+token, "SUMA_KNOWLEDGE_197_EMBEDDING_URL="+embedding.URL, "SUMA_KNOWLEDGE_197_EMBEDDING_VERSION="+version, "SUMA_KNOWLEDGE_197_REBUILD_OPERATION_ID="+operation, "SUMA_KNOWLEDGE_197_LIFETIME="+label)
		stdin, e := cmd.StdinPipe()
		if e != nil {
			t.Fatal(e)
		}
		stdout, e := cmd.StdoutPipe()
		if e != nil {
			t.Fatal(e)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		wait := make(chan error, 1)
		go func() { wait <- cmd.Wait() }()
		ready := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				var line struct {
					Ready  bool   `json:"ready"`
					Origin string `json:"origin"`
				}
				if json.Unmarshal(scanner.Bytes(), &line) == nil && line.Ready {
					ready <- line.Origin
					return
				}
			}
		}()
		stopped := false
		stop := func() {
			t.Helper()
			if stopped {
				return
			}
			stopped = true
			io.WriteString(stdin, "shutdown\n")
			stdin.Close()
			select {
			case e := <-wait:
				os.WriteFile(filepath.Join(dir, label+".stderr.txt"), stderr.Bytes(), 0600)
				if e != nil {
					t.Fatalf("owned native service exit: %v; %s", e, stderr.String())
				}
			case <-time.After(15 * time.Second):
				cmd.Process.Kill()
				<-wait
				t.Fatal("owned native service did not acknowledge shutdown")
			}
			marker, e := os.ReadFile(filepath.Join(dir, "native", "owned-shutdown-"+label+".json"))
			if e != nil {
				t.Fatal(e)
			}
			var receipt map[string]any
			if json.Unmarshal(marker, &receipt) != nil || receipt["closed"] != true {
				t.Fatal("native shutdown evidence missing")
			}
			lifetimes = append(lifetimes, receipt)
		}
		t.Cleanup(stop)
		select {
		case origin := <-ready:
			u, e := url.Parse(origin)
			if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
				t.Fatal("unexpected native listener")
			}
			return origin, stop
		case e := <-wait:
			stopped = true
			t.Fatalf("native startup: %v; %s", e, stderr.String())
		case <-time.After(25 * time.Second):
			t.Fatal("owned native startup deadline")
		}
		return "", stop
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var requests []map[string]any
	rawCall := func(method, origin, path string, raw []byte, cookie *http.Cookie, want int, user, organization string) map[string]any {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
			if method != http.MethodGet {
				req.Header.Set("X-Suma-Expected-User", user)
				req.Header.Set("X-Suma-Expected-Organization", organization)
			}
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		response.Body.Close()
		statusAccepted := response.StatusCode == want || want == 0 && (response.StatusCode == 200 || response.StatusCode == 503)
		if e != nil || !statusAccepted {
			t.Fatalf("actual %s %s: status=%d want=%d err=%v body=%s", method, path, response.StatusCode, want, e, body)
		}
		requests = append(requests, map[string]any{"method": method, "path": path, "status": response.StatusCode, "response_sha256": ic.Digest(body)})
		var value map[string]any
		if json.Unmarshal(body, &value) != nil {
			t.Fatal("actual response is not an object")
		}
		return value
	}
	awaitReady := func(origin string) map[string]any {
		t.Helper()
		for i := 0; i < 60; i++ {
			// A live rebuilding owner may expose a readiness503. Its actual phase
			// remains observable; only a genuinely verified ready generation passes.
			status := rawCall("GET", origin, "/health", nil, nil, 0, "", "")
			if status["phase"] == "ready" {
				if status["verified"] != true {
					t.Fatal("native generation has not verified its actual backend", status)
				}
				return status
			}
			if status["phase"] != "rebuilding" {
				t.Fatal("native fixture is not progressing toward a ready generation", status)
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("native rebuild did not complete within the owned fixture bound")
		return nil
	}
	origin, stop1 := start(1, "owned-embedding-v1", "")
	initialHealth := awaitReady(origin)
	t.Setenv("SUMA_KNOWLEDGE_URL", origin)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", token)
	f, _, dispatch, _ := taskDispatchFixture(t)
	t.Cleanup(func() {
		// Retain the isolated SQL source even if a later native assertion fails.
		// This runs before the existing fixture closes its database connection.
		file := filepath.Join(dir, "private.sqlite")
		if _, e := os.Stat(file); os.IsNotExist(e) {
			if _, e = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(file, "'", "''") + "'"); e != nil {
				t.Logf("isolated SQL evidence preservation: %v", e)
			}
		}
	})
	server := httptest.NewServer(f.handler)
	defer server.Close()
	call := func(method, path string, value any, want int) map[string]any {
		t.Helper()
		var raw []byte
		if value != nil {
			raw, err = json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
		}
		return rawCall(method, server.URL, "/api/v1"+path, raw, f.cookie, want, f.user.ID, f.user.TenantID)
	}
	query := func() map[string]any {
		t.Helper()
		return call("GET", "/project-knowledge/index-search?project_id="+f.project+"&q=rescue&limit=10", nil, 200)
	}
	create := func(title string) map[string]any {
		t.Helper()
		draft := call("POST", "/project-knowledge", f.body(map[string]any{"kind": "memory", "title": title, "summary": "Reviewed native index fixture", "content": "🧭 rollback 先核对原回执，再决定恢复。\nApproved evidence and a bounded recovery procedure.", "sources": []any{}, "expected_version": 0}), 201)
		return call("POST", "/project-knowledge/"+draft["id"].(string)+"/commands", f.body(map[string]any{"action": "publish", "expected_version": draft["version"]}), 200)
	}
	published := create("Owned native publication")
	if err = f.s.ProjectIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	result := query()
	if result["mode"] != "lancedb_hybrid" || len(result["items"].([]any)) != 1 {
		t.Fatal("real configured hybrid path did not yield the approved publication", result)
	}
	hit := result["items"].([]any)[0].(map[string]any)
	if stringify(hit["reference"]) != stringify(taskKnowledgeReference(published)) || hit["title"] != published["published_title"] {
		t.Fatal("native hit lost its canonical publication")
	}
	location := hit["locator"].(map[string]any)
	content := []rune(published["published"].(map[string]any)["content"].(string))
	startAt, endAt := int(location["start"].(float64)), int(location["end"].(float64))
	if location["unit"] != "unicode_codepoints" || startAt < 0 || endAt <= startAt || endAt > len(content) || string(content[startAt:endAt]) != hit["snippet"] {
		t.Fatal("public snippet was not reconstructed from the exact codepoint range", hit)
	}
	dispatch["knowledge"] = []any{hit["reference"]}
	plan := call("POST", "/task-dispatch-plans", dispatch, 201)
	if taskKnowledgeSnapshot(t, plan["context"].(map[string]any))["sha256"] != published["published_sha256"] {
		t.Fatal("native discovery did not enter the original exact task context")
	}
	var originalPayload string
	if err = f.s.DB.QueryRow("SELECT payload FROM suma_index_outbox WHERE source_kind='project_knowledge' AND source_id=? AND action='upsert'", published["id"]).Scan(&originalPayload); err != nil {
		t.Fatal(err)
	}
	edited := call("PATCH", "/project-knowledge/"+published["id"].(string), f.body(map[string]any{"kind": "memory", "title": "UNPUBLISHED_ONLY_197", "summary": "A draft is not a publication", "content": "UNPUBLISHED_ONLY_197", "sources": []any{}, "expected_version": published["version"]}), 200)
	if stringify(query()["items"].([]any)[0].(map[string]any)["reference"]) != stringify(hit["reference"]) {
		t.Fatal("an unpublished aggregate version invalidated the old publication")
	}
	call("POST", "/project-knowledge/"+published["id"].(string)+"/commands", f.body(map[string]any{"action": "retire", "expected_version": edited["version"]}), 200)
	if len(query()["items"].([]any)) != 0 {
		t.Fatal("current retirement accepted an old native hit before tombstone delivery")
	}
	if err = f.s.ProjectIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	beforeReplay := embeddingCalls.Load()
	replayed := rawCall("POST", origin, "/index", []byte(originalPayload), nil, 200, "", "")
	if replayed["disposition"] != "obsolete" || replayed["request_sha256"] != ic.Digest([]byte(originalPayload)) || embeddingCalls.Load() != beforeReplay || len(query()["items"].([]any)) != 0 {
		t.Fatal("late original event revived a tombstone or repeated embedding", replayed)
	}
	stop1()
	origin, stop2 := start(2, "owned-embedding-v1", "")
	awaitReady(origin)
	t.Setenv("SUMA_KNOWLEDGE_URL", origin)
	if len(query()["items"].([]any)) != 0 {
		t.Fatal("native restart lost its deletion tombstone")
	}
	active := create("A separately reviewed next publication")
	if err = f.s.ProjectIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if len(query()["items"].([]any)) != 1 {
		t.Fatal("new reviewed knowledge could not be projected after restart")
	}
	stop2()
	origin, stop3 := start(3, "owned-embedding-v2", "owned-rebuild-197")
	finalHealth := awaitReady(origin)
	t.Setenv("SUMA_KNOWLEDGE_URL", origin)
	finalResult := query()
	if finalResult["generation"] == result["generation"] || len(finalResult["items"].([]any)) != 1 || stringify(finalResult["items"].([]any)[0].(map[string]any)["reference"]) != stringify(taskKnowledgeReference(active)) {
		t.Fatal("explicit generation rebuild lost current publications or retained the old generation", finalResult)
	}
	call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
	status := call("GET", "/project-knowledge/index-status?project_id="+f.project, nil, 200)
	if status["coverage"] != "outbox_delivery_only" || status["pending"] != float64(0) || status["failed"] != float64(0) || status["total"] != float64(3) || taskKnowledgeEffectCounts(t, f).runs != 0 {
		t.Fatal("delivery facts or original task gate were changed", status)
	}
	stop3()
	if _, err = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(filepath.Join(dir, "private.sqlite"), "'", "''") + "'"); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{"stage": "T-KNOWLEDGE-PROJECTION-197", "status": "actual_http_sqlite_native_lance_hybrid_restart_tombstone_and_explicit_generation_confirmed", "native_lifetimes": lifetimes, "controlled_embedding_requests": embeddingCalls.Load(), "commercial_embedding_calls": 0, "model_calls": 0, "business_runs": 0, "customer_acceptance": false, "approved_reference": hit["reference"], "final_reference": taskKnowledgeReference(active), "canonical_locator": location, "initial_health": initialHealth, "final_health": finalHealth, "delivery_status": status, "original_payload_sha256": ic.Digest([]byte(originalPayload)), "replay_disposition": replayed["disposition"], "requests": requests}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil || os.WriteFile(filepath.Join(dir, "receipt.json"), append(encoded, '\n'), 0600) != nil {
		t.Fatal("cannot persist the actual journey receipt", err)
	}
}
