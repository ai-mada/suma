package platform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This opt-in journey uses a real HTTP listener and on-disk SQLite. Tool
// availability is explicitly the existing owned metadata fixture; this test
// does not execute a vendor, model, Runner or customer acceptance operation.
// It proves the new discovery-to-original-preview seam without repeating the
// already verified native development and quality pipeline.
func TestKnowledgeDiscoveryActualHTTPPublicationPreviewAndChangedBasis(t *testing.T) {
	if os.Getenv("SUMA_KNOWLEDGE_DISCOVERY_196_ACTUAL") != "1" {
		t.Skip("explicit local isolated actual journey")
	}
	f, _, dispatch, _ := taskDispatchFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	evidenceName := "actual"
	if suffix := os.Getenv("SUMA_KNOWLEDGE_DISCOVERY_196_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("only an independent final evidence directory is accepted")
		}
		evidenceName += "-" + suffix
	}
	dir := filepath.Join(root, ".cache", "knowledge-discovery-196", evidenceName)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "receipt.json")); !os.IsNotExist(err) {
		t.Fatal("retain the earlier receipt instead of overwriting actual evidence")
	}
	var requests []map[string]any
	call := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		var input io.Reader
		if body != nil {
			encoded, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(encoded)
		}
		req, e := http.NewRequest(method, server.URL+"/api/v1"+path, input)
		if e != nil {
			t.Fatal(e)
		}
		req.AddCookie(f.cookie)
		req.Header.Set("Content-Type", "application/json")
		if method != http.MethodGet {
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if e != nil || response.StatusCode != status {
			t.Fatalf("actual %s %s: status=%d want=%d err=%v body=%s", method, path, response.StatusCode, status, e, raw)
		}
		sum := sha256.Sum256(raw)
		requests = append(requests, map[string]any{"method": method, "path": path, "status": status, "response_sha256": hex.EncodeToString(sum[:]), "response_bytes": len(raw)})
		var out map[string]any
		if e = json.Unmarshal(raw, &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	const approved = "suma-discovery196-approved 中文引用 <scope> \\\u0022literal\\\u0022"
	const draftOnly = "suma-discovery196-unreviewed-draft-only"
	doc := call("POST", "/documents", f.body(map[string]any{"title": "Actual publication source", "description": approved}), 201)
	var publications []map[string]any
	for _, kind := range []string{"memory", "spec", "skill"} {
		created := call("POST", "/project-knowledge/extractions", f.body(map[string]any{"kind": kind, "title": "Approved " + kind, "sources": []any{map[string]any{"kind": "documents", "id": doc["id"], "version": doc["version"], "quote": approved}}}), 201)
		published := call("POST", "/project-knowledge/"+created["id"].(string)+"/commands", f.body(map[string]any{"action": "publish", "expected_version": created["version"]}), 200)
		publications = append(publications, published)
	}
	old := publications[0]
	oldID := old["id"].(string)
	oldRef := taskKnowledgeReference(old)
	edited := call("PATCH", "/project-knowledge/"+oldID, f.body(map[string]any{"kind": "memory", "title": draftOnly, "summary": draftOnly, "content": draftOnly, "sources": []any{}, "expected_version": old["version"]}), 200)
	before := taskKnowledgeEffectCounts(t, f)
	query := func(q, kind string, limit int, cursor string) map[string]any {
		t.Helper()
		values := url.Values{"project_id": {f.project}, "q": {q}, "limit": {strconv.Itoa(limit)}}
		if kind != "" {
			values.Set("kind", kind)
		}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		return call("GET", "/project-knowledge/discovery?"+values.Encode(), nil, 200)
	}
	page := query("suma-discovery196-approved", "memory", 10, "")
	hits, ok := page["items"].([]any)
	if !ok || len(hits) != 1 {
		t.Fatal("published memory not discoverable", page)
	}
	hit := hits[0].(map[string]any)
	if stringify(hit["reference"]) != stringify(oldRef) || hit["title"] != "Approved memory" || strings.Contains(stringify(page), draftOnly) || page["mode"] != "keyword" || page["scope"] != "current_project_publications" {
		t.Fatal("discovery substituted draft metadata or lost exact publication", page)
	}
	draftPage := query(draftOnly, "memory", 10, "")
	if len(draftPage["items"].([]any)) != 0 || draftPage["exhausted"] != true || draftPage["next_cursor"] != "" {
		t.Fatal("unreviewed draft text became discoverable", draftPage)
	}
	seen := map[string]bool{}
	cursor := ""
	for attempt := 0; attempt < 4; attempt++ {
		p := query("suma-discovery196-approved", "", 1, cursor)
		for _, item := range p["items"].([]any) {
			id := item.(map[string]any)["reference"].(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatal("keyset repeated a publication", id)
			}
			seen[id] = true
		}
		cursor = p["next_cursor"].(string)
		if p["exhausted"] == true {
			if cursor != "" {
				t.Fatal("exhausted page retained continuation")
			}
			break
		}
		if cursor == "" || attempt == 3 {
			t.Fatal("bounded current publication discovery did not finish")
		}
	}
	if len(seen) != 3 || before != taskKnowledgeEffectCounts(t, f) {
		t.Fatal("discovery changed business state or missed a published kind", seen)
	}
	revisionPath := "/project-knowledge/" + oldID + "/revisions/" + strconv.Itoa(int(old["published_revision"].(float64))) + "?project_id=" + f.project
	preview := call("GET", revisionPath, nil, 200)
	snapshot := preview["snapshot"].(map[string]any)
	if snapshot["sha256"] != old["published_sha256"] || !strings.Contains(snapshot["content"].(string), approved) || strings.Contains(stringify(snapshot), draftOnly) {
		t.Fatal("exact preview did not retain the approved original", snapshot)
	}
	dispatch["knowledge"] = []any{hit["reference"]}
	plan := call("POST", "/task-dispatch-plans", dispatch, 201)
	fixed := taskKnowledgeSnapshot(t, plan["context"].(map[string]any))
	if fixed["sha256"] != oldRef["sha256"] || fixed["revision"] != oldRef["revision"] || strings.Contains(stringify(fixed), draftOnly) {
		t.Fatal("explicit discovery selection did not reach the existing fixed task context", fixed)
	}
	newPublication := call("POST", "/project-knowledge/"+oldID+"/commands", f.body(map[string]any{"action": "publish", "expected_version": edited["version"]}), 200)
	if len(query("suma-discovery196-approved", "memory", 10, "")["items"].([]any)) != 0 {
		t.Fatal("new publication left the old body eligible for fresh discovery")
	}
	effects := taskKnowledgeEffectCounts(t, f)
	call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
	if taskKnowledgeEffectCounts(t, f) != effects || effects.runs != 0 {
		t.Fatal("changed publication started work or wrote an acceptance receipt")
	}
	historical := call("GET", revisionPath, nil, 200)
	if stringify(historical) != stringify(preview) {
		t.Fatal("publication replacement rewrote old immutable evidence")
	}
	var seq int
	var databaseName, databasePath string
	if err = f.s.DB.QueryRow("PRAGMA database_list").Scan(&seq, &databaseName, &databasePath); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(filepath.Join(dir, "private.sqlite"), "'", "''") + "'"); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{"stage": "T-KNOWLEDGE-DISCOVERY-196", "status": "actual_http_sqlite_publication_discovery_exact_preview_and_original_task_context_confirmed", "database": filepath.Base(databasePath), "publications": len(publications), "kinds": []string{"memory", "spec", "skill"}, "page_unique_publications": len(seen), "draft_isolated": true, "original_reference": oldRef, "new_reference": taskKnowledgeReference(newPublication), "immutable_history_retained": true, "original_plan_id": plan["id"], "stale_start_status": 409, "business_runs": effects.runs, "vendor_calls": 0, "model_calls": 0, "native_tool_calls": 0, "customer_acceptance": false, "requests": requests}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "receipt.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
