package deployer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func testDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type providerControlFixture struct {
	t                *testing.T
	mu               sync.Mutex
	plans            map[string]releasecontract.DeploymentAuthorizationGrant
	manifests        map[string]releasecontract.ArtifactManifest
	blobs            map[string][]byte
	consumes, begins int
	observations     []releasecontract.DeploymentObservation
	consumed         map[string]bool
	dropConsumeACK   bool
}

func (f *providerControlFixture) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer provider-control-secret" {
		http.Error(w, "auth", 403)
		return
	}
	var out any
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v1/provider/deployment-authorizations/consume":
		var req releasecontract.ConsumeDeploymentAuthorization
		_ = json.NewDecoder(r.Body).Decode(&req)
		grant, ok := f.plans[req.OperationID]
		if !ok || req.AuthorizationTicket != "ephemeral-ticket" || req.CallerWorkerID != "release-worker" || req.RequestSHA256 != grant.RequestSHA256 {
			http.Error(w, "invalid authorization", 403)
			return
		}
		f.consumes++
		if f.consumed == nil {
			f.consumed = map[string]bool{}
		}
		f.consumed[req.OperationID] = true
		out = grant
		if f.dropConsumeACK {
			f.dropConsumeACK = false
			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()
			return
		}
	case r.Method == "POST" && r.URL.Path == "/api/v1/provider/deployment-authorizations/inspect":
		var req releasecontract.InspectDeploymentAuthorization
		_ = json.NewDecoder(r.Body).Decode(&req)
		grant, ok := f.plans[req.OperationID]
		if !ok || !f.consumed[req.OperationID] || grant.RequestSHA256 != req.RequestSHA256 {
			http.NotFound(w, r)
			return
		}
		out = grant
	case r.Method == "POST" && r.URL.Path == "/api/v1/provider/deployment-authorizations/begin-effect":
		var req releasecontract.BeginDeploymentEffect
		_ = json.NewDecoder(r.Body).Decode(&req)
		grant, ok := f.plans[req.OperationID]
		if !ok || req.AuthorizationID != grant.AuthorizationID || req.Generation != grant.Plan.Generation {
			http.Error(w, "invalid effect", 409)
			return
		}
		f.begins++
		out = releasecontract.DeploymentEffectGrant{EffectGrantID: "effect-" + req.OperationID, OperationID: req.OperationID, Generation: req.Generation, State: "granted"}
	case r.Method == "POST" && r.URL.Path == "/api/v1/provider/deployment-observations":
		var obs releasecontract.DeploymentObservation
		_ = json.NewDecoder(r.Body).Decode(&obs)
		hash, _ := releasecontract.ObservationDigest(obs)
		if hash != obs.EvidenceSHA256 {
			f.t.Error("bad observation evidence")
		}
		f.observations = append(f.observations, obs)
		out = map[string]bool{"ok": true}
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/provider/artifacts/"):
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/provider/artifacts/"), "/")
		if len(parts) == 1 {
			m, ok := f.manifests[parts[0]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, hash, _ := releasecontract.CanonicalManifest(m)
			out = map[string]any{"id": parts[0], "manifest_sha256": hash, "manifest": m, "state": "sealed"}
		} else if len(parts) == 3 && parts[1] == "blobs" {
			body, ok := f.blobs[parts[2]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
			return
		} else {
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}
func (f *providerControlFixture) request(op, artifact string, generation int64, action, rollback string) releasecontract.DeploymentRequest {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	manifest := f.manifests[artifact]
	_, hash, _ := releasecontract.CanonicalManifest(manifest)
	ref := releasecontract.AssignmentRef{RunID: "run-" + op, NodeID: "deploy", Attempt: 1, Fence: 1, WorkerID: "release-worker"}
	plan := releasecontract.ReleasePlanContent{CandidateID: "candidate-" + artifact, Action: action, EnvironmentID: "environment-1", ProviderID: "static-provider", SiteID: "fixture-site", EnvironmentPolicyVersion: 1, ExpectedGeneration: generation - 1, Generation: generation, ArtifactID: artifact, ArtifactSHA256: hash, SourceCommit: strings.Repeat("a", 40), BuildAttestationID: "build-proof", PackageTestAttestationID: "package-proof", AcceptedTaskBasisSHA256: strings.Repeat("b", 64), ReservedExecution: releasecontract.ReservedExecution{RunID: ref.RunID, NodeID: ref.NodeID}, Health: releasecontract.HealthPolicy{Checks: []releasecontract.HealthCheck{{Path: "/", ExpectedStatus: 200, ExpectedFile: "index.html", ExpectedSHA256: manifest.Files[0].SHA256}}, TimeoutMS: 1000, Attempts: 1}, RollbackSourceOperationID: rollback}
	planHash, _ := releasecontract.CanonicalDigest(plan)
	execution := releasecontract.ReleaseExecution{Protocol: releasecontract.ReleaseProtocol, OperationID: op, PlanID: "plan-" + op, PlanRevision: 1, PlanSHA256: planHash, EnvironmentID: plan.EnvironmentID, ProviderID: plan.ProviderID, ExpectedGeneration: plan.ExpectedGeneration, Generation: plan.Generation, ArtifactID: artifact, ArtifactSHA256: hash}
	req := releasecontract.DeploymentRequest{ReleaseExecution: execution, Assignment: ref, AuthorizationTicket: "ephemeral-ticket"}
	requestHash, _ := releasecontract.DeploymentRequestDigest(req)
	f.plans[op] = releasecontract.DeploymentAuthorizationGrant{AuthorizationID: "authorization-" + op, OperationID: op, ProviderID: plan.ProviderID, Assignment: ref, Plan: plan, RequestSHA256: requestHash, State: "authorized"}
	return req
}
func providerFixture(t *testing.T) (*providerControlFixture, *httptest.Server, Config) {
	t.Helper()
	f := &providerControlFixture{t: t, plans: map[string]releasecontract.DeploymentAuthorizationGrant{}, manifests: map[string]releasecontract.ArtifactManifest{}, blobs: map[string][]byte{}}
	for _, name := range []string{"A", "B"} {
		body := []byte("<html>real static revision " + name + "</html>\n")
		hash := testDigest(body)
		f.blobs[hash] = body
		f.manifests["artifact-"+name] = releasecontract.ArtifactManifest{SchemaVersion: 1, Kind: releasecontract.StaticSiteKind, Files: []releasecontract.ArtifactFile{{Path: "index.html", SizeBytes: int64(len(body)), SHA256: hash}}}
	}
	control := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Setenv("SUMA_DEPLOY_TEST_CONTROL", "provider-control-secret")
	t.Setenv("SUMA_DEPLOY_TEST_WORKER", "worker-provider-secret")
	t.Setenv("SUMA_DEPLOY_TEST_RECONCILE", "reconciler-secret")
	return f, control, Config{ProviderID: "static-provider", Root: filepath.Join(t.TempDir(), "provider"), ControlURL: control.URL, ControlTokenEnv: "SUMA_DEPLOY_TEST_CONTROL", ReconcileTokenEnv: "SUMA_DEPLOY_TEST_RECONCILE", Workers: map[string]string{"release-worker": "SUMA_DEPLOY_TEST_WORKER"}, Sites: map[string]SiteConfig{"fixture-site": {Address: "127.0.0.1:0"}}}
}
func providerCall(t *testing.T, base, method, path, token string, body any) (int, releasecontract.DeploymentObservation) {
	t.Helper()
	var data io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		data = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, data)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var obs releasecontract.DeploymentObservation
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err = json.NewDecoder(resp.Body).Decode(&obs); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode, obs
}
func waitProvider(t *testing.T, base, operation string) releasecontract.DeploymentObservation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, obs := providerCall(t, base, "GET", "/v2/operations/"+operation, "reconciler-secret", nil)
		if status == 200 && (obs.State == "succeeded" || obs.State == "failed" || obs.State == "cancelled" || obs.State == "reconciling") {
			return obs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("provider operation did not settle")
	return releasecontract.DeploymentObservation{}
}

func TestLocalStaticProviderDeployHealthRollbackAndDurableRestart(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	for i, item := range []struct{ op, artifact, action, rollback string }{{"operation-deploy-A", "artifact-A", "deploy", ""}, {"operation-deploy-B", "artifact-B", "deploy", ""}, {"operation-rollback-A", "artifact-A", "rollback", "operation-deploy-A"}} {
		req := f.request(item.op, item.artifact, int64(i+1), item.action, item.rollback)
		status, _ := providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", req)
		if status != 202 && status != 200 {
			t.Fatalf("dispatch HTTP %d", status)
		}
		obs := waitProvider(t, management.URL, item.op)
		if obs.State != "succeeded" || obs.Effect.State != "applied" || obs.Health.State != "passed" || obs.FenceGeneration != int64(i+1) {
			t.Fatalf("real deployment did not pass health: %+v", obs)
		}
		resp, err := http.Get(server.SiteURL("fixture-site") + "/")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if testDigest(b) != f.manifests[item.artifact].Files[0].SHA256 {
			t.Fatalf("actual HTTP content is wrong: %s", b)
		}
		req.AuthorizationTicket = ""
		status, _ = providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", req)
		if status != 200 {
			t.Fatalf("idempotent POST %d", status)
		}
	}
	f.mu.Lock()
	if f.consumes != 3 || f.begins != 3 {
		t.Errorf("duplicate effects consumed=%d begins=%d", f.consumes, f.begins)
	}
	f.mu.Unlock()
	if err = server.Close(); err != nil {
		t.Fatal(err)
	}
	server, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	resp, err := http.Get(server.SiteURL("fixture-site") + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if testDigest(b) != f.manifests["artifact-A"].Files[0].SHA256 {
		t.Fatal("restart lost durable active revision")
	}
}

func TestLocalStaticProviderRejectsUnapprovedInputWithoutEffect(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	req := f.request("operation-forged-ticket", "artifact-A", 1, "deploy", "")
	req.AuthorizationTicket = "forged"
	status, _ := providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", req)
	if status != 202 && status != 403 {
		t.Fatalf("unexpected rejected dispatch HTTP %d", status)
	}
	if status == 202 {
		obs := waitProvider(t, management.URL, req.OperationID)
		if obs.State == "succeeded" || obs.Effect.State != "not_started" {
			t.Fatal("unapproved dispatch applied")
		}
	}
	resp, err := http.Get(server.SiteURL("fixture-site") + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatal("unapproved deployment activated site")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.begins != 0 {
		t.Fatal("forged ticket obtained effect grant")
	}
}

func TestLocalStaticProviderConcurrentDuplicateAndStaleGeneration(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	request := f.request("operation-parallel-A", "artifact-A", 1, "deploy", "")
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			status, _ := providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", request)
			if status != 200 && status != 202 {
				t.Errorf("concurrent dispatch HTTP %d", status)
			}
		}()
	}
	workers.Wait()
	obs := waitProvider(t, management.URL, request.OperationID)
	if obs.State != "succeeded" {
		t.Fatal(obs)
	}
	request = f.request("operation-stale-generation", "artifact-B", 1, "deploy", "")
	_, _ = providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", request)
	obs = waitProvider(t, management.URL, request.OperationID)
	if obs.State == "succeeded" || obs.Effect.State != "not_started" || obs.FenceGeneration != 1 {
		t.Fatalf("stale generation applied: %+v", obs)
	}
	resp, err := http.Get(server.SiteURL("fixture-site") + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if testDigest(body) != f.manifests["artifact-A"].Files[0].SHA256 {
		t.Fatal("stale generation overwrote A")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.begins != 1 {
		t.Fatalf("duplicate activation grants: %d", f.begins)
	}
}

func TestLocalStaticProviderCancellationRequiresObservedFence(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	started := make(chan struct{})
	var once sync.Once
	blocking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/") {
			once.Do(func() { close(started) })
			<-r.Context().Done()
			return
		}
		f.handler(w, r)
	}))
	defer blocking.Close()
	cfg.ControlURL = blocking.URL
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	req := f.request("operation-cancel-staging", "artifact-A", 1, "deploy", "")
	_, _ = providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", req)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("real artifact download did not start")
	}
	status, ack := providerCall(t, management.URL, "POST", "/v2/operations/"+req.OperationID+"/cancel", "worker-provider-secret", map[string]string{"operation_id": "cancel-staging-operation"})
	if status != 202 || ack.State == "cancelled" {
		t.Fatalf("request ACK invented a stopped observation: %+v", ack)
	}
	obs := waitProvider(t, management.URL, req.OperationID)
	if obs.State != "cancelled" || obs.Effect.State != "not_started" || obs.FenceGeneration != 1 {
		t.Fatalf("cancel did not persist fencing evidence: %+v", obs)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.begins != 0 {
		t.Fatal("cancelled staging obtained effect grant")
	}
}

