package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func TestArtifactProfileRejectsUnsafeOutputBeforeExecution(t *testing.T) {
	repo, base := gitFixture(t)
	exe, _ := os.Executable()
	raw, _ := json.Marshal(map[string]any{"executable": exe, "args": []string{"-test.run=TestArtifactProcessHelper", "--"}, "env": map[string]string{"SUMA_ARTIFACT_HELPER": "build"}, "artifact_output": "../outside"})
	var spec CommandSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "artifact-profile", NodeID: "build", Attempt: 1, Fence: 1, WorkerID: "builder", Type: "command", Command: []string{"build"}}
	r := Execute(context.Background(), a, ExecConfig{AllowExec: true, Repository: repo, Workspace: repo, BaseCommit: base, Commands: map[string]CommandSpec{"build": spec}})
	if r.ExitCode == 0 || !strings.Contains(r.Error, "artifact") {
		t.Fatalf("unsafe artifact profile executed: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(repo, ".cache/runner/worktrees", attemptKey(a))); !os.IsNotExist(err) {
		t.Fatal("unsafe profile prepared an attempt")
	}
}

func TestArtifactProcessHelper(t *testing.T) {
	mode := os.Getenv("SUMA_ARTIFACT_HELPER")
	if mode == "package_test" || mode == "package_test_mutate" {
		dir := os.Getenv("SUMA_ARTIFACT_DIR")
		b, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil || string(b) != "<html>real artifact</html>\n" {
			os.Exit(33)
		}
		b, err = os.ReadFile("file.txt")
		if err != nil || string(b) != "candidate\n" {
			os.Exit(34)
		}
		if mode == "package_test_mutate" {
			_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("mutated after test"), 0600)
		}
		_, _ = os.Stdout.WriteString("tested actual package at source C\n")
		os.Exit(0)
	}
	if mode != "build" {
		return
	}
	if counter := os.Getenv("SUMA_ARTIFACT_COUNTER"); counter != "" {
		f, e := os.OpenFile(counter, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			os.Exit(35)
		}
		_, _ = f.WriteString("executed\n")
		_ = f.Close()
	}
	if err := os.MkdirAll(".cache/release/site", 0700); err != nil {
		os.Exit(31)
	}
	if err := os.WriteFile(".cache/release/site/index.html", []byte("<html>real artifact</html>\n"), 0600); err != nil {
		os.Exit(32)
	}
	_, _ = os.Stdout.WriteString("built actual static file\n")
	os.Exit(0)
}

type artifactHTTPFixture struct {
	t                    *testing.T
	mu                   sync.Mutex
	manifest             releasecontract.ArtifactManifest
	blobs                map[string][]byte
	resolved             releasecontract.ArtifactBinding
	puts, uploads, seals int
	dropSealACK          bool
	results              []Result
}

