package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func nativeSealFixture(t *testing.T, attributes ...string) (*attemptWorkspace, NativeWorkspaceEvidence) {
	t.Helper()
	repo, _ := gitFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "delete.txt"), []byte("remove me\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "delete.txt")
	if len(attributes) != 0 {
		if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte(attributes[0]), 0600); err != nil {
			t.Fatal(err)
		}
		gitTest(t, repo, "add", ".gitattributes")
	}
	gitTest(t, repo, "commit", "-m", "native seal fixture")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	a := Assignment{RunID: "native-seal-run", NodeID: "code", Attempt: 1, Fence: 7, WorkerID: "native-worker", Type: "agent", Command: []string{"native"}}
	w, err := prepareWorkspace(context.Background(), a, ExecConfig{Repository: repo, BaseCommit: base}, CommandSpec{})
	if err != nil {
		t.Fatal(err)
	}
	return w, NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: strings.Repeat("a", 64), State: "closed", Files: []NativeWorkspaceFileEffect{}, Checks: []NativeWorkspaceCheckResult{}}
}

func nativeSealWrite(t *testing.T, w *attemptWorkspace, evidence *NativeWorkspaceEvidence, path, previous, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(w.Source, path)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Source, path), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	evidence.Generation++
	old := ""
	if previous != "" {
		old = digest([]byte(previous))
	}
	evidence.Files = append(evidence.Files, NativeWorkspaceFileEffect{Path: path, OperationID: "fixture-file-" + path, PreviousSHA256: old, SHA256: digest([]byte(content)), Generation: evidence.Generation})
}

func TestNativeWorkspaceSealCommitsAcceptedEffectsAndLeavesCheckoutAlone(t *testing.T) {
	w, evidence := nativeSealFixture(t)
	nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "fixed\n")
	nativeSealWrite(t, w, &evidence, "nested/new.txt", "", "new file\n")
	if err := os.Remove(filepath.Join(w.Source, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	evidence.Generation++
	evidence.Files = append(evidence.Files, NativeWorkspaceFileEffect{Path: "delete.txt", OperationID: "fixture-remove", PreviousSHA256: digest([]byte("remove me\n")), Removed: true, Generation: evidence.Generation})
	seal, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil || seal.State != "sealed" || seal.Commit == w.BaseCommit || seal.ChangedPaths != 3 || !commitID.MatchString(seal.Tree) {
		t.Fatalf("seal: %+v %v", seal, err)
	}
	if got := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")); got != seal.Commit {
		t.Fatalf("attempt HEAD %q differs from seal %+v", got, seal)
	}
	if got := strings.TrimSpace(gitTest(t, w.Source, "status", "--porcelain=v1", "--untracked-files=all")); got != "" {
		t.Fatalf("sealed index/worktree differ: %q", got)
	}
	if got := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", seal.Commit+"^")); got != w.BaseCommit {
		t.Fatal("candidate does not directly descend from fixed base")
	}
	if got := strings.TrimSpace(gitTest(t, w.Repository, "rev-parse", "HEAD")); got != w.BaseCommit {
		t.Fatal("user repository HEAD moved")
	}
	if got := gitTest(t, w.Repository, "show", "HEAD:file.txt"); got != "base\n" {
		t.Fatal("user repository content changed")
	}
	if got := strings.TrimSpace(gitTest(t, w.Source, "show", "-s", "--format=%an <%ae>", seal.Commit)); got != "suma native workspace <native-workspace@suma.invalid>" {
		t.Fatalf("machine candidate used unexpected author %q", got)
	}
	captured, err := w.capture(context.Background(), Result{ExitCode: 0})
	if err != nil || captured["clean"] != true || captured["head_commit"] != seal.Commit {
		t.Fatalf("capture did not retain clean candidate: %+v %v", captured, err)
	}
}