func TestLocalStaticProviderWrongContent200FailsHealth(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	other := []byte("other actual page\n")
	f.blobs[testDigest(other)] = other
	m := f.manifests["artifact-A"]
	m.Files = append(m.Files, releasecontract.ArtifactFile{Path: "other.html", SizeBytes: int64(len(other)), SHA256: testDigest(other)})
	f.manifests["artifact-A"] = m
	request := f.request("operation-wrong-health-200", "artifact-A", 1, "deploy", "")
	f.mu.Lock()
	grant := f.plans[request.OperationID]
	grant.Plan.Health.Checks[0].Path = "/other.html"
	request.PlanSHA256, _ = releasecontract.CanonicalDigest(grant.Plan)
	grant.RequestSHA256, _ = releasecontract.DeploymentRequestDigest(request)
	f.plans[request.OperationID] = grant
	f.mu.Unlock()
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	_, _ = providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", request)
	obs := waitProvider(t, management.URL, request.OperationID)
	if obs.State != "failed" || obs.Effect.State != "applied" || obs.Health.State != "failed" || len(obs.Health.Checks) != 1 || obs.Health.Checks[0].StatusCode != 200 || obs.Health.Checks[0].BodySHA256 != testDigest(other) {
		t.Fatalf("wrong 200 passed health or lost applied fact: %+v", obs)
	}
}

