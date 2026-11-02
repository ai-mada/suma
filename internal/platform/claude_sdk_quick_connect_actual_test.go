package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// This opt-in journey joins the production desktop controller, actual HTTP and
// SQLite, a real Go Worker and the reviewed SDK asset closure. Its native CLI is
// an explicitly owned --version-only fixture: no SDK query or model is run.
func TestClaudeSDKQuickConnectActualOriginalReceiptWorkerAndPausedProfile(t *testing.T) {
	if os.Getenv("SUMA_CLAUDE_SDK_CONNECT_201_ACTUAL") != "1" {
		t.Skip("explicit isolated desktop/HTTP/Worker journey only")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the owned native fixture and this actual receipt target Windows")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, ".cache", "claude-sdk-connect-201")
	driver := filepath.Join(stage, "actual-driver.cjs")
	fixture, err := os.ReadFile(filepath.Join(root, "internal", "platform", "testdata", "claude-sdk-connect-201", "driver.cjs"))
	if err != nil || len(fixture) < 100 {
		t.Fatal("the versioned desktop actual-driver fixture is missing")
	}
	if existing, e := os.ReadFile(driver); os.IsNotExist(e) {
		if err = os.MkdirAll(stage, 0700); err != nil || os.WriteFile(driver, fixture, 0600) != nil {
			t.Fatal("stage the versioned actual driver")
		}
	} else if e != nil || !bytes.Equal(existing, fixture) {
		t.Fatal("retain and reconcile an existing actual driver that differs from the versioned fixture")
	}
	directoryName := "actual"
	if suffix := os.Getenv("SUMA_CLAUDE_SDK_CONNECT_201_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("retain the original evidence and use only the explicit final fixture suffix")
		}
		directoryName += "-" + suffix
	}
	dir := filepath.Join(stage, directoryName)
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("retain the previous actual fixture; never overwrite or reseed it")
	}
	for _, file := range []string{"actual-driver.cjs", filepath.Join("bin", "suma-control.exe"), filepath.Join("native", "claude.exe")} {
		if stat, e := os.Stat(filepath.Join(stage, file)); e != nil || !stat.Mode().IsRegular() || stat.Size() == 0 {
			t.Fatalf("prepare the owned actual input first: %s", file)
		}
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f, repository := desktopRunnerFixture(t)
	// Save independent source and state evidence before fixture cleanup. A Git
	// bundle preserves the complete unique test repository without copying .git.
	bundle := exec.Command("git", "bundle", "create", filepath.Join(dir, "owned-repository.bundle"), "--all")
	bundle.Dir = repository
	if output, e := bundle.CombinedOutput(); e != nil {
		t.Fatalf("preserve the owned repository: %v: %s", e, output)
	}
	t.Cleanup(func() {
		if _, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite")); e != nil {
			t.Errorf("preserve the owned SQLite fixture: %v", e)
		}
	})
	type observation struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
	}
	var lock sync.Mutex
	var observations []observation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &claudeSDKActualStatus{ResponseWriter: w}
		f.handler.ServeHTTP(recorder, r)
		lock.Lock()
		observations = append(observations, observation{r.Method, r.URL.Path, recorder.status})
		lock.Unlock()
	}))
	defer server.Close()
	input := map[string]any{
		"origin": server.URL, "project_id": f.project, "user_id": f.user.ID, "tenant_id": f.user.TenantID,
		"cookie":     f.cookie.Name + "=" + f.cookie.Value,
		"key_base64": base64.StdEncoding.EncodeToString(f.s.DesktopWorkspaceKey),
		"directory":  dir, "binary": filepath.Join(stage, "bin", "suma-control.exe"),
		"native_cli": filepath.Join(stage, "native", "claude.exe"),
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	// Fixture cookies and the local transport key cross only the owned stdin
	// pipe. Do not persist them in a command file or return them in the receipt.
	stdout, err := os.Create(filepath.Join(dir, "driver.stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(dir, "driver.stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	node := os.Getenv("SUMA_CLAUDE_SDK_CONNECT_201_NODE")
	if node == "" {
		node = `C:\Program Files\nodejs\node.exe`
	}
	if !filepath.IsAbs(node) {
		t.Fatal("the optional test Node executable must be an absolute local path")
	}
	command := exec.Command(node, driver)
	command.Dir, command.Stdin, command.Stdout, command.Stderr = root, bytes.NewReader(encoded), stdout, stderr
	runErr := command.Run()
	lock.Lock()
	requests := append([]observation(nil), observations...)
	lock.Unlock()
	raw, err := json.MarshalIndent(map[string]any{"requests": requests, "driver_exit_succeeded": runErr == nil, "scope": "Owned actual TCP HTTP/SQLite and desktop lifecycle; --version-only native fixture, no SDK query, vendor/model or business run"}, "", "  ")
	if err != nil || os.WriteFile(filepath.Join(dir, "http-observations.json"), raw, 0600) != nil {
		t.Fatal("preserve actual HTTP observations")
	}
	if runErr != nil {
		t.Fatalf("owned actual driver did not complete: %v; private evidence retained", runErr)
	}
	var facts struct {
		Passed            bool   `json:"passed"`
		WorkerID          string `json:"worker_id"`
		ProfileID         string `json:"profile_id"`
		OriginalRecovered bool   `json:"original_register_receipt_recovered"`
		NativeOnlyVersion bool   `json:"native_calls_version_only"`
		Stopped           bool   `json:"stopped"`
		ModelCalls        *int   `json:"model_calls"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if err != nil || json.Unmarshal(data, &facts) != nil || !facts.Passed || !facts.OriginalRecovered || !facts.NativeOnlyVersion || !facts.Stopped || facts.ModelCalls == nil || *facts.ModelCalls != 0 {
		t.Fatal("the actual public receipt is incomplete")
	}
	profile, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, facts.ProfileID)
	if err != nil || profile.WorkerID != facts.WorkerID || profile.State != "paused" || profile.Version != 1 || profile.Profile.Protocol != "claude-sdk-v1" {
		t.Fatal("actual profile is not the original paused SDK registration")
	}
	for table, expected := range map[string]int{"suma_release_workers": 1, "suma_tool_profiles": 1, "dag_runs": 0} {
		var count int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != expected {
			t.Fatalf("owned database count mismatch for %s: %d, expected %d", table, count, expected)
		}
	}
}

type claudeSDKActualStatus struct {
	http.ResponseWriter
	status int
}

func (w *claudeSDKActualStatus) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *claudeSDKActualStatus) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
