package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func TestRepositorySourceCandidateSealsExactApprovedBytes(t *testing.T) {
	repo, base := gitFixture(t)
	source := filepath.Join(t.TempDir(), "source")
	gitTest(t, repo, "clone", "--no-local", repo, source)
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, source, "add", "file.txt")
	gitTest(t, source, "commit", "-m", "approved repository source")
	head := strings.TrimSpace(gitTest(t, source, "rev-parse", "HEAD"))
	bundle := filepath.Join(t.TempDir(), "retained.bundle")
	gitTest(t, source, "bundle", "create", bundle, "HEAD", "^"+base)
	bytesApproved, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	approved := ri.RepositorySourceExecution{IntakeID: "intake-1", PlanID: "plan-1", TaskID: "task-1", TaskVersion: 4, BaseCommit: base, HeadCommit: head, BundleSHA256: digest(bytesApproved), SizeBytes: int64(len(bytesApproved))}
	a := Assignment{RunID: "source-run", NodeID: "source", WorkerID: "writer", Attempt: 1, Fence: 1, Type: "command", Command: []string{"candidate"}, Config: map[string]any{ri.RepositorySourceExecutionKey: approved}}
	fixture := &candidateFixtureServer{}
	candidate := fixture.serve(t)
	t.Cleanup(candidate.Close)
	var downloads atomic.Int32
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/runner/repository-source-intakes/intake-1/bundle" {
			candidate.Config.Handler.ServeHTTP(w, req)
			return
		}
		downloads.Add(1)
		var assignment cc.AssignmentRef
		encoded, err := base64.RawURLEncoding.DecodeString(req.Header.Get("X-Suma-Assignment"))
		if req.Method != http.MethodGet || req.URL.Query().Get("project_id") != "project" || req.Header.Get("Authorization") != "Bearer fixture" || err != nil || json.Unmarshal(encoded, &assignment) != nil || assignment != candidateRef(a) {
			t.Error("managed source download missed exact assignment authority")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(bytesApproved)))
		_, _ = w.Write(bytesApproved)
	}))
	t.Cleanup(control.Close)
	cfg := candidateConfig(t, repo, base, &Client{BaseURL: control.URL, Token: "fixture", ProjectID: "project"}, "write")
	counter := filepath.Join(t.TempDir(), "executed")
	cfg.Commands["candidate"].Env["SUMA_CANDIDATE_COUNTER"] = counter
	ctx := context.Background()
	cfg.ToolInventory, err = DescribeTools(ctx, cfg)
	if err != nil || len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available {
		t.Fatalf("managed source profile unavailable: %v", err)
	}
	a.Config["tool_profile_sha256"] = cfg.ToolInventory[0].Profile.ProfileSHA256
	r := Execute(ctx, a, cfg)
	if r.ExitCode != 0 || r.Error != "" || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed" || r.WorkspaceEvidence["clean"] != true || r.WorkspaceEvidence[ri.RepositorySourceExecutionKey] != approved || r.WorkspaceEvidence["head_commit"] != head {
		t.Fatalf("managed source execution failed: %+v", r)
	}
	fixture.mu.Lock()
	sealed := fixture.puts == 1 && fixture.binding.ValidSealed() && fixture.binding.BundleSHA256 == approved.BundleSHA256 && bytes.Equal(fixture.bundle, bytesApproved)
	fixture.mu.Unlock()
	if downloads.Load() != 1 || !sealed {
		t.Fatal("sealed candidate did not preserve the single approved bundle")
	}
	if _, err = os.Stat(counter); !os.IsNotExist(err) {
		t.Fatal("repository source candidate ran the selected shell command")
	}
	if _, err = gitText(ctx, repo, "rev-parse", "--verify", head+"^{commit}"); err == nil {
		t.Fatal("repository source import polluted the administrator repository")
	}
}
