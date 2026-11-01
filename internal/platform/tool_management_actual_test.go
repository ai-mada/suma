package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The opt-in journey uses actual HTTP, SQLite and the compiled Suma CLI.
// Inventory and active-run rows are explicitly owned metadata fixtures; no
// vendor, Runner, agent, model or native development-tool process is started or declared healthy.
func TestToolManagementActualHTTPAndCLI(t *testing.T) {
	if os.Getenv("SUMA_TOOL_MANAGEMENT_193_ACTUAL") != "1" {
		t.Skip("explicit isolated management journey required")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, e := os.Stat(filepath.Join(root, "go.mod")); e == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("repository missing")
		}
		root = next
	}
	dir := filepath.Join(root, ".cache", "tool-management-193", "actual")
	if _, e := os.Stat(filepath.Join(dir, "receipt.json")); !os.IsNotExist(e) {
		t.Fatal("prior actual receipt retained; never reseed")
	}
	tmp := filepath.Join(dir, "tmp")
	if err = os.MkdirAll(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(key, tmp)
	}
	executable := os.Getenv("SUMA_TOOL_MANAGEMENT_193_CLI")
	if !filepath.IsAbs(executable) {
		t.Fatal("exact compiled CLI required")
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	f := newFixture(t)
	ctx := context.Background()
	var mu sync.Mutex
	requests := []map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, map[string]string{"method": r.Method, "path": r.URL.Path, "query": r.URL.RawQuery, "expected_user": r.Header.Get("X-Suma-Expected-User"), "expected_organization": r.Header.Get("X-Suma-Expected-Organization")})
		mu.Unlock()
		f.handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	call := func(method, route string, body any, want int, bearer string) map[string]any {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req, e := http.NewRequest(method, server.URL+"/api/v1"+route, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "en-US")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		} else {
			req.AddCookie(f.cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		response, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s status=%d want=%d: %s", method, route, response.StatusCode, want, data)
		}
		var out map[string]any
		if json.Unmarshal(data, &out) != nil {
			t.Fatal("non-JSON management response")
		}
		return out
	}
	worker := call("POST", "/release-workers", map[string]any{"project_id": f.project, "worker_id": "owned-tool-worker193", "profiles": []any{}, "deployment": false, "idempotency_key": newID()}, 201, "")
	profile := tc.Profile{CommandID: "owned-management-a193", Adapter: "command", Protocol: "text-v1", ProfileSHA256: digest("owned metadata source a193"), ExecutableSHA256: digest("not an observed native executable193"), ToolVersion: "owned metadata fixture; no native health probe"}
	created := call("POST", "/tool-profiles", ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "owned-tool-worker193", Name: "Owned metadata193", Profile: profile, IdempotencyKey: newID()}, 201, "")
	raw, _ := json.Marshal(created)
	var first ToolProfile
	if json.Unmarshal(raw, &first) != nil || first.State != "paused" {
		t.Fatal("new profile was not paused")
	}
	inventory := ToolInventoryRequest{ProjectID: f.project, WorkerID: first.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true, Reason: "Owned fixture report only; no native process/model"}}}
	call("POST", "/runner/tool-inventory", inventory, 200, worker["token"].(string))
	command := func(p ToolProfile, action string) ToolProfile {
		t.Helper()
		out := call("POST", "/tool-profiles/"+p.ID+"/commands", ToolProfileCommandRequest{ProjectID: f.project, Action: action, ExpectedVersion: p.Version, IdempotencyKey: newID()}, 200, "")
		raw, _ := json.Marshal(out)
		var next ToolProfile
		if json.Unmarshal(raw, &next) != nil {
			t.Fatal("profile result missing")
		}
		return next
	}
	first = command(first, "enable")
	if first.State != "enabled" {
		t.Fatal("original enable failed")
	}
	adjustedProfile := profile
	adjustedProfile.CommandID = "owned-management-b193"
	adjustedProfile.ProfileSHA256 = digest("owned changed metadata source b193")
	adjusted := call("POST", "/tool-profiles", ToolProfileCreateRequest{ProjectID: f.project, WorkerID: first.WorkerID, Name: "Owned metadata193 adjusted", Profile: adjustedProfile, IdempotencyKey: newID()}, 201, "")
	if adjusted["state"] != "paused" {
		t.Fatal("adjusted source implicitly enabled")
	}
	tx, e := f.s.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	insertImpactRun193(t, tx, f.user.TenantID, f.project, "owned-management-run193", "running")
	insertImpactNode193(t, tx, "owned-management-run193", "owned-node193", "running", first)
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	cliResults := []map[string]any{}
	cli := func(state string, wrongView bool) ToolProfileList {
		t.Helper()
		before := expertCount(t, f, "suma_operations")
		args := []string{"--control", server.URL, "--project", f.project, "--language", "en-US", "tools", "list", "--state", state, "--query", "Owned metadata193", "--limit", "2"}
		cmd := exec.Command(executable, args...)
		cmd.Dir = dir
		cmd.Env = []string{}
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			switch strings.ToLower(key) {
			case "path", "systemroot", "windir", "comspec", "pathext", "userprofile", "home", "appdata", "localappdata":
				cmd.Env = append(cmd.Env, entry)
			}
		}
		expected := f.user.ID
		if wrongView {
			expected = strings.Repeat("f", 32)
		}
		cmd.Env = append(cmd.Env, "TEMP="+tmp, "TMP="+tmp, "TMPDIR="+tmp, "SUMA_TOKEN="+f.cookie.Value, "SUMA_EXPECTED_USER="+expected, "SUMA_EXPECTED_ORGANIZATION="+f.user.TenantID)
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		e := cmd.Run()
		if bytes.Contains(out.Bytes(), []byte(f.cookie.Value)) || bytes.Contains(stderr.Bytes(), []byte(f.cookie.Value)) {
			t.Fatal("credential in CLI output")
		}
		if expertCount(t, f, "suma_operations") != before {
			t.Fatal("CLI read produced an operation")
		}
		if wrongView {
			if e == nil || out.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte("view_context_changed")) {
				t.Fatal("wrong CLI view accepted", e, stderr.String())
			}
			cliResults = append(cliResults, map[string]any{"state": state, "wrong_view_rejected": true, "operation_delta": 0})
			return ToolProfileList{}
		}
		if e != nil {
			t.Fatal("compiled CLI read failed", e, stderr.String())
		}
		var value ToolProfileList
		if json.Unmarshal(out.Bytes(), &value) != nil {
			t.Fatal("CLI output changed shape")
		}
		cliResults = append(cliResults, map[string]any{"state": state, "total": value.Total, "items": len(value.Items), "operation_delta": 0})
		return value
	}
	active := cli("active", false)
	if active.Total != 2 {
		t.Fatal("actual active query lost original or adjusted source", active.Total)
	}
	diagnostic := call("GET", "/tool-profiles/"+first.ID+"/diagnostics?project_id="+url.QueryEscape(f.project), nil, 200, "")
	raw, _ = json.Marshal(diagnostic)
	var observed ToolProfileDiagnostics
	if json.Unmarshal(raw, &observed) != nil || observed.Impact == nil || observed.Impact.Coverage != "complete" || observed.Impact.ObservedReferenceCount != 1 {
		t.Fatal("actual impact source did not match", string(raw))
	}
	first = command(first, "pause")
	first = command(first, "revoke")
	if first.State != "revoked" || !tc.EqualProfile(first.Profile, profile) {
		t.Fatal("removal replaced immutable source")
	}
	if remaining := cli("active", false); remaining.Total != 1 || remaining.Items[0].ID == first.ID {
		t.Fatal("removed connection stayed in active view")
	}
	history := cli("revoked", false)
	if history.Total != 1 || history.Items[0].ID != first.ID {
		t.Fatal("history lost original connection")
	}
	cli("active", true)
	detail := call("GET", "/tool-profiles/"+first.ID+"?project_id="+url.QueryEscape(f.project), nil, 200, "")
	if detail["state"] != "revoked" {
		t.Fatal("historical detail lost")
	}
	var runState string
	if e = f.s.DB.QueryRow(`SELECT state FROM dag_runs WHERE id=?`, "owned-management-run193").Scan(&runState); e != nil || runState != "running" {
		t.Fatal("removal fabricated process/run stop", runState, e)
	}
	snapshot := filepath.Join(dir, "private.sqlite")
	if _, e = f.s.DB.ExecContext(ctx, "VACUUM INTO ?", snapshot); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	recorded := append([]map[string]string(nil), requests...)
	mu.Unlock()
	receipt := map[string]any{"stage": "T-TOOL-MANAGEMENT-193", "status": "actual_http_sqlite_compiled_cli_confirmed", "cli_sha256": hex.EncodeToString(sum[:]), "profile_id": first.ID, "adjusted_profile_id": adjusted["id"], "original_profile_sha256": profile.ProfileSHA256, "final_state": first.State, "run_state_retained": runState, "impact": observed.Impact, "cli": cliResults, "http_requests": recorded, "native_tool_processes": 0, "runner_processes": 0, "vendor_calls": 0, "model_calls": 0, "customer_acceptance": false, "boundary": "Real management HTTP/SQLite/compiled Suma CLI only. Inventory availability and active-run rows are explicit owned metadata fixtures, not native tool health, scheduler dispatch or proof of external process stopping."}
	data, _ := json.MarshalIndent(receipt, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "receipt.json"), data, 0600); e != nil {
		t.Fatal(e)
	}
}