func (f *artifactHTTPFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer fixture-worker-token" {
		http.Error(w, "auth", 403)
		return
	}
	var out any
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v1/runner/artifact-uploads":
		var req releasecontract.ArtifactUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			f.t.Error(err)
			http.Error(w, "json", 400)
			return
		}
		f.manifest = req.Manifest
		_, hash, err := releasecontract.CanonicalManifest(req.Manifest)
		if err != nil {
			f.t.Error(err)
			http.Error(w, "manifest", 400)
			return
		}
		f.uploads++
		missing := []string{}
		for _, file := range req.Manifest.Files {
			if _, ok := f.blobs[file.SHA256]; !ok {
				missing = append(missing, file.SHA256)
			}
		}
		out = releasecontract.ArtifactUploadState{UploadID: "upload-1", State: "receiving", ManifestSHA256: hash, MissingBlobs: missing}
	case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/v1/runner/artifact-uploads/upload-1/blobs/"):
		b, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Error(err)
		}
		hash := strings.TrimPrefix(r.URL.Path, "/api/v1/runner/artifact-uploads/upload-1/blobs/")
		if digest(b) != hash {
			f.t.Error("actual bytes hash mismatch")
			http.Error(w, "hash", 409)
			return
		}
		f.blobs[hash] = b
		f.puts++
		out = map[string]bool{"ok": true}
	case r.Method == "POST" && r.URL.Path == "/api/v1/runner/artifact-uploads/upload-1/seal":
		_, hash, err := releasecontract.CanonicalManifest(f.manifest)
		if err != nil {
			f.t.Error(err)
		}
		var total int64
		for _, file := range f.manifest.Files {
			b, ok := f.blobs[file.SHA256]
			if !ok || int64(len(b)) != file.SizeBytes {
				f.t.Error("seal before receiving actual bytes")
				http.Error(w, "missing", 409)
				return
			}
			total += file.SizeBytes
		}
		f.seals++
		out = releasecontract.ArtifactSealReceipt{ArtifactID: "artifact-1", ManifestSHA256: hash, State: "sealed", FileCount: len(f.manifest.Files), SizeBytes: total}
		if f.dropSealACK {
			f.dropSealACK = false
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
	case r.Method == "POST" && r.URL.Path == "/api/v1/runner/artifacts/resolve":
		var req releasecontract.ArtifactResolveRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.SourceNodeID != "build" || req.Assignment.NodeID != "package-test" {
			f.t.Error("wrong frozen input request")
			http.Error(w, "identity", 409)
			return
		}
		out = f.resolved
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/runner/artifacts/artifact-1/blobs/"):
		hash := strings.TrimPrefix(r.URL.Path, "/api/v1/runner/artifacts/artifact-1/blobs/")
		b, ok := f.blobs[hash]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
		return
	case r.Method == "POST" && r.URL.Path == "/api/v1/runner/results":
		var request struct {
			Result Result `json:"result"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			f.t.Error(err)
		}
		f.results = append(f.results, request.Result)
		out = map[string]bool{"ok": true}
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func TestArtifactUploadCrashResumesAfterLostSealACKWithoutRerunning(t *testing.T) {
	repo, base := gitFixture(t)
	exe, _ := os.Executable()
	counter := filepath.Join(t.TempDir(), "executions.txt")
	store := &artifactHTTPFixture{t: t, blobs: map[string][]byte{}, dropSealACK: true}
	server := httptest.NewServer(http.HandlerFunc(store.serve))
	defer server.Close()
	client := &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture-worker-token"}
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "artifact-resume", NodeID: "build", Attempt: 1, Fence: 1, WorkerID: "builder", Type: "command", Command: []string{"build"}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Artifacts: client, Journal: j, Commands: map[string]CommandSpec{"build": {Executable: exe, Args: []string{"-test.run=TestArtifactProcessHelper", "--"}, Env: map[string]string{"SUMA_ARTIFACT_HELPER": "build", "SUMA_ARTIFACT_COUNTER": counter}, ArtifactOutput: ".cache/release/site"}}}
	r := Execute(context.Background(), a, cfg)
	if !r.artifactPending || len(j.Pending()) != 0 {
		t.Fatalf("transfer not durably pending: %+v %+v", r, j.Pending())
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = Replay(context.Background(), client, j); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	count := len(store.results)
	var proof *releasecontract.ArtifactEvidence
	if count == 1 {
		proof = store.results[0].ArtifactEvidence
	}
	store.mu.Unlock()
	if count != 1 || proof == nil || proof.Kind != "build" {
		t.Fatalf("durable artifact was not resumed: results=%d proof=%+v", count, proof)
	}
	b, err := os.ReadFile(counter)
	if err != nil || string(b) != "executed\n" {
		t.Fatalf("build reran after restart: %q %v", b, err)
	}
	if len(j.Pending()) != 0 {
		t.Fatal("final result not acknowledged")
	}
}
func artifactResult(t *testing.T, r Result) *releasecontract.ArtifactEvidence {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		ArtifactEvidence *releasecontract.ArtifactEvidence `json:"artifact_evidence"`
	}
	if err = json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	return wire.ArtifactEvidence
}

func TestArtifactBuildUploadsActualBytesAndBindsSource(t *testing.T) {
	repo, base := gitFixture(t)
	exe, _ := os.Executable()
	store := &artifactHTTPFixture{t: t, blobs: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(store.serve))
	defer server.Close()
	client := &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture-worker-token"}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Artifacts: client, Commands: map[string]CommandSpec{"build": {Executable: exe, Args: []string{"-test.run=TestArtifactProcessHelper", "--"}, Env: map[string]string{"SUMA_ARTIFACT_HELPER": "build"}, ArtifactOutput: ".cache/release/site"}}}
	a := Assignment{RunID: "artifact-build", NodeID: "build", Attempt: 1, Fence: 1, WorkerID: "builder", Type: "command", Command: []string{"build"}}
	r := Execute(context.Background(), a, cfg)
	proof := artifactResult(t, r)
	if r.ExitCode != 0 || r.Error != "" || proof == nil {
		t.Fatalf("no real build artifact: %+v proof=%+v", r, proof)
	}
	_, profileHash, _ := artifactProfile("build", cfg.Commands["build"])
	if proof.Kind != "build" || proof.SourceCommit != base || proof.ProfileSHA256 != profileHash || proof.ArtifactID != "artifact-1" || store.puts != 1 || store.seals != 1 {
		t.Fatalf("wrong proof/bytes %+v puts=%d seals=%d", proof, store.puts, store.seals)
	}
	if got := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD")); got != base {
		t.Fatal("changed original checkout")
	}
}

func TestArtifactPackageTestUsesResolvedCommitAndRealBundle(t *testing.T) {
	repo, base := gitFixture(t)
	_ = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("candidate\n"), 0600)
	gitTest(t, repo, "add", "file.txt")
	gitTest(t, repo, "commit", "-m", "artifact fixture source C")
	candidate := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "checkout", "--detach", base)
	body := []byte("<html>real artifact</html>\n")
	manifest := releasecontract.ArtifactManifest{SchemaVersion: 1, Kind: releasecontract.StaticSiteKind, Files: []releasecontract.ArtifactFile{{Path: "index.html", SizeBytes: int64(len(body)), SHA256: digest(body)}}}
	manifest, mhash, _ := releasecontract.CanonicalManifest(manifest)
	store := &artifactHTTPFixture{t: t, blobs: map[string][]byte{digest(body): body}, resolved: releasecontract.ArtifactBinding{ArtifactID: "artifact-1", ManifestSHA256: mhash, SourceCommit: candidate, BuildRunID: "artifact-run", BuildNodeID: "build", BuildAttempt: 1, BuildFence: 1, Manifest: manifest}}
	server := httptest.NewServer(http.HandlerFunc(store.serve))
	defer server.Close()
	exe, _ := os.Executable()
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Artifacts: &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture-worker-token"}, Commands: map[string]CommandSpec{"verify": {Executable: exe, Args: []string{"-test.run=TestArtifactProcessHelper", "--"}, TestEvidence: true, ArtifactInput: true, Env: map[string]string{"SUMA_ARTIFACT_HELPER": "package_test"}}}}
	a := Assignment{RunID: "artifact-run", NodeID: "package-test", Attempt: 1, Fence: 1, WorkerID: "tester", Type: "test", Command: []string{"verify"}, Config: map[string]any{"artifact_from": "build"}}
	r := Execute(context.Background(), a, cfg)
	proof := artifactResult(t, r)
	if r.ExitCode != 0 || proof == nil || !strings.Contains(r.Output, "tested actual package") {
		t.Fatalf("actual package test failed: %+v proof=%+v", r, proof)
	}
	if proof.Kind != "package_test" || proof.SourceCommit != candidate || proof.ManifestSHA256 != mhash || r.WorkspaceEvidence["base_commit"] != candidate || r.WorkspaceEvidence["admin_base_commit"] != base {
		t.Fatalf("wrong tested commit/artifact %+v %+v", proof, r.WorkspaceEvidence)
	}
}

func TestArtifactBuildCandidatePinsActualSourceC(t *testing.T) {
	cfg, a, base, candidate := candidateFixture(t)
	exe, _ := os.Executable()
	cfg.Commands = map[string]CommandSpec{"build": {Executable: exe, Args: []string{"-test.run=TestArtifactProcessHelper", "--"}, Env: map[string]string{"SUMA_ARTIFACT_HELPER": "build"}, ArtifactOutput: ".cache/release/site"}}
	store := &artifactHTTPFixture{t: t, blobs: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(store.serve))
	defer server.Close()
	cfg.Artifacts = &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture-worker-token"}
	a.Type = "command"
	a.Command = []string{"build"}
	journalDir := t.TempDir()
	currentJournal, err := OpenJournal(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if currentJournal != nil {
			_ = currentJournal.Close()
		}
	})
	if err = currentJournal.Accept(a); err != nil {
		t.Fatal(err)
	}
	cfg.Journal = currentJournal
	r := Execute(context.Background(), a, cfg)
	proof := artifactResult(t, r)
	if r.ExitCode != 0 || r.Error != "" || r.OutcomeUnknown || r.artifactPending || r.candidatePending || proof == nil || proof.SourceCommit != candidate || r.WorkspaceEvidence["base_commit"] != candidate || r.WorkspaceEvidence["head_commit"] != candidate || r.WorkspaceEvidence["admin_base_commit"] != base {
		t.Fatalf("build did not preserve candidate C: %+v proof=%+v", r, proof)
	}
	if len(currentJournal.pendingArtifacts()) != 1 || len(currentJournal.pendingCandidates()) != 0 || r.WorkspaceEvidence["candidate_bundle"] != nil {
		t.Fatal("artifact build entered a second candidate transfer")
	}
	if err = currentJournal.Finish(r); err != nil {
		t.Fatal(err)
	}
	if err = currentJournal.Close(); err != nil {
		t.Fatal(err)
	}
	currentJournal, err = OpenJournal(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	pending := currentJournal.Pending()
	if len(pending) != 1 || pending[0].ExitCode != 0 || pending[0].Error != "" || pending[0].ArtifactEvidence == nil || *pending[0].ArtifactEvidence != *proof || len(currentJournal.pendingArtifacts()) != 0 || len(currentJournal.pendingCandidates()) != 0 {
		t.Fatalf("restart did not retain the completed artifact result: %+v", pending)
	}
	if err = Replay(context.Background(), cfg.Artifacts, currentJournal); err != nil {
		t.Fatal(err)
	}
	if len(currentJournal.Pending()) != 0 || len(store.results) != 1 || store.uploads != 1 || store.seals != 1 {
		t.Fatal("restart repeated artifact transfer or lost its result ACK")
	}
}

func TestArtifactCollectionRejectsHardlinksAndUnsafeNames(t *testing.T) {
	for _, name := range []string{"hardlink", "unsafe-name"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			first := filepath.Join(dir, "index.html")
			if err := os.WriteFile(first, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "hardlink":
				if err := os.Link(first, filepath.Join(dir, "copy.html")); err != nil {
					t.Fatal(err)
				}
			case "unsafe-name":
				if err := os.WriteFile(filepath.Join(dir, "bad name.html"), []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := collectArtifact(dir, ""); err == nil {
				t.Fatal("unsafe artifact collection accepted")
			}
		})
	}
}

func TestArtifactInputRejectsCorruptBytesAndMissingSourceBeforeProcess(t *testing.T) {
	for _, mode := range []string{"corrupt-blob", "missing-commit", "changed-input"} {
		t.Run(mode, func(t *testing.T) {
			repo, base := gitFixture(t)
			_ = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("candidate\n"), 0600)
			gitTest(t, repo, "add", "file.txt")
			gitTest(t, repo, "commit", "-m", "fixture C")
			candidate := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
			gitTest(t, repo, "checkout", "--detach", base)
			body := []byte("<html>real artifact</html>\n")
			m := releasecontract.ArtifactManifest{SchemaVersion: 1, Kind: releasecontract.StaticSiteKind, Files: []releasecontract.ArtifactFile{{Path: "index.html", SizeBytes: int64(len(body)), SHA256: digest(body)}}}
			m, hash, _ := releasecontract.CanonicalManifest(m)
			store := &artifactHTTPFixture{t: t, blobs: map[string][]byte{digest(body): body}, resolved: releasecontract.ArtifactBinding{ArtifactID: "artifact-1", ManifestSHA256: hash, SourceCommit: candidate, BuildRunID: "artifact-input-bad", BuildNodeID: "build", BuildAttempt: 1, BuildFence: 1, Manifest: m}}
			helper := "package_test"
			if mode == "corrupt-blob" {
				store.blobs[digest(body)] = []byte("corrupt")
			}
			if mode == "missing-commit" {
				store.resolved.SourceCommit = strings.Repeat("f", 40)
			}
			if mode == "changed-input" {
				helper = "package_test_mutate"
			}
			server := httptest.NewServer(http.HandlerFunc(store.serve))
			defer server.Close()
			exe, _ := os.Executable()
			cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Artifacts: &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture-worker-token"}, Commands: map[string]CommandSpec{"verify": {Executable: exe, Args: []string{"-test.run=TestArtifactProcessHelper", "--"}, TestEvidence: true, ArtifactInput: true, Env: map[string]string{"SUMA_ARTIFACT_HELPER": helper}}}}
			a := Assignment{RunID: "artifact-input-bad", NodeID: "package-test", Attempt: 1, Fence: 1, WorkerID: "tester", Type: "test", Command: []string{"verify"}, Config: map[string]any{"artifact_from": "build"}}
			r := Execute(context.Background(), a, cfg)
			if r.ExitCode == 0 || artifactResult(t, r) != nil || !strings.Contains(r.Error, "artifact") {
				t.Fatalf("bad immutable package accepted: %+v", r)
			}
		})
	}
}

func TestArtifactFinalStorageFailureRetainsDurableCheckpoint(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "artifact-save", NodeID: "build", Attempt: 1, Fence: 1, WorkerID: "builder"}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	p := artifactTransfer{Assignment: a, Result: Result{Assignment: a, ExitCode: 0}}
	if err = j.checkpointArtifact(p); err != nil {
		t.Fatal(err)
	}
	j.path = filepath.Join(t.TempDir(), "missing", "journal.json")
	if err = j.Finish(p.Result); err == nil {
		t.Fatal("expected storage failure")
	}
	if len(j.Pending()) != 0 || len(j.pendingArtifacts()) != 1 {
		t.Fatal("failed durable finish exposed final result or discarded upload checkpoint")
	}
}
