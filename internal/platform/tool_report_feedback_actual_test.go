package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This opt-in journey serves production UI assets and the real HTTP/SQLite
// control plane. Its inventory is explicitly owned metadata, not a native CLI
// probe, vendor authentication, model result, Worker run or acceptance.
func TestToolReportFeedbackActualHTTPAndBrowser(t *testing.T) {
	if os.Getenv("SUMA_TOOL_REPORT_FEEDBACK_204_ACTUAL") != "1" {
		t.Skip("explicit isolated report-feedback journey required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(root, "tests", "integration", "tool-report-feedback", "driver.cjs")
	web := filepath.Join(root, ".cache", "tool-report-feedback-204", "web", "out")
	for _, file := range []string{driver, filepath.Join(web, "index.html")} {
		if info, e := os.Stat(file); e != nil || !info.Mode().IsRegular() {
			t.Fatal("prepared actual driver and isolated UI build required")
		}
	}
	runID := strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", "")
	base := filepath.Join(root, ".cache", "tool-report-feedback-204", "actual")
	if err = os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, runID)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("retain previous evidence; each actual run needs its own directory")
	}
	tmp := filepath.Join(dir, "tmp")
	if err = os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(key, tmp)
	}
	f := newFixture(t)
	t.Cleanup(func() {
		if _, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite")); e != nil {
			t.Errorf("preserve isolated SQLite state: %v", e)
		}
	})
	const workerID, localTool, commandID = "owned-report-worker204", "codex-jsonl", "owned-report-command204"
	profile := tc.Profile{CommandID: commandID, Adapter: "codex", Protocol: "codex-jsonl-v1", ProfileSHA256: digest("owned report204 configuration metadata"), ExecutableSHA256: digest("owned report204 native metadata, not an executable probe"), ToolVersion: "owned metadata204; not a vendor CLI"}
	unavailable := profile
	unavailable.CommandID = "owned-report-unconfigured204"
	unavailable.ProfileSHA256 = digest("owned report204 unconfigured metadata")
	items := []tc.Observation{{Profile: profile, Available: true}, {Profile: unavailable, Available: false, Reason: "missing_configuration"}}
	initial := ToolInventoryRequest{ProjectID: f.project, WorkerID: workerID, OperationID: newID(), Items: items}
	newReport := initial
	newReport.OperationID = newID()
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	actionKey := hex.EncodeToString(secret)
	type observation struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		Status  int    `json:"status"`
		Fixture bool   `json:"fixture"`
	}
	type actionFact struct {
		Action       string `json:"action"`
		OperationID  string `json:"operation_id,omitempty"`
		ObservedAt   string `json:"observed_at,omitempty"`
		BaselineAsOf string `json:"baseline_as_of,omitempty"`
	}
	var lock, actionLock sync.Mutex
	var observations []observation
	var actions []actionFact
	var lastAsOf string
	var origin, token, workerRecordID string
	var publicSession map[string]any
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// The fixture key and machine credential never reach the renderer, argv,
	// response evidence or logs. Only this helper makes authorized fixture writes.
	call := func(method, route string, body any, bearer string, want int) ([]byte, error) {
		raw, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		req, e := http.NewRequest(method, origin+"/api/v1"+route, bytes.NewReader(raw))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Fixture-Internal", actionKey)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		} else {
			req.AddCookie(f.cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		response, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer response.Body.Close()
		result, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if e != nil || response.StatusCode != want {
			return nil, fmt.Errorf("owned HTTP fixture returned status %d, expected %d", response.StatusCode, want)
		}
		return result, nil
	}
	app := (&Server{Service: f.s, Engine: f.e, WebDir: web}).Handler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/view" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"project_id": f.project, "worker_id": workerID, "local_tool": localTool, "command_id": commandID, "session": publicSession})
			return
		}
		if r.URL.Path == "/__fixture/action" {
			if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Suma-Fixture-Key")), []byte(actionKey)) != 1 {
				http.Error(w, "owned fixture action denied", http.StatusForbidden)
				return
			}
			var input struct {
				Action string `json:"action"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
				http.Error(w, "invalid owned fixture action", http.StatusBadRequest)
				return
			}
			actionLock.Lock()
			defer actionLock.Unlock()
			fact := actionFact{Action: input.Action}
			var raw []byte
			var e error
			switch input.Action {
			case "report_initial", "replay_initial", "report_new":
				request := initial
				if input.Action == "report_new" {
					request = newReport
					// Wait for a real DB tick after the latest returned baseline;
					// no clock mutation or fabricated source revision is involved.
					lock.Lock()
					baseline := lastAsOf
					lock.Unlock()
					fact.BaselineAsOf = baseline
					if before, parseErr := time.Parse(time.RFC3339Nano, baseline); parseErr == nil {
						deadline := time.Now().Add(2 * time.Second)
						for {
							clock, clockErr := f.s.toolInventoryClock(r.Context(), f.s.DB)
							if clockErr != nil || time.Now().After(deadline) {
								e = fmt.Errorf("owned database tick unavailable")
								break
							}
							if clock.After(before) {
								break
							}
							time.Sleep(time.Millisecond)
						}
					}
				}
				if e == nil {
					raw, e = call(http.MethodPost, "/runner/tool-inventory", request, token, http.StatusOK)
				}
				var receipt ToolInventoryReceipt
				if e == nil {
					e = json.Unmarshal(raw, &receipt)
				}
				if e == nil && !receipt.Acknowledged {
					e = fmt.Errorf("owned report was not acknowledged")
				}
				fact.OperationID, fact.ObservedAt = request.OperationID, receipt.ObservedAt
			case "revoke_worker":
				raw, e = call(http.MethodPatch, "/release-workers/"+workerRecordID, map[string]any{"project_id": f.project, "status": "revoked", "expected_version": 1, "idempotency_key": newID()}, "", http.StatusOK)
			default:
				http.Error(w, "unknown owned fixture action", http.StatusBadRequest)
				return
			}
			if e != nil {
				http.Error(w, "owned fixture action failed", http.StatusInternalServerError)
				return
			}
			actions = append(actions, fact)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"action": input.Action, "passed": true, "operation_id": fact.OperationID, "observed_at": fact.ObservedAt})
			return
		}
		recorder := &toolReportHTTPRecorder204{ResponseWriter: w, capture: r.Method == http.MethodGet && r.URL.Path == "/api/v1/tool-discovery"}
		app.ServeHTTP(recorder, r)
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			lock.Lock()
			defer lock.Unlock()
			observations = append(observations, observation{r.Method, r.URL.Path, recorder.status, subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Suma-Fixture-Internal")), []byte(actionKey)) == 1})
			var discovery ToolDiscovery
			if recorder.capture && recorder.status == http.StatusOK && json.Unmarshal(recorder.body.Bytes(), &discovery) == nil {
				lastAsOf = discovery.AsOf
			}
		}
	}))
	defer server.Close()
	origin = server.URL
	created, err := call(http.MethodPost, "/release-workers", map[string]any{"project_id": f.project, "worker_id": workerID, "profiles": []any{}, "deployment": false, "idempotency_key": newID()}, "", http.StatusCreated)
	var worker map[string]any
	if err != nil || json.Unmarshal(created, &worker) != nil {
		t.Fatal("owned machine registration failed")
	}
	token, _ = worker["token"].(string)
	workerRecordID, _ = worker["id"].(string)
	if token == "" || workerRecordID == "" {
		t.Fatal("owned machine credential absent")
	}
	session, err := call(http.MethodGet, "/auth/session", nil, "", http.StatusOK)
	var actualSession map[string]any
	if err != nil || json.Unmarshal(session, &actualSession) != nil {
		t.Fatal("actual public session unavailable")
	}
	publicSession = map[string]any{"user": actualSession["user"], "organization": actualSession["organization"]}
	businessCounts := func() map[string]int {
		counts := discoveryBusinessCounts(t, f)
		// Original report receipts, inventory and worker-revocation activity
		// are intended fixture effects, not task/profile development mutations.
		for _, table := range []string{"suma_operations", "suma_activity", "suma_tool_inventories"} {
			delete(counts, table)
		}
		return counts
	}
	before := businessCounts()
	input := map[string]any{"origin": origin, "project_id": f.project, "user_id": f.user.ID, "tenant_id": f.user.TenantID, "worker_id": workerID, "local_tool": localTool, "command_id": commandID, "cookie": map[string]string{"name": f.cookie.Name, "value": f.cookie.Value}, "action_key": actionKey, "directory": dir}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("SUMA_TOOL_REPORT_FEEDBACK_204_NODE")
	if node == "" {
		node = `C:\Program Files\nodejs\node.exe`
	}
	if !filepath.IsAbs(node) {
		t.Fatal("an absolute local Node executable is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, driver)
	// This single cross-platform fixture file sets only Windows' existing
	// HideWindow field when available; no process launcher or wrapper is added.
	attributes := &syscall.SysProcAttr{}
	if hidden := reflect.ValueOf(attributes).Elem().FieldByName("HideWindow"); hidden.IsValid() && hidden.CanSet() && hidden.Kind() == reflect.Bool {
		hidden.SetBool(true)
		command.SysProcAttr = attributes
	}
	var stdout, stderr bytes.Buffer
	command.Dir, command.Stdin, command.Stdout, command.Stderr = root, bytes.NewReader(encoded), &stdout, &stderr
	runErr := command.Run()
	redact := func(raw string) string {
		for _, value := range []string{actionKey, f.cookie.Value, token} {
			raw = strings.ReplaceAll(raw, value, "[private fixture value removed]")
		}
		return raw
	}
	for name, raw := range map[string]string{"driver.stdout.json": stdout.String(), "driver.stderr.txt": stderr.String()} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(redact(raw)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var result struct {
		Stage  string `json:"stage"`
		Passed bool   `json:"passed"`
		Groups map[string]struct {
			Passed bool `json:"passed"`
		} `json:"groups"`
	}
	if runErr != nil || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Stage != "T-TOOL-REPORT-FEEDBACK-204" || !result.Passed || !result.Groups["cn_new_report_and_exact_return"].Passed || !result.Groups["en_old_ack_cancel_and_revocation"].Passed {
		t.Fatalf("actual browser driver failed; isolated evidence retained at %s", dir)
	}
	actionLock.Lock()
	defer actionLock.Unlock()
	seen := map[string]int{}
	var initialAt, replayAt, nextAt, nextBaseline string
	for _, fact := range actions {
		seen[fact.Action]++
		switch fact.Action {
		case "report_initial":
			initialAt = fact.ObservedAt
		case "replay_initial":
			replayAt = fact.ObservedAt
		case "report_new":
			nextAt, nextBaseline = fact.ObservedAt, fact.BaselineAsOf
		}
	}
	_, e1 := time.Parse(time.RFC3339Nano, initialAt)
	next, e2 := time.Parse(time.RFC3339Nano, nextAt)
	baseline, e3 := time.Parse(time.RFC3339Nano, nextBaseline)
	if seen["report_initial"] == 0 || seen["replay_initial"] == 0 || seen["report_new"] == 0 || seen["revoke_worker"] != 1 || replayAt != initialAt || e1 != nil || e2 != nil || e3 != nil || !next.After(baseline) {
		t.Fatal("original receipt, actual new report or precise revocation evidence missing")
	}
	if !reflect.DeepEqual(before, businessCounts()) {
		t.Fatal("read-only feedback changed business state")
	}
	lock.Lock()
	requests := append([]observation(nil), observations...)
	lock.Unlock()
	deniedRead := false
	for _, request := range requests {
		// The original exact-profile matcher is a read-only POST, explicitly
		// excluded from business commands by BrowserCommandRequiresView.
		readOnlyMatch := request.Method == http.MethodPost && request.Path == "/api/v1/tool-profiles/matches"
		if !request.Fixture && request.Method != http.MethodGet && !readOnlyMatch {
			t.Fatal("renderer performed a write")
		}
		if request.Path == "/api/v1/tool-discovery" && request.Status == http.StatusForbidden {
			deniedRead = true
		}
	}
	if !deniedRead {
		t.Fatal("browser did not observe exact worker revocation")
	}
	receipt := map[string]any{"stage": result.Stage, "run_id": runID, "passed": true, "requests": requests, "actions": actions, "old_ack_observed_at_unchanged": true, "initial_report_observed_at": initialAt, "new_observed_at": nextAt, "new_report_after_baseline": true, "browser_writes": 0, "business_counts_unchanged": true, "driver_exit": 0, "fixture": "Actual HTTP/SQLite and owned machine bootstrap/report metadata; no native probe, vendor, model, Worker task, ToolProfile registration or enable", "private_values_in_argv_or_receipt": false}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil || os.WriteFile(filepath.Join(dir, "receipt.json"), raw, 0600) != nil {
		t.Fatal("preserve actual feedback receipt")
	}
	t.Logf("actual feedback HTTP/SQLite/browser receipt retained at %s; no vendor/model/Worker", dir)
}

type toolReportHTTPRecorder204 struct {
	http.ResponseWriter
	status  int
	capture bool
	body    bytes.Buffer
}

func (w *toolReportHTTPRecorder204) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *toolReportHTTPRecorder204) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.capture && w.body.Len()+len(raw) <= 1<<20 {
		_, _ = w.body.Write(raw)
	}
	return w.ResponseWriter.Write(raw)
}