func TestNativeWorkspaceSealNoChangeDoesNotCreateCommit(t *testing.T) {
	for _, rewrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "untouched", true: "reverted"}[rewrite], func(t *testing.T) {
			w, evidence := nativeSealFixture(t)
			if rewrite {
				nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "base\n")
			}
			seal, err := sealNativeWorkspace(context.Background(), w, evidence)
			if err != nil || seal.State != "no_change" || seal.Commit != w.BaseCommit || seal.ChangedPaths != 0 {
				t.Fatalf("no-change seal: %+v %v", seal, err)
			}
			if got := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")); got != w.BaseCommit {
				t.Fatal("no-change attempt created a commit")
			}
		})
	}
}

func TestNativeWorkspaceSealRejectsUnacceptedOrDriftedSources(t *testing.T) {
	cases := map[string]func(*testing.T, *attemptWorkspace, *NativeWorkspaceEvidence){
		"unaccepted": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("not accepted\n"), 0600)
		},
		"ignored-unaccepted": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			nativeSealWrite(t, w, e, ".gitignore", ".cache/\n", ".cache/\nhidden.txt\n")
			os.WriteFile(filepath.Join(w.Source, "hidden.txt"), []byte("hidden edit\n"), 0600)
		},
		"result-drift": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			nativeSealWrite(t, w, e, "file.txt", "base\n", "accepted\n")
			os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("later content\n"), 0600)
		},
		"previous-drift": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			nativeSealWrite(t, w, e, "file.txt", "wrong base\n", "accepted\n")
		},
		"protected-path": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			nativeSealWrite(t, w, e, ".cache/accepted.txt", "", "protected\n")
		},
		"head-drift": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			gitTest(t, w.Source, "-c", "user.name=Unexpected", "-c", "user.email=unexpected@example.invalid", "commit", "--allow-empty", "-m", "unexpected HEAD")
		},
		"staged-drift": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) {
			nativeSealWrite(t, w, e, "file.txt", "base\n", "accepted\n")
			gitTest(t, w.Source, "add", "file.txt")
		},
		"source-identity": func(t *testing.T, w *attemptWorkspace, e *NativeWorkspaceEvidence) { w.Source = w.Repository },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			w, evidence := nativeSealFixture(t)
			change(t, w, &evidence)
			if result, err := sealNativeWorkspace(context.Background(), w, evidence); err == nil {
				t.Fatalf("unsafe source accepted: %+v", result)
			}
			if got := strings.TrimSpace(gitTest(t, w.Repository, "rev-parse", "HEAD")); got != w.BaseCommit {
				t.Fatal("rejected seal moved user HEAD")
			}
		})
	}
}

func TestNativeWorkspaceSealRecoversPreparedAndLostReferenceAcknowledgement(t *testing.T) {
	for _, refsApplied := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared-before-CAS", true: "refs-applied-ACK-lost"}[refsApplied], func(t *testing.T) {
			w, evidence := nativeSealFixture(t)
			nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
			original, err := sealNativeWorkspace(context.Background(), w, evidence)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(w.Directory, "native-workspace", "seal.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var checkpoint map[string]any
			if err := json.Unmarshal(raw, &checkpoint); err != nil {
				t.Fatal(err)
			}
			checkpoint["state"] = "prepared"
			raw, _ = json.Marshal(checkpoint)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if !refsApplied {
				gitTest(t, w.Source, "update-ref", "HEAD", w.BaseCommit, original.Commit)
				gitTest(t, w.Source, "update-ref", "-d", "refs/suma/native-candidates/"+original.OperationID, original.Commit)
			}
			gitTest(t, w.Source, "read-tree", w.BaseCommit)
			recovered, err := sealNativeWorkspace(context.Background(), w, evidence)
			if err != nil || recovered != original {
				t.Fatalf("original %+v, recovered %+v: %v", original, recovered, err)
			}
			if got := strings.TrimSpace(gitTest(t, w.Source, "status", "--porcelain=v1")); got != "" {
				t.Fatalf("recovered index dirty: %q", got)
			}
		})
	}
}

