package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// A real HTTP/SQLite preparation journey with an owned authenticated inventory
// fixture. No tool process, vendor, model, deployment or acceptance is executed.
func TestToolProfileReplacementActualAtomicHTTPHistoryAndReceipt(t *testing.T) {
	if os.Getenv("SUMA_TOOL_REPLACEMENT_200_ACTUAL") != "1" {
		t.Skip("explicit isolated HTTP/SQLite journey only")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	evidence := "actual"
	if suffix := os.Getenv("SUMA_TOOL_REPLACEMENT_200_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("preserve prior evidence and use the explicit final suffix")
		}
		evidence += "-" + suffix
	}
	dir := filepath.Join(root, ".cache", "tool-replacement-200", evidence)
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("never reseed or overwrite the original actual directory")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	snapshot := filepath.Join(dir, "private.sqlite")
	t.Cleanup(func() {
		if _, e := os.Stat(snapshot); os.IsNotExist(e) {
			if _, e = f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", snapshot); e != nil {
				t.Errorf("preserve owned database: %v", e)
			}
		}
	})
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	type observation struct {
		Method, Path string
		Status       int
	}
	var observations []observation
	call := func(method, path string, body any, token string, status int, out any) []byte {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, e := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		if token == "" {
			req.AddCookie(f.cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		if e != nil {
			t.Fatal(e)
		}
		observations = append(observations, observation{method, path, response.StatusCode})
		partial, _ := json.MarshalIndent(map[string]any{"status": "in_progress_or_interrupted", "requests": observations, "native_processes": 0, "model_calls": 0}, "", "  ")
		if e = os.WriteFile(filepath.Join(dir, "partial.json"), partial, 0600); e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != status {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, response.StatusCode, status, data)
		}
		if out != nil && json.Unmarshal(data, out) != nil {
			t.Fatal("invalid actual HTTP response")
		}
		return data
	}
	profiles := []tc.Profile{toolTestProfile(), toolTestProfile(), toolTestProfile()}
	profiles[1].CommandID, profiles[2].CommandID = "replacement-b", "replacement-c"
	tokens := make([]string, 2)
	workers := []string{"replacement-old-worker", "replacement-target-worker"}
	for index, worker := range workers {
		var issued struct {
			Token string `json:"token"`
		}
		call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), "", 201, &issued)
		tokens[index] = issued.Token
		call("POST", "/runner/tool-inventory", ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: profiles[index], Available: true}}}, issued.Token, 200, nil)
	}
	var original ToolProfile
	call("POST", "/tool-profiles", ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workers[0], Name: "Owned original configuration", Profile: profiles[0], IdempotencyKey: newID()}, "", 201, &original)
	command := func(action string) {
		t.Helper()
		call("POST", "/tool-profiles/"+original.ID+"/commands", ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: original.Version, Action: action, IdempotencyKey: newID()}, "", 200, &original)
	}
	command("enable")
	oldID, oldHash := original.ID, original.Profile.ProfileSHA256
	endpoint := "/tool-profiles/" + oldID + "/replacements"
	create := ToolProfileReplacementRequest{ProjectID: f.project, ExpectedVersion: original.Version, WorkerID: workers[1], Profile: profiles[1], Name: "Owned prepared target", PauseOriginal: false, IdempotencyKey: newID()}
	var prepared ToolProfileReplacement
	call("POST", endpoint, create, "", 201, &prepared)
	if !ToolProfileReplacementMatches(prepared, create, oldID) || prepared.CreatedBy != f.user.ID || prepared.Original.State != "enabled" || prepared.Disposition != "created" {
		t.Fatal("preparation changed original or failed exact snapshot binding", prepared)
	}
	beforeProfiles := expertCount(t, f, "suma_tool_profiles")
	reuse := ToolProfileReplacementRequest{ProjectID: f.project, ExpectedVersion: original.Version, WorkerID: workers[1], Profile: profiles[1], PauseOriginal: true, Reuse: &ToolProfileReplacementReuse{ProfileID: prepared.Replacement.ID, ExpectedVersion: prepared.Replacement.Version}, IdempotencyKey: newID()}
	var reused ToolProfileReplacement
	first := call("POST", endpoint, reuse, "", 201, &reused)
	if !ToolProfileReplacementMatches(reused, reuse, oldID) || reused.Disposition != "reused" || reused.Original.Version != original.Version+1 || reused.Original.Profile.ProfileSHA256 != oldHash || expertCount(t, f, "suma_tool_profiles") != beforeProfiles {
		t.Fatal("explicit reuse created a duplicate or altered immutable original configuration", reused)
	}
	original = reused.Original
	call("POST", endpoint, reuse, tokens[0], 403, nil)
	changed := reuse
	changed.PauseOriginal = false
	call("POST", endpoint, changed, "", 409, nil)
	command("enable")
	call("POST", "/runner/tool-inventory", ToolInventoryRequest{ProjectID: f.project, WorkerID: workers[1], OperationID: newID(), Items: []tc.Observation{{Profile: profiles[1], Available: true}, {Profile: profiles[2], Available: true}}}, tokens[1], 200, nil)
	withFault := ToolProfileReplacementRequest{ProjectID: f.project, ExpectedVersion: original.Version, WorkerID: workers[1], Profile: profiles[2], Name: "Owned rollback target", PauseOriginal: true, IdempotencyKey: newID()}
	before := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
	if _, err = f.s.DB.Exec(`CREATE TRIGGER owned_handoff_atomic200 BEFORE INSERT ON suma_tool_profile_replacements BEGIN SELECT RAISE(ABORT,'owned relationship interruption'); END`); err != nil {
		t.Fatal(err)
	}
	call("POST", endpoint, withFault, "", 500, nil)
	after := []int{expertCount(t, f, "suma_tool_profiles"), expertCount(t, f, "suma_tool_profile_replacements"), expertCount(t, f, "suma_operations")}
	if stringify(before) != stringify(after) {
		t.Fatal("failed relationship insert left partial profile, pause or receipt effects")
	}
	var unchanged ToolProfile
	call("GET", "/tool-profiles/"+oldID+"?project_id="+f.project, nil, "", 200, &unchanged)
	if unchanged.State != "enabled" || unchanged.Version != original.Version {
		t.Fatal("failed transaction paused the original")
	}
	if _, err = f.s.DB.Exec(`DROP TRIGGER owned_handoff_atomic200`); err != nil {
		t.Fatal(err)
	}
	var recovered ToolProfileReplacement
	committed := call("POST", endpoint, withFault, "", 201, &recovered)
	if !ToolProfileReplacementMatches(recovered, withFault, oldID) || recovered.Original.Profile.ProfileSHA256 != oldHash {
		t.Fatal("same intent recovery lost the original immutable profile")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	if replay := call("POST", endpoint, withFault, "", 201, nil); !bytes.Equal(committed, replay) {
		t.Fatal("committed handoff recomputed snapshots after observation expiry")
	}
	if replay := call("POST", endpoint, reuse, "", 201, nil); !bytes.Equal(first, replay) {
		t.Fatal("old committed relation was rewritten after subsequent profile changes")
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 3; page++ {
		var history ToolProfileReplacementList
		path := endpoint + "?project_id=" + f.project + "&direction=outgoing&limit=1"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		call("GET", path, nil, "", 200, &history)
		if history.ProjectID != f.project || history.ProfileID != oldID || history.Direction != "outgoing" || len(history.Items) != 1 || !ValidToolProfileReplacement(history.Items[0]) || seen[history.Items[0].ID] {
			t.Fatal("history mixed scope, duplicated or skipped its immutable relationships", history)
		}
		seen[history.Items[0].ID] = true
		cursor = history.NextCursor
	}
	if cursor != "" || len(seen) != 3 {
		t.Fatal("history did not finish its three committed relations")
	}
	var incoming ToolProfileReplacementList
	call("GET", "/tool-profiles/"+prepared.Replacement.ID+"/replacements?project_id="+f.project+"&direction=incoming", nil, "", 200, &incoming)
	if len(incoming.Items) != 2 || incoming.Direction != "incoming" {
		t.Fatal("target-side history lost creation/reuse relations")
	}
	var detail ToolProfileReplacement
	call("GET", "/tool-profile-replacements/"+recovered.ID+"?project_id="+f.project, nil, "", 200, &detail)
	if stringify(detail) != stringify(recovered) || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("refresh changed committed relation or started development")
	}
	if _, err = f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"status": "actual_http_sqlite_atomic_handoff_history_and_original_receipt_confirmed", "requests": observations, "prepared": prepared, "reused": reused, "recovered": recovered, "relation_count": len(seen), "rollback_verified": true, "worker_credential_receipt_replay_denied": true, "original_profile_hash_preserved": true, "freshness_expired_receipt_replay_exact": true, "native_processes": 0, "model_calls": 0, "business_runs": 0, "scope": "owned authenticated inventory fixture; no real tool execution, model, installation, deployment or customer acceptance", "created_at": time.Now().UTC().Format(time.RFC3339Nano)}
	raw, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	for _, token := range tokens {
		if strings.Contains(string(raw), token) {
			t.Fatal("private worker credential reached evidence")
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "receipt.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
