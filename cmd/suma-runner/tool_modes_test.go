package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

func TestPrepareToolsEmptyResultDoesNotCreateConfigurationOrReadCredentials(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	file := filepath.Join(root, "commands.json")
	var out, stderr bytes.Buffer
	code := run([]string{"--language=en", "--prepare-tools", file}, &out, &stderr, func(name string) (string, bool) {
		if strings.Contains(name, "TOKEN") {
			t.Fatal("preparation read a platform credential")
		}
		return "", false
	})
	var result struct {
		Protocol     string `json:"protocol"`
		Prepared     bool   `json:"prepared"`
		CommandCount int    `json:"command_count"`
		Remaining    []any  `json:"remaining"`
	}
	if code != 1 || json.Unmarshal(out.Bytes(), &result) != nil || result.Protocol != "suma-runner-tools-preparation-v1" || result.Prepared || result.CommandCount != 0 || len(result.Remaining) != 8 {
		t.Fatalf("empty preparation facts lost: code=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("empty preparation created a configuration", err)
	}
}

func TestPrepareToolsEmptyFlagCannotFallThroughToWorkerStartup(t *testing.T) {
	for _, args := range [][]string{
		{"--prepare-tools="},
		{"--prepare-tools=", "--project=project", "--worker=worker"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := run(append([]string{"--language=en"}, args...), &out, &stderr, func(name string) (string, bool) {
				if strings.Contains(name, "TOKEN") {
					t.Fatal("empty preparation flag fell through to Runner credentials")
				}
				return "", false
			})
			if code != 1 || out.Len() != 0 || (!strings.Contains(stderr.String(), "not prepared") && !strings.Contains(stderr.String(), "cannot be combined")) {
				t.Fatalf("empty preparation flag did not fail in its own mode: code=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
		})
	}
}

func TestRunnerToolModesRejectMixedOptionsBeforeCredentialReads(t *testing.T) {
	for _, args := range [][]string{
		{"--prepare-tools=x", "--report-tools"},
		{"--prepare-tools=x", "--allow-exec"},
		{"--prepare-tools=x", "--commands=x"},
		{"--prepare-tools=x", "--project=p"},
		{"--prepare-tools=x", "--discover-installed-tools"},
		{"--prepare-tools=x", "--inspect-state"},
		{"--prepare-tools=x", "--describe-acp-models-stdin"},
		{"--report-tools", "--describe-tools"},
		{"--report-tools", "--pool-config=x"},
		{"--report-tools", "--merge-preflight=x"},
		{"--report-tools", "--external-config=x"},
		{"--report-tools", "--discover-installed-tools"},
		{"--report-tools", "--inspect-state"},
		{"--report-tools", "--describe-acp-models-stdin"},
		{"--report-tools", "--local-tool=codex-jsonl"},
		{"--report-tools=false", "--project=p", "--worker=w"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := run(append([]string{"--language=en"}, args...), &out, &stderr, func(name string) (string, bool) {
				if strings.Contains(name, "TOKEN") {
					t.Fatal("mixed modes read a credential")
				}
				return "", false
			})
			if code == 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "cannot be combined") {
				t.Fatalf("mode guard missing: code=%d stdout=%s stderr=%s", code, &out, &stderr)
			}
		})
	}
}

func TestReportToolsLostAckRecoversOriginalBodyAndNeverClaims(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "commands.json")
	if err := os.WriteFile(file, []byte(`{"original":{"executable":"not-installed-and-must-not-run","args":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	bodies := [][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/runner/tool-inventory" || r.Header.Get("Authorization") != "Bearer private-runner-token" {
			t.Error("single report reached a task endpoint or lost identity", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		raw, _ := json.Marshal(body)
		bodies = append(bodies, raw)
		if len(bodies) <= 2 {
			_, _ = w.Write([]byte(`{"acknowledged":true,"observed_at":`))
			return
		}
		_, _ = w.Write([]byte(`{"acknowledged":true,"observed_at":"2026-11-01T01:00:00Z"}`))
	}))
	defer server.Close()
	args := []string{"--language=en", "--report-tools", "--control=" + server.URL, "--project=project", "--worker=registered-worker", "--commands=" + file, "--state=" + filepath.Join(root, "state"), "--workspace=" + root}
	lookup := func(name string) (string, bool) {
		if name == "SUMA_RUNNER_TOKEN" {
			return "private-runner-token", true
		}
		return "", false
	}
	var out, stderr bytes.Buffer
	if code := run(args, &out, &stderr, lookup); code == 0 || out.Len() != 0 || len(bodies) != 1 {
		t.Fatalf("lost ACK was not retained: code=%d stdout=%s stderr=%s requests=%d", code, &out, &stderr, len(bodies))
	}
	// Recovery does not depend on current configuration remaining readable.
	if err := os.WriteFile(file, []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := run(args, &out, &stderr, lookup); code == 0 || len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("invalid current config blocked original recovery: code=%d stdout=%s stderr=%s requests=%d", code, &out, &stderr, len(bodies))
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := run(args, &out, &stderr, lookup); code != 0 || len(bodies) != 3 || !bytes.Equal(bodies[0], bodies[2]) {
		t.Fatalf("missing current config blocked exact recovery: code=%d stdout=%s stderr=%s requests=%d", code, &out, &stderr, len(bodies))
	}
	var result struct {
		Protocol   string `json:"protocol"`
		ProjectID  string `json:"project_id"`
		Recovered  bool   `json:"recovered"`
		ObservedAt string `json:"observed_at"`
		Items      []struct {
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"items"`
	}
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Protocol != "suma-runner-tool-report-v1" || result.ProjectID != "project" || !result.Recovered || result.ObservedAt != "2026-11-01T01:00:00Z" || len(result.Items) != 1 || result.Items[0].Available || !strings.Contains(result.Items[0].Reason, "disabled") || bytes.Contains(out.Bytes(), []byte("private-runner-token")) {
		t.Fatalf("report invented execution or lost original ACK facts: %s", &out)
	}
}

func TestReportToolsRequiresRegisteredCredentialAndOriginalJournalOwnership(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	args := []string{"--language=en", "--report-tools", "--project=project", "--worker=worker", "--state=" + state}
	var out, stderr bytes.Buffer
	if code := run(args, &out, &stderr, nil); code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "registered execution credential") {
		t.Fatalf("missing credential did not fail before startup: code=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
	if _, err := os.Lstat(state); !os.IsNotExist(err) {
		t.Fatal("credential guard opened the journal", err)
	}
	j, err := runner.OpenJournal(state)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	out.Reset()
	stderr.Reset()
	if code := run(args, &out, &stderr, func(name string) (string, bool) {
		if name == "SUMA_RUNNER_TOKEN" {
			return "registered-token", true
		}
		return "", false
	}); code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "journal already owned") {
		t.Fatalf("single report bypassed the original lock: code=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
}