func TestNativeWorkspaceSealDoesNotInferSuccessFromHeadWithoutOperationReceipt(t *testing.T) {
	w, evidence := nativeSealFixture(t)
	nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
	seal, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil {
		t.Fatal(err)
	}
	gitTest(t, w.Source, "update-ref", "-d", "refs/suma/native-candidates/"+seal.OperationID, seal.Commit)
	if _, err := sealNativeWorkspace(context.Background(), w, evidence); err == nil {
		t.Fatal("matching HEAD without original operation proof was accepted")
	}
}

func TestNativeWorkspaceSealCompletedOperationDoesNotReapplyWithdrawnReferences(t *testing.T) {
	w, evidence := nativeSealFixture(t)
	nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
	seal, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil {
		t.Fatal(err)
	}
	gitTest(t, w.Source, "update-ref", "HEAD", w.BaseCommit, seal.Commit)
	gitTest(t, w.Source, "update-ref", "-d", "refs/suma/native-candidates/"+seal.OperationID, seal.Commit)
	gitTest(t, w.Source, "read-tree", w.BaseCommit)
	if _, err := sealNativeWorkspace(context.Background(), w, evidence); err == nil {
		t.Fatal("completed operation silently reapplied withdrawn references")
	}
	if head := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")); head != w.BaseCommit {
		t.Fatal("withdrawn reference was modified")
	}
}

func TestNativeWorkspaceSealGitAttributesAndIgnoredFilesRemainExplicit(t *testing.T) {
	for _, kind := range []string{"LF", "CRLF", "external-filter", "accepted-ignored"} {
		t.Run(kind, func(t *testing.T) {
			var attributes []string
			if kind == "LF" || kind == "CRLF" {
				attributes = []string{"*.txt text eol=lf\n"}
			} else if kind == "external-filter" {
				attributes = []string{"*.txt filter=arbitrary\n"}
			}
			w, evidence := nativeSealFixture(t, attributes...)
			switch kind {
			case "LF", "CRLF":
				content := "accepted\n"
				if kind == "CRLF" {
					content = "accepted\r\n"
				}
				nativeSealWrite(t, w, &evidence, "file.txt", "base\n", content)
			case "external-filter":
				nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
				gitTest(t, w.Source, "config", "filter.arbitrary.clean", "exit 77")
			case "accepted-ignored":
				nativeSealWrite(t, w, &evidence, ".gitignore", ".cache/\n", ".cache/\nignored.txt\n")
				nativeSealWrite(t, w, &evidence, "ignored.txt", "", "accepted ignored source\n")
			}
			seal, err := sealNativeWorkspace(context.Background(), w, evidence)
			if kind == "CRLF" || kind == "external-filter" {
				if err == nil {
					t.Fatalf("unsupported conversion produced a seal: %+v", seal)
				}
				if head := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")); head != w.BaseCommit {
					t.Fatal("unsupported conversion changed HEAD")
				}
			} else if err != nil || seal.State != "sealed" {
				t.Fatalf("supported candidate: %+v %v", seal, err)
			}
		})
	}
}

