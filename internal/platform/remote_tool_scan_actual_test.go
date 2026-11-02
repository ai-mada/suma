package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"github.com/ai-mada/suma/internal/runner"
	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

// This package is deliberately owned test transport, not vendor software. It
// accepts only the original fixed version/help probes and records no secrets.
const ownedCodeBuddyScan205 = `const fs=require('node:fs'),path=require('node:path');
const args=process.argv.slice(2),home=process.env.HOME||'',audit=path.resolve(__dirname,'../../../probe-audit.jsonl');
const isolated=home.includes('tool-installation-receipts')&&path.resolve(home)===path.resolve(process.cwd());
const credentials=['SUMA_RUNNER_TOKEN','SUMA_AGENT_TOKEN','OPENAI_API_KEY','ANTHROPIC_API_KEY'].some(key=>!!process.env[key]);
fs.appendFileSync(audit,JSON.stringify({pid:process.pid,args,home_isolated:isolated,credentials_present:credentials})+'\n');
if(!isolated||credentials)process.exit(81);
if(args.length===1&&args[0]==='--version')console.log('owned CodeBuddy version/help fixture205');
else if(args.length===1&&args[0]==='--help')console.log('--input-format --output-format --verbose');
else process.exit(82);
`

// Root runs this once after the real production-component build is prepared.
// The actual Server/SQLite, continuous Worker, Journal, native Node probes and
// lost committed ACKs are real. No SDK/model/task/configuration write is run.
func TestRemoteToolScanActualWorkerHTTPAndBrowser(t *testing.T) {
	if os.Getenv("SUMA_REMOTE_TOOL_SCAN_205_ACTUAL") != "1" {
		t.Skip("explicit isolated remote installation scan journey required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("SUMA_REMOTE_TOOL_SCAN_205_NODE")
	if !filepath.IsAbs(node) {
		t.Fatal("explicit existing absolute Node executable required")
	}
	driver := filepath.Join(root, "tests", "integration", "remote-tool-scan", "driver.cjs")
	web := filepath.Join(root, ".cache", "remote-tool-scan-205", "web", "out")
	for _, file := range []string{node, driver, filepath.Join(web, "index.html")} {
		if info, e := os.Stat(file); e != nil || !info.Mode().IsRegular() {
			t.Fatal("prepared existing Node/driver/isolated build required")
		}
	}
	runID := strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", "")
	base := filepath.Join(root, ".cache", "remote-tool-scan-205", "actual")
	if err = os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, runID)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("retain prior actual evidence")
	}
	tmp := filepath.Join(dir, "tmp")
	if err = os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(key, tmp)
	}
	bin := filepath.Join(dir, "owned-tools")
	pkg := filepath.Join(bin, "node_modules", "@tencent-ai", "codebuddy-code")
	if err = os.MkdirAll(pkg, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"package.json": `{"name":"@tencent-ai/codebuddy-code","private":true,"bin":{"codebuddy":"cli.cjs"}}`, "cli.cjs": ownedCodeBuddyScan205} {
		if err = os.WriteFile(filepath.Join(pkg, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(node))
	t.Setenv("SUMA_AGENT_TOKEN", "owned-canary-never-forwarded205")
	t.Setenv("OPENAI_API_KEY", "owned-canary-no-model205")
	t.Setenv("ANTHROPIC_API_KEY", "owned-canary-no-model205")
	f := newFixture(t)
	t.Cleanup(func() {
		if _, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite")); e != nil {
			t.Errorf("preserve isolated SQLite state: %v", e)
		}
	})
	const workerID = "owned-installation-worker205"
	const localTool = "codebuddy-cli"
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	actionKey := hex.EncodeToString(secret)
	type requestFact struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		Status      int    `json:"status"`
		OperationID string `json:"operation_id,omitempty"`
		BodySHA256  string `json:"body_sha256,omitempty"`
		ScanID      string `json:"scan_id,omitempty"`
		Outcome     string `json:"outcome,omitempty"`
		AckLost     bool   `json:"ack_lost,omitempty"`
	}
	var mu sync.Mutex
	var facts []requestFact
	var createLost, resultLost bool
	var token, workerRecordID, origin string
	var publicSession map[string]any
	revocations := 0
	app := (&Server{Service: f.s, Engine: f.e, WebDir: web}).Handler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/view" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"project_id": f.project, "worker_id": workerID, "local_tool": localTool, "session": publicSession})
			return
		}
		if r.URL.Path == "/__fixture/action" {
			if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Suma-Fixture-Key")), []byte(actionKey)) != 1 {
				http.Error(w, "owned fixture action denied", 403)
				return
			}
			var input struct {
				Action string `json:"action"`
			}
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
			d.DisallowUnknownFields()
			if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.Action != "revoke_worker" {
				http.Error(w, "invalid fixture action", 400)
				return
			}
			replayDeadline := time.Now().Add(8 * time.Second)
			for {
				mu.Lock()
				replayed := 0
				for _, fact := range facts {
					if fact.Path == "/api/v1/runner/tool-installation-scans/results" && fact.Status == 200 && fact.Outcome == "completed" {
						replayed++
					}
				}
				mu.Unlock()
				if replayed >= 2 {
					break
				}
				if time.Now().After(replayDeadline) {
					http.Error(w, "original result acknowledgement not recovered", 500)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			body := map[string]any{"project_id": f.project, "status": "revoked", "expected_version": 1, "idempotency_key": newID()}
			raw, _ := json.Marshal(body)
			request, e := http.NewRequest(http.MethodPatch, origin+"/api/v1/release-workers/"+workerRecordID, bytes.NewReader(raw))
			if e != nil {
				http.Error(w, "fixture action unavailable", 500)
				return
			}
			request.AddCookie(f.cookie)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Suma-Expected-User", f.user.ID)
			request.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
			response, e := http.DefaultClient.Do(request)
			if e != nil {
				http.Error(w, "fixture action unavailable", 500)
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				http.Error(w, "fixture action rejected", 500)
				return
			}
			mu.Lock()
			revocations++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"passed": true, "action": input.Action})
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			app.ServeHTTP(w, r)
			return
		}
		var body map[string]any
		var raw []byte
		if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/tool-installation-scans" || r.URL.Path == "/api/v1/runner/tool-installation-scans/results") {
			raw, _ = io.ReadAll(io.LimitReader(r.Body, 128<<10))
			r.Body = io.NopCloser(bytes.NewReader(raw))
			_ = json.Unmarshal(raw, &body)
		}
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, r)
		fact := requestFact{Method: r.Method, Path: r.URL.Path, Status: recorder.Code}
		drop := false
		if body != nil {
			fact.OperationID, _ = body["idempotency_key"].(string)
			fact.BodySHA256 = digest(string(raw))
			fact.Outcome, _ = body["outcome"].(string)
			if ref, ok := body["scan"].(map[string]any); ok {
				fact.ScanID, _ = ref["scan_id"].(string)
			}
		}
		mu.Lock()
		if r.URL.Path == "/api/v1/tool-installation-scans" && r.Method == http.MethodPost && recorder.Code == 201 && !createLost {
			createLost = true
			drop = true
		}
		if r.URL.Path == "/api/v1/runner/tool-installation-scans/results" && recorder.Code == 200 && fact.Outcome == "completed" && !resultLost {
			resultLost = true
			drop = true
		}
		fact.AckLost = drop
		facts = append(facts, fact)
		mu.Unlock()
		if drop {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("actual HTTP ACK loss needs owned HTTP/1 connection")
				return
			}
			connection, buffer, e := hijacker.Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			defer connection.Close()
			_, _ = fmt.Fprintf(buffer, "HTTP/1.1 %d %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n{", recorder.Code, http.StatusText(recorder.Code), recorder.Body.Len())
			_ = buffer.Flush()
			return
		}
		for name, values := range recorder.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	}))
	defer server.Close()
	origin = server.URL
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(method, route string, body any) ([]byte, error) {
		raw, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		request, e := http.NewRequest(method, origin+"/api/v1"+route, bytes.NewReader(raw))
		if e != nil {
			return nil, e
		}
		request.AddCookie(f.cookie)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Suma-Expected-User", f.user.ID)
		request.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		response, e := client.Do(request)
		if e != nil {
			return nil, e
		}
		defer response.Body.Close()
		result, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if e != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("owned HTTP bootstrap status %d", response.StatusCode)
		}
		return result, nil
	}
	raw, err := call(http.MethodPost, "/release-workers", map[string]any{"project_id": f.project, "worker_id": workerID, "profiles": []any{}, "deployment": false, "idempotency_key": newID()})
	var registration map[string]any
	if err != nil || json.Unmarshal(raw, &registration) != nil {
		t.Fatal("original machine registration failed", err)
	}
	token, _ = registration["token"].(string)
	workerRecordID, _ = registration["id"].(string)
	if token == "" || workerRecordID == "" {
		t.Fatal("machine identity missing")
	}
	t.Setenv("SUMA_RUNNER_TOKEN", token)
	raw, err = call(http.MethodGet, "/auth/session", nil)
	var session map[string]any
	if err != nil || json.Unmarshal(raw, &session) != nil {
		t.Fatal("public session unavailable", err)
	}
	publicSession = map[string]any{"user": session["user"], "organization": session["organization"]}
	journal, err := runner.OpenMaintenanceJournal(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	workspace := filepath.Join(dir, "workspace")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	worker := &runner.Worker{ID: workerID, Client: &runner.Client{BaseURL: origin, ProjectID: f.project, Token: token, HTTP: client}, Journal: journal, AllowInstalledToolDiscovery: true, MaintenanceOnly: true, PollInterval: 100 * time.Millisecond, Exec: runner.ExecConfig{Workspace: workspace, Commands: map[string]runner.CommandSpec{}, Secrets: []string{token}}}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(workerCtx) }()
	var stop sync.Once
	stopWorker := func() {
		stop.Do(func() {
			cancelWorker()
			select {
			case e := <-workerDone:
				if e != nil && !errors.Is(e, context.Canceled) {
					t.Errorf("maintenance Worker exit: %v", e)
				}
			case <-time.After(8 * time.Second):
				t.Error("maintenance Worker failed to drain")
			}
		})
	}
	defer stopWorker()
	wait := func(name string, predicate func() bool) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if predicate() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("timed out waiting for " + name)
	}
	wait("fresh authenticated scan capability", func() bool {
		body, e := call(http.MethodGet, "/tool-installation-scans?project_id="+f.project+"&worker_id="+workerID, nil)
		var page ToolInstallationScanPage
		return e == nil && json.Unmarshal(body, &page) == nil && page.Maintenance.State == "supported" && page.Maintenance.InstalledDiscovery != nil && len(page.Items) == 0
	})
	input := map[string]any{"origin": origin, "project_id": f.project, "user_id": f.user.ID, "tenant_id": f.user.TenantID, "worker_id": workerID, "local_tool": localTool, "cookie": map[string]string{"name": f.cookie.Name, "value": f.cookie.Value}, "action_key": actionKey, "directory": dir}
	encoded, _ := json.Marshal(input)
	driverCtx, cancelDriver := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelDriver()
	command := exec.CommandContext(driverCtx, node, driver)
	attributes := &syscall.SysProcAttr{}
	if hidden := reflect.ValueOf(attributes).Elem().FieldByName("HideWindow"); hidden.IsValid() && hidden.CanSet() && hidden.Kind() == reflect.Bool {
		hidden.SetBool(true)
		command.SysProcAttr = attributes
	}
	var stdout, stderr bytes.Buffer
	command.Dir, command.Stdin, command.Stdout, command.Stderr = root, bytes.NewReader(encoded), &stdout, &stderr
	driverErr := command.Run()
	redact := func(value string) string {
		for _, secret := range []string{actionKey, token, f.cookie.Value} {
			value = strings.ReplaceAll(value, secret, "[private fixture value removed]")
		}
		return value
	}
	for name, value := range map[string]string{"driver.stdout.json": stdout.String(), "driver.stderr.txt": stderr.String()} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(redact(value)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var result struct {
		Stage  string `json:"stage"`
		Passed bool   `json:"passed"`
		ScanID string `json:"scan_id"`
		Groups map[string]struct {
			Passed bool `json:"passed"`
		} `json:"groups"`
	}
	if driverErr != nil || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Stage != "T-REMOTE-TOOL-SCAN-205" || !result.Passed || !result.Groups["cn_recover_and_prepare"].Passed || !result.Groups["en_history_and_revocation"].Passed {
		t.Fatalf("actual driver failed; private evidence retained at %s", dir)
	}
	// The browser may see committed completion before the Worker recovers its
	// lost ACK. Wait for the same exact original result intent, never re-probe.
	wait("original result ACK replay", func() bool {
		mu.Lock()
		defer mu.Unlock()
		count := 0
		for _, fact := range facts {
			if fact.Path == "/api/v1/runner/tool-installation-scans/results" && fact.Status == 200 && fact.Outcome == "completed" {
				count++
			}
		}
		return count >= 2
	})
	stopWorker()
	var scanRow toolInstallationRow
	var payload string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_installation_scans WHERE id=?`, result.ScanID).Scan(&payload); err != nil || json.Unmarshal([]byte(payload), &scanRow) != nil || tii.ValidateScan(scanRow.Value) != nil {
		t.Fatal("original persisted installation result unavailable", err)
	}
	scan := scanRow.Value
	if scan.State != "completed" || len(scan.Result.Items) != 1 || scan.Result.Items[0].AdapterID != localTool || !scan.Result.Items[0].Installed || !scan.Result.Items[0].LaunchSupported || scan.Result.StopProof.NativeProcessCount != 2 || scan.Result.StopProof.StoppedProcessCount != 2 {
		t.Fatal("actual installation/probe closure evidence differs")
	}
	auditRaw, err := os.ReadFile(filepath.Join(bin, "probe-audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(auditRaw)), "\n")
	if len(lines) != 2 {
		t.Fatal("actual scan was repeated or did not run version/help exactly once")
	}
	probeKinds := map[string]int{}
	for _, line := range lines {
		var fact struct {
			PID                int      `json:"pid"`
			Args               []string `json:"args"`
			HomeIsolated       bool     `json:"home_isolated"`
			CredentialsPresent bool     `json:"credentials_present"`
		}
		if json.Unmarshal([]byte(line), &fact) != nil || fact.PID < 1 || len(fact.Args) != 1 || !fact.HomeIsolated || fact.CredentialsPresent {
			t.Fatal("native probe did not preserve isolation")
		}
		probeKinds[fact.Args[0]]++
	}
	if probeKinds["--version"] != 1 || probeKinds["--help"] != 1 {
		t.Fatal("unexpected native operation")
	}
	counts := map[string]int{}
	for _, table := range []string{"suma_tool_installation_scans", "suma_tool_profiles", "suma_tasks", "dag_runs"} {
		var count int
		if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	if counts["suma_tool_installation_scans"] != 1 || counts["suma_tool_profiles"] != 0 || counts["suma_tasks"] != 0 || counts["dag_runs"] != 0 {
		t.Fatal("scan created execution/configuration effects", counts)
	}
	mu.Lock()
	requests := append([]requestFact(nil), facts...)
	revoked := revocations
	lostCreate, lostResult := createLost, resultLost
	mu.Unlock()
	sameIntent := func(route string) bool {
		key, hash := "", ""
		n := 0
		for _, fact := range requests {
			if fact.Path != route || fact.OperationID == "" {
				continue
			}
			if key == "" {
				key, hash = fact.OperationID, fact.BodySHA256
			}
			if key != fact.OperationID || hash != fact.BodySHA256 {
				return false
			}
			n++
		}
		return n >= 2
	}
	if !lostCreate || !lostResult || !sameIntent("/api/v1/tool-installation-scans") || !sameIntent("/api/v1/runner/tool-installation-scans/results") || revoked != 1 {
		t.Fatal("committed same-intent ACK recovery was not observed")
	}
	receipt := map[string]any{"stage": result.Stage, "run_id": runID, "passed": true, "requests": requests, "counts": counts, "scan_id": scan.ID, "native_probe_kinds": probeKinds, "native_process_trees": 2, "all_native_trees_stopped": true, "home_isolated": true, "credentials_forwarded": false, "browser_create_ack_lost_and_recovered_same_body_key": true, "machine_result_ack_lost_and_recovered_same_body_key": true, "worker_count": 1, "worker_stopped": true, "revocations": revoked, "driver_exit": 0, "fixture": "Real HTTP/SQLite/Worker/Journal, owned Node version/help package and committed ACK loss; no vendor/model/SDK/task/profile enable or production acceptance", "private_values_in_argv_or_receipt": false}
	evidence, _ := json.MarshalIndent(receipt, "", "  ")
	if err = os.WriteFile(filepath.Join(dir, "receipt.json"), evidence, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual remote scan receipt retained at %s", dir)
}