func TestLocalStaticProviderActualProcessCrashAfterActiveCommitRecovers(t *testing.T) {
	providerCrashRecovery(t, "after-active")
}
func TestLocalStaticProviderConsumeACKLostAndActualProcessCrashRecovers(t *testing.T) {
	providerCrashRecovery(t, "after-consume")
}
func providerCrashRecovery(t *testing.T, phase string) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	f.dropConsumeACK = phase == "after-consume"
	request := f.request("operation-process-crash", "artifact-A", 1, "deploy", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg)
	command := exec.Command(exe, "-test.run=^TestProviderCrashProcessHelper$")
	command.Env = append(os.Environ(), "SUMA_DEPLOY_CRASH_HELPER=1", "SUMA_DEPLOY_CRASH_CONFIG="+string(raw), "SUMA_DEPLOY_CRASH_PHASE="+phase)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "READY ") {
				ready <- strings.TrimPrefix(line, "READY ")
				return
			}
		}
	}()
	var base string
	select {
	case base = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatalf("child provider not ready: %s", stderr.String())
	}
	_, _ = providerCall(t, base, "POST", "/v2/deployments", "worker-provider-secret", request)
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("provider did not crash at active boundary")
	}
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 87 {
		t.Fatalf("unexpected child exit %v %s", err, stderr.String())
	}
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	obs := waitProvider(t, management.URL, request.OperationID)
	if obs.State != "succeeded" || obs.Effect.ActiveGeneration != 1 {
		t.Fatalf("crash after real active commit not recovered: %+v", obs)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.consumes != 1 || f.begins != 1 {
		t.Fatalf("recovery repeated deployment authorization/effect: %d %d", f.consumes, f.begins)
	}
}