func TestNativeWorkspaceSealDoesNotInvokeReferenceTransactionHooks(t *testing.T) {
	w, evidence := nativeSealFixture(t)
	nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
	hooks := filepath.Join(w.Directory, "fixture-hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(w.Directory, "hook-executed")
	script := "#!/bin/sh\nprintf executed > '" + filepath.ToSlash(marker) + "'\n"
	if err := os.WriteFile(filepath.Join(hooks, "reference-transaction"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, w.Source, "config", "core.hooksPath", hooks)
	seal, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil || seal.State != "sealed" {
		t.Fatalf("seal with configured hook: %+v %v", seal, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("candidate sealing invoked a repository hook")
	}
}

func TestNativeWorkspaceSealPreparedBeforeObjectWriteKeepsOriginalOperation(t *testing.T) {
	w, evidence := nativeSealFixture(t)
	nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
	directory := filepath.Join(w.Directory, "native-workspace")
	if err := safeMkdir(directory); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(directory, "seal.index.lock")
	if err := os.WriteFile(lock, []byte("fixture interrupted index operation"), 0600); err != nil {
		t.Fatal(err)
	}
	partial, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err == nil || partial.State != "unknown" || partial.OperationID == "" || partial.PreviousCommit != w.BaseCommit || partial.Commit != "" {
		t.Fatalf("prepared operation did not retain unknown identity: %+v %v", partial, err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, "seal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var before nativeSealCheckpoint
	if err := json.Unmarshal(raw, &before); err != nil || before.State != "prepared" || before.Result.Commit != "" {
		t.Fatalf("original prepare not retained: %+v %v", before, err)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	seal, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil || seal.State != "sealed" || seal.OperationID != before.OperationID {
		t.Fatalf("prepared recovery: %+v %v", seal, err)
	}
	raw, err = os.ReadFile(filepath.Join(directory, "seal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var after nativeSealCheckpoint
	if err := json.Unmarshal(raw, &after); err != nil || after.AuthorDate != before.AuthorDate {
		t.Fatal("recovery changed original machine author date")
	}
	evidence.Files[0].OperationID = "different-accepted-operation"
	if _, err := sealNativeWorkspace(context.Background(), w, evidence); err == nil {
		t.Fatal("same seal operation accepted a changed evidence request")
	}
}

func TestNativeWorkspaceSealUnknownAfterReferenceEffectRetainsCandidate(t *testing.T) {
	w, evidence := nativeSealFixture(t)
	nativeSealWrite(t, w, &evidence, "file.txt", "base\n", "accepted\n")
	gitDirectory := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "--absolute-git-dir"))
	lock := filepath.Join(gitDirectory, "index.lock")
	if err := os.WriteFile(lock, []byte("fixture interrupted index install"), 0600); err != nil {
		t.Fatal(err)
	}
	partial, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err == nil || partial.State != "unknown" || partial.OperationID == "" || partial.PreviousCommit != w.BaseCommit || partial.Commit == "" || partial.Commit == w.BaseCommit {
		t.Fatalf("unknown Git effect lost original operation: %+v %v", partial, err)
	}
	if actual := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")); actual != partial.Commit {
		t.Fatal("fixture did not interrupt after reference effect")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	sealed, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil || sealed.State != "sealed" || sealed.OperationID != partial.OperationID || sealed.Commit != partial.Commit {
		t.Fatalf("unknown effect did not recover same candidate: %+v %v", sealed, err)
	}
}

func TestNativeWorkspaceSealRejectsSourceLinkAndOversizedSource(t *testing.T) {
	t.Run("link", func(t *testing.T) {
		w, evidence := nativeSealFixture(t)
		if err := os.Symlink(filepath.Join(w.Repository, "file.txt"), filepath.Join(w.Source, "linked.txt")); err != nil {
			if runtime.GOOS != "windows" {
				t.Fatalf("cannot create source link fixture: %v", err)
			}
			// Windows junctions exercise the actual reparse boundary without
			// requiring the optional symbolic-link privilege on this host.
			target := filepath.Join(w.Directory, "junction-target")
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "outside.txt"), []byte("outside source\n"), 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", filepath.Join(w.Source, "linked"), target)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("cannot create Windows junction fixture: %v %s", err, output)
			}
		}
		if _, err := sealNativeWorkspace(context.Background(), w, evidence); err == nil {
			t.Fatal("source link or reparse directory was accepted")
		}
	})
	t.Run("source-byte-limit", func(t *testing.T) {
		w, evidence := nativeSealFixture(t)
		f, err := os.OpenFile(filepath.Join(w.Source, "oversized.txt"), os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = f.Truncate(nativeSealMaxBytes + 1)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sealNativeWorkspace(context.Background(), w, evidence); err == nil {
			t.Fatal("source byte limit was bypassed")
		}
	})
}
