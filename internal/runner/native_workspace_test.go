package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func nativeFixture(t *testing.T, checks []string, callback nativeWorkspaceCheck) (*nativeWorkspace, *attemptWorkspace, CommandSpec) {
	return nativeFixtureAssignment(t, Assignment{TenantID: "tenant", ProjectID: "project", RunID: "run", NodeID: "node", Attempt: 1, Fence: 2, WorkerID: "worker", Type: "agent"}, checks, callback)
}

func nativeFixtureAssignment(t *testing.T, a Assignment, checks []string, callback nativeWorkspaceCheck) (*nativeWorkspace, *attemptWorkspace, CommandSpec) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := &attemptWorkspace{AttemptKey: attemptKey(a), Assignment: a, Directory: dir, Source: src, State: "prepared"}
	var spec CommandSpec
	raw, _ := json.Marshal(map[string]any{"native_workspace": NativeWorkspacePolicy{Checks: checks}})
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	n, err := openNativeWorkspace(context.Background(), a, ExecConfig{}, w, spec, strings.Repeat("a", 64), callback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n, w, spec
}

func nativeCall(t *testing.T, n *nativeWorkspace, name, op string, args any) (any, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return n.invoke(context.Background(), name, raw, op)
}

func TestNativeWorkspaceCASReceiptsAndDeletion(t *testing.T) {
	n, w, _ := nativeFixture(t, nil, nil)
	old := digest([]byte("base\n"))
	args := map[string]any{"path": "file.txt", "content": "fixed\n", "expected_sha256": old}
	if _, err := nativeCall(t, n, "write", "write-one", args); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "write", "write-one", args); err != nil {
		t.Fatalf("lost ACK replay: %v", err)
	}
	if got := n.Evidence().Generation; got != 1 {
		t.Fatalf("replay generation %d", got)
	}
	args["content"] = "other\n"
	if _, err := nativeCall(t, n, "write", "write-one", args); err == nil {
		t.Fatal("changed operation accepted")
	}
	if _, err := nativeCall(t, n, "write", "stale", map[string]any{"path": "file.txt", "content": "stale", "expected_sha256": old}); err == nil {
		t.Fatal("stale CAS accepted")
	}
	if _, err := nativeCall(t, n, "write", "create", map[string]any{"path": "new/created.txt", "content": "new\n", "expected_absent": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "remove", "remove", map[string]any{"path": "new/created.txt", "expected_sha256": digest([]byte("new\n"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "remove", "remove", map[string]any{"path": "new/created.txt", "expected_sha256": digest([]byte("new\n"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.Source, "new", "created.txt")); !os.IsNotExist(err) {
		t.Fatal("remove did not remove one file")
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "read", "after-close", map[string]any{"path": "file.txt"}); err == nil {
		t.Fatal("closed scope served file")
	}
}

func TestNativeWorkspaceRejectsPathsAndStrictInput(t *testing.T) {
	n, _, _ := nativeFixture(t, nil, nil)
	for _, path := range []string{"../file.txt", "/file.txt", "C:/file.txt", "file.txt:stream", ".git/config", ".env", "dir/NUL", "dir/con.txt", "file.txt.", "file.txt ", "FILE.txt", "./file.txt"} {
		if _, err := nativeCall(t, n, "read", "bad", map[string]any{"path": path}); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	for _, raw := range []string{`{"path":"file.txt","path":"file.txt"}`, `{"path":null}`, `{"path":"file.txt","start_line":1e0}`, `{"path":"file.txt","start_line":1.0}`, `{"path":"file.txt","extra":true}`, `{"path":"\ud800"}`} {
		if _, err := n.invoke(context.Background(), "read", []byte(raw), "strict"); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestNativeWorkspaceRejectsGitExecutionPolicyFiles(t *testing.T) {
	for _, name := range []string{".gitattributes", ".gitmodules", "nested/.gitattributes", "nested/.GITMODULES"} {
		if _, err := nativeWorkspacePath(name); err == nil {
			t.Errorf("Git execution policy path accepted: %s", name)
		}
	}
	for _, name := range []string{".gitignore", "nested/.gitignore"} {
		if _, err := nativeWorkspacePath(name); err != nil {
			t.Errorf("ordinary ignore file rejected: %s", name)
		}
	}
}

func TestNativeWorkspaceChecksCurrentAndDetectMutation(t *testing.T) {
	calls := 0
	n, w, _ := nativeFixture(t, []string{"unit"}, func(context.Context, string) (NativeWorkspaceCheckResult, error) {
		calls++
		return NativeWorkspaceCheckResult{ProfileSHA256: strings.Repeat("b", 64), ExitCode: 0}, nil
	})
	if err := n.ReadyToSeal(); err == nil {
		t.Fatal("missing required check accepted")
	}
	if _, err := nativeCall(t, n, "check", "check-one", map[string]any{"check_id": "unit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "check", "check-one", map[string]any{"check_id": "unit"}); err != nil || calls != 1 {
		t.Fatalf("check repeated: %d %v", calls, err)
	}
	if err := n.ReadyToSeal(); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "write", "write", map[string]any{"path": "file.txt", "content": "next\n", "expected_sha256": digest([]byte("base\n"))}); err != nil {
		t.Fatal(err)
	}
	if err := n.ReadyToSeal(); err == nil {
		t.Fatal("stale check accepted")
	}
	if err := os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("outside tool\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "check", "check-two", map[string]any{"check_id": "unit"}); err == nil {
		t.Fatal("unaccepted source mutation accepted")
	}
}

func TestNativeWorkspaceReadPaginationAndLongLine(t *testing.T) {
	n, w, _ := nativeFixture(t, nil, nil)
	if err := os.WriteFile(filepath.Join(w.Source, "long.txt"), []byte(strings.Repeat("x", 65536)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "read", "long", map[string]any{"path": "long.txt"}); err == nil {
		t.Fatal("long line silently truncated")
	}
	if _, err := nativeCall(t, n, "read", "read", map[string]any{"path": "file.txt", "start_line": 1, "max_lines": 1}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeWorkspaceEditUsesOriginalUniqueNonOverlappingText(t *testing.T) {
	n, _, _ := nativeFixture(t, nil, nil)
	args := map[string]any{"path": "file.txt", "expected_sha256": digest([]byte("base\n")), "edits": []any{map[string]any{"old_text": "base", "new_text": "fixed"}}}
	if _, err := nativeCall(t, n, "edit", "edit", args); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "edit", "edit", args); err != nil {
		t.Fatalf("edit lost ACK reapplied patch: %v", err)
	}
	if n.Evidence().Generation != 1 {
		t.Fatal("edit replay advanced generation")
	}
	if _, err := nativeCall(t, n, "edit", "ambiguous", map[string]any{"path": "file.txt", "expected_sha256": digest([]byte("fixed\n")), "edits": []any{map[string]any{"old_text": "fixed", "new_text": "a"}, map[string]any{"old_text": "ixed", "new_text": "b"}}}); err == nil {
		t.Fatal("overlapping edits accepted")
	}
	if _, err := nativeCall(t, n, "edit", "missing", map[string]any{"path": "file.txt", "expected_sha256": digest([]byte("fixed\n")), "edits": []any{map[string]any{"old_text": "absent", "new_text": "x"}}}); err == nil {
		t.Fatal("missing original text accepted")
	}
}

func TestNativeWorkspacePreparedOperationsRecoverWithoutRepeatingChecks(t *testing.T) {
	for _, state := range []string{"old", "new", "third", "removed", "check"} {
		t.Run(state, func(t *testing.T) {
			n, w, spec := nativeFixture(t, nil, nil)
			op := &nativeWorkspaceOperation{ID: "pending", Kind: "write", RequestSHA: strings.Repeat("c", 64), State: "prepared", Path: "file.txt", PreviousSHA: digest([]byte("base\n")), SHA: digest([]byte("next\n")), Content: "next\n", Generation: 1}
			if state == "removed" {
				op.Kind = "remove"
				op.SHA = ""
				op.Content = ""
			}
			if state == "check" {
				op.Kind = "check"
				op.Generation = 0
			}
			n.mu.Lock()
			n.ledger.Operations[op.ID] = op
			if err := n.saveLedger(); err != nil {
				t.Fatal(err)
			}
			n.mu.Unlock()
			if err := n.Close(); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "new":
				_ = os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("next\n"), 0600)
			case "third":
				_ = os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("third\n"), 0600)
			case "removed":
				_ = os.Remove(filepath.Join(w.Source, "file.txt"))
			}
			reopened, err := openNativeWorkspace(context.Background(), w.Assignment, ExecConfig{}, w, spec, strings.Repeat("a", 64), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if state == "third" || state == "check" {
				if reopened.Evidence().State != "unknown" {
					t.Fatal("uncertain operation was retried")
				}
				if _, err := nativeCall(t, reopened, "write", "next-operation", map[string]any{"path": "other.txt", "content": "x", "expected_absent": true}); err == nil {
					t.Fatal("unknown did not block writes")
				}
				return
			}
			if reopened.Evidence().Generation != 1 || reopened.Evidence().State == "unknown" {
				t.Fatalf("recovery did not finish one operation: %+v", reopened.Evidence())
			}
		})
	}
}

type nativeTestBearer struct{ token string }

func (b nativeTestBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
func TestNativeWorkspaceMCPOfficialSDKAndAuthorization(t *testing.T) {
	n, _, _ := nativeFixture(t, nil, nil)
	bridge, err := n.StartMCP()
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			client := mcp.NewClient(&mcp.Implementation{Name: "native-test", Version: "1"}, nil)
			session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: bridge.Endpoint, HTTPClient: &http.Client{Transport: nativeTestBearer{bridge.Token}, Timeout: 5 * time.Second}}, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			if info := session.InitializeResult().ServerInfo; info.Name != "suma-native-workspace" || info.Version != NativeWorkspaceVersion {
				t.Fatalf("wrong identity: %+v", info)
			}
			list, err := session.ListTools(context.Background(), nil)
			if err != nil || len(list.Tools) != 6 || list.CacheScope != "private" || list.TTLMs != 0 {
				t.Fatalf("catalog/cache: %+v %v", list, err)
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_workspace_read", Arguments: map[string]any{"path": "file.txt"}})
			if err != nil || result.IsError {
				t.Fatalf("read: %+v %v", result, err)
			}
			result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_workspace_write", Arguments: map[string]any{"path": "new.txt", "content": "x", "expected_absent": true}})
			if err != nil || !result.IsError {
				t.Fatalf("missing hidden operation accepted: %+v %v", result, err)
			}
			result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Meta: mcp.Meta{"suma/operation_id": "mcp-" + protocol}, Name: "suma_workspace_write", Arguments: map[string]any{"path": "new-" + protocol + ".txt", "content": "x", "expected_absent": true}})
			if err != nil || result.IsError {
				t.Fatalf("write: %+v %v", result, err)
			}
		})
	}
	for _, condition := range []string{"missing", "origin", "cookie", "host"} {
		r, _ := http.NewRequest(http.MethodPost, bridge.Endpoint, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+bridge.Token)
		switch condition {
		case "missing":
			r.Header.Del("Authorization")
		case "origin":
			r.Header.Set("Origin", "http://example.invalid")
		case "cookie":
			r.Header.Set("Cookie", "session=untrusted")
		case "host":
			r.Host = "example.invalid"
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode < 400 {
			t.Errorf("accepted %s", condition)
		}
	}
}

func TestNativeWorkspaceCheckMutationAndCloseDrain(t *testing.T) {
	t.Run("mutation", func(t *testing.T) {
		var source string
		n, w, _ := nativeFixture(t, []string{"unit"}, func(context.Context, string) (NativeWorkspaceCheckResult, error) {
			_ = os.WriteFile(filepath.Join(source, "file.txt"), []byte("side effect"), 0600)
			return NativeWorkspaceCheckResult{ExitCode: 0, ProfileSHA256: strings.Repeat("b", 64)}, nil
		})
		source = w.Source
		if _, err := nativeCall(t, n, "check", "check", map[string]any{"check_id": "unit"}); err == nil {
			t.Fatal("check mutation accepted")
		}
		if n.Evidence().State != "unknown" {
			t.Fatal("mutation did not block sealing")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		started := make(chan struct{})
		returned := make(chan struct{})
		n, _, _ := nativeFixture(t, []string{"unit"}, func(ctx context.Context, _ string) (NativeWorkspaceCheckResult, error) {
			close(started)
			<-ctx.Done()
			close(returned)
			return NativeWorkspaceCheckResult{ExitCode: 1, Stopped: true, ProfileSHA256: strings.Repeat("b", 64)}, nil
		})
		done := make(chan struct{})
		go func() { _, _ = nativeCall(t, n, "check", "check", map[string]any{"check_id": "unit"}); close(done) }()
		<-started
		if err := n.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("close returned before check stopped")
		}
		<-done
	})
}

func TestNativeWorkspaceRejectsCursorAliasesAndCancelledSuccess(t *testing.T) {
	t.Run("cursor", func(t *testing.T) {
		n, _, _ := nativeFixture(t, nil, nil)
		_, err := nativeCall(t, n, "write", "create", map[string]any{"path": "z.txt", "content": "z", "expected_absent": true})
		if err != nil {
			t.Fatal(err)
		}
		result, err := nativeCall(t, n, "list", "", map[string]any{"limit": 1})
		if err != nil {
			t.Fatal(err)
		}
		cursor := result.(map[string]any)["next_cursor"].(string)
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		fields["unknown"] = true
		raw, _ = json.Marshal(fields)
		if _, err := nativeCall(t, n, "list", "", map[string]any{"limit": 1, "cursor": base64.RawURLEncoding.EncodeToString(raw)}); err == nil {
			t.Fatal("cursor unknown fields accepted")
		}
	})
	t.Run("cancelled success", func(t *testing.T) {
		n, _, _ := nativeFixture(t, []string{"unit"}, func(ctx context.Context, _ string) (NativeWorkspaceCheckResult, error) {
			<-ctx.Done()
			return NativeWorkspaceCheckResult{ExitCode: 0, ProfileSHA256: strings.Repeat("b", 64)}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if _, err := n.invoke(ctx, "check", []byte(`{"check_id":"unit"}`), "cancelled"); err == nil {
			t.Fatal("check reported success after cancellation without stop proof")
		}
	})
}

func TestNativeWorkspaceRejectsActualLinksAndFileBounds(t *testing.T) {
	n, w, _ := nativeFixture(t, nil, nil)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("fixture private data"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(w.Source, "linked")
	if err := os.Symlink(filepath.Dir(outside), link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		if output, e := exec.Command("cmd", "/d", "/c", "mklink", "/J", link, filepath.Dir(outside)).CombinedOutput(); e != nil {
			t.Fatalf("junction fixture: %v %s", e, output)
		}
	}
	if _, err := nativeCall(t, n, "read", "", map[string]any{"path": "linked/outside.txt"}); err == nil {
		t.Fatal("actual outside link was read")
	}
	if _, err := nativeCall(t, n, "remove", "remove-link", map[string]any{"path": "linked/outside.txt", "expected_sha256": digest([]byte("fixture private data"))}); err == nil {
		t.Fatal("actual outside link was removed")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Source, "too-large.txt"), []byte(strings.Repeat("x", nativeWorkspaceFileLimit+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "read", "", map[string]any{"path": "too-large.txt"}); err == nil {
		t.Fatal("oversized file read")
	}
}

func TestNativeWorkspaceCloseThenReadyToSeal(t *testing.T) {
	for _, withCheck := range []bool{false, true} {
		t.Run(strconv.FormatBool(withCheck), func(t *testing.T) {
			checks := []string{}
			if withCheck {
				checks = []string{"unit"}
			}
			n, _, _ := nativeFixture(t, checks, func(context.Context, string) (NativeWorkspaceCheckResult, error) {
				return NativeWorkspaceCheckResult{ExitCode: 0, ProfileSHA256: strings.Repeat("b", 64)}, nil
			})
			if withCheck {
				if _, err := nativeCall(t, n, "check", "check", map[string]any{"check_id": "unit"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := n.Close(); err != nil {
				t.Fatal(err)
			}
			if err := n.ReadyToSeal(); err != nil {
				t.Fatal(err)
			}
			n.mu.Lock()
			n.ledger.Unknown = true
			n.mu.Unlock()
			if err := n.ReadyToSeal(); err == nil {
				t.Fatal("unknown closed workspace ready to seal")
			}
		})
	}
}
