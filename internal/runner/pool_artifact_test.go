package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func TestPoolArtifactCheckpointKeepsAssignmentAndOriginalOperation(t *testing.T) {
	ctx := context.Background()
	a := poolTestAssignment()
	a.Type = "command"
	basis := rc.ApplicationBuildBasis{ApplicationID: "application", VersionID: "version", VersionSHA256: strings.Repeat("a", 64),
		ScopeID: "scope", ScopeTreeOID: strings.Repeat("b", 40), SourceCommit: strings.Repeat("c", 40), ReceiptSHA256: strings.Repeat("d", 64)}
	a.Config = map[string]any{rc.ApplicationBuildConfigKey: basis}
	blob := []byte("actual checkpoint bytes")
	hash := digest(blob)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, hash), blob, 0600); err != nil {
		t.Fatal(err)
	}
	manifest, mh, err := rc.CanonicalManifest(rc.ArtifactManifest{SchemaVersion: 1, Kind: rc.StaticSiteKind, Files: []rc.ArtifactFile{{Path: "index.html", SizeBytes: int64(len(blob)), SHA256: hash}}})
	if err != nil {
		t.Fatal(err)
	}
	store := &artifactHTTPFixture{t: t, blobs: map[string][]byte{}, dropSealACK: true}
	requests := map[string]string{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/api/v1/pool-runner/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			t.Error("artifact request used private namespace")
			http.NotFound(w, r)
			return
		}
		relative := strings.TrimPrefix(r.URL.Path, prefix)
		if relative == "heartbeat" {
			_ = json.NewEncoder(w).Encode(map[string]bool{"cancel_requested": false})
			return
		}
		if relative != "results" {
			raw, decodeErr := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Suma-Assignment"))
			var identity cc.AssignmentRef
			if decodeErr != nil || json.Unmarshal(raw, &identity) != nil || identity != candidateRef(a) {
				t.Error("artifact request lost its exact assignment scope")
				http.Error(w, "assignment", 400)
				return
			}
			operation := r.Header.Get("X-Suma-Operation-ID")
			if previous := requests[relative]; operation == "" || previous != "" && previous != operation {
				t.Error("artifact retry lost its original operation")
				http.Error(w, "operation", 400)
				return
			}
			requests[relative] = operation
			if r.Method == http.MethodPost && r.Header.Get("Content-Type") != "application/json" {
				t.Error("artifact JSON request has the wrong content type")
				http.Error(w, "content type", 400)
				return
			}
		}
		r.URL.Path = "/api/v1/runner/" + relative
		store.serve(w, r)
	}))
	defer s.Close()
	client := &Client{BaseURL: s.URL, Token: "fixture-worker-token", Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}}
	if err = client.postArtifact(ctx, "artifact-uploads", a, "unscoped", nil, nil); err == nil {
		t.Fatal("unscoped client used a Pool artifact endpoint")
	}
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	if err = journal.Accept(a); err != nil {
		t.Fatal(err)
	}
	p := artifactTransfer{Assignment: a, Result: Result{Assignment: a, WorkspaceEvidence: map[string]any{"clean": true, "head_commit": basis.SourceCommit}},
		Manifest: manifest, ManifestSHA256: mh, ProfileID: "build", ProfileSHA256: strings.Repeat("e", 64), BlobDirectory: dir, ApplicationBuild: &basis}
	if err = journal.checkpointArtifact(p); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Client: client, Journal: journal, ID: a.WorkerID}
	if err = worker.resumeArtifacts(ctx, time.Second); err == nil || len(journal.pendingArtifacts()) != 1 {
		t.Fatalf("lost seal ACK discarded checkpoint: %v", err)
	}
	if err = worker.resumeArtifacts(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err = replayPool(ctx, client, journal); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.puts != 1 || store.seals != 2 || len(store.results) != 1 || len(journal.Pending()) != 0 || len(journal.pendingArtifacts()) != 0 {
		t.Fatal("original Pool artifact operation was not recovered")
	}
	proof := store.results[0].ArtifactEvidence
	if proof == nil || proof.ApplicationBuild == nil || *proof.ApplicationBuild != basis || proof.ManifestSHA256 != mh {
		t.Fatal("recovered result lost the original application proof")
	}
}