func TestProviderCrashProcessHelper(t *testing.T) {
	if os.Getenv("SUMA_DEPLOY_CRASH_HELPER") != "1" {
		return
	}
	var cfg Config
	if json.Unmarshal([]byte(os.Getenv("SUMA_DEPLOY_CRASH_CONFIG")), &cfg) != nil {
		os.Exit(81)
	}
	if os.Getenv("SUMA_DEPLOY_CRASH_PHASE") == "after-consume" {
		cfg.afterConsume = func() { os.Exit(87) }
	} else {
		cfg.afterActivate = func() { os.Exit(87) }
	}
	server, err := New(cfg)
	if err != nil {
		os.Exit(82)
	}
	defer server.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(83)
	}
	_, _ = os.Stdout.WriteString("READY http://" + listener.Addr().String() + "\n")
	_ = http.Serve(listener, server.Handler())
	os.Exit(84)
}

func TestLocalStaticProviderRequiresExplicitBearerAndExclusiveOwner(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if second, err := New(cfg); err == nil {
		_ = second.Close()
		t.Fatal("second provider acquired the same durable root")
	}
	management := httptest.NewServer(server.Handler())
	defer management.Close()
	request := f.request("operation-explicit-bearer", "artifact-A", 1, "deploy", "")
	_, _ = providerCall(t, management.URL, "POST", "/v2/deployments", "worker-provider-secret", request)
	_ = waitProvider(t, management.URL, request.OperationID)
	req, _ := http.NewRequest("GET", management.URL+"/v2/operations/"+request.OperationID, nil)
	req.Header.Set("Authorization", "reconciler-secret")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("missing Bearer scheme accepted: HTTP %d", response.StatusCode)
	}
}
