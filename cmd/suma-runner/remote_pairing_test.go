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
	"time"

	"github.com/ai-mada/suma/internal/runner"
	rp "github.com/ai-mada/suma/internal/runnerpairingcontract"
)

func TestRunnerPairingModeFlagsCannotFallThroughToExecution(t *testing.T) {
	for _, args := range [][]string{
		{"--pair-runner="},
		{"--pair-runner=not-a-pair-private-value"},
		{"--identity="},
		{"--device-name=private-value"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--report-tools"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--prepare-tools=x"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--allow-exec"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--commands=x"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--worker=x"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--state=x"},
		{"--pair-runner=" + strings.Repeat("a", 32), "--pool-config=x"},
		{"--identity=x", "--describe-tools"},
		{"--identity=x", "--discover-installed-tools"},
		{"--identity=x", "--inspect-state"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := run(append([]string{"--language=en"}, args...), &out, &stderr, func(name string) (string, bool) {
				if strings.Contains(name, "TOKEN") {
					t.Fatal("invalid pairing mode read an execution credential")
				}
				return "", false
			})
			if code != 1 || out.Len() != 0 || stderr.Len() == 0 || strings.Contains(stderr.String(), "private-value") {
				t.Fatalf("invalid enrollment mode did not fail closed: code=%d out=%s err=%s", code, &out, &stderr)
			}
		})
	}
}

type pairingCLIStore struct {
	values  map[string]string
	missing bool
}

func (s *pairingCLIStore) Get(service, account string) (string, error) {
	value, ok := s.values[service+account]
	if !ok || s.missing {
		return "", runner.ErrRunnerSecretNotFound
	}
	return value, nil
}
func (s *pairingCLIStore) Set(service, account, secret string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[service+account] = secret
	return nil
}
func noPairingEnvironmentCredential(t *testing.T) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if strings.Contains(name, "TOKEN") {
			t.Fatal("saved identity read an unrelated environment credential")
		}
		return "", false
	}
}

func TestRunnerPairingCLIPreparesOnlyPublicIdentityWithoutTaskJournal(t *testing.T) {
	project, pair := strings.Repeat("1", 32), strings.Repeat("2", 32)
	file := filepath.Join(t.TempDir(), "identity.json")
	store := &pairingCLIStore{}
	var proposal rp.RunnerPairingProposalRequest
	var token string
	calls := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(r.URL.Path, "/api/v1/runner-pairings/"+pair+"/") {
			t.Error("pairing CLI entered a task endpoint", r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "/proposal") {
			if json.NewDecoder(r.Body).Decode(&proposal) != nil {
				t.Error("invalid proposal")
			}
		}
		created := time.Now().UTC().Truncate(time.Second)
		value := rp.RunnerPairing{ID: pair, ProjectID: project, Version: 2, State: "proposed", ControlURL: server.URL, CreatedAt: created.Format(time.RFC3339), ExpiresAt: created.Add(10 * time.Minute).Format(time.RFC3339), DeviceName: proposal.DeviceName, CredentialSHA256: proposal.CredentialSHA256}
		if strings.HasSuffix(r.URL.Path, "/status") {
			value.State = "approved"
			value.Version = 3
			value.WorkerID = "remote-" + strings.Repeat("a", 32)
			value.CredentialExpiresAt = created.Add(24 * time.Hour).Format(time.RFC3339)
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	args := []string{"--language=en", "--pair-runner=" + pair, "--control=" + server.URL, "--project=" + project, "--identity=" + file}
	if code := runWithRunnerSecrets(args, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 0 || calls != 2 || !strings.Contains(out.String(), `"prepared":true`) || bytes.Contains(out.Bytes(), []byte(token)) {
		t.Fatalf("pairing CLI failed or disclosed a secret: code=%d calls=%d out=%s err=%s", code, calls, &out, &stderr)
	}
	if _, err := os.Lstat(file + ".journal"); !os.IsNotExist(err) {
		t.Fatal("pairing automatically opened a task journal", err)
	}
	for _, frame := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte{'\n'}) {
		var result struct {
			CredentialSHA256 string `json:"credential_sha256"`
		}
		if json.Unmarshal(frame, &result) != nil || len(result.CredentialSHA256) != 64 || result.CredentialSHA256 != proposal.CredentialSHA256 || result.CredentialSHA256 == token {
			t.Fatal("terminal output did not expose the exact public fingerprint for browser review")
		}
	}
	raw, err := os.ReadFile(file)
	if err != nil || bytes.Contains(raw, []byte(token)) || proposal.DeviceName == "" {
		t.Fatal("public identity was incomplete or secret", err)
	}
}

func TestRunnerIdentityReportAdoptsScopeAndRejectsDriftBeforeJournalChanges(t *testing.T) {
	project, pair, worker := strings.Repeat("1", 32), strings.Repeat("2", 32), "remote-"+strings.Repeat("a", 32)
	file := filepath.Join(t.TempDir(), "identity.json")
	store := &pairingCLIStore{}
	calls := 0
	var token string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/runner/tool-inventory" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("saved identity changed the original reporting path or secret", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"acknowledged":true,"observed_at":"2026-11-01T03:00:00Z"}`))
	}))
	defer server.Close()
	identity, err := runner.PrepareRunnerPairingIdentity(file, server.URL, project, pair, "owned host", store)
	if err != nil {
		t.Fatal(err)
	}
	token = identity.Token()
	if err = identity.SaveApproved(worker, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	_ = identity.Close()
	var out, stderr bytes.Buffer
	args := []string{"--language=en", "--report-tools", "--identity=" + file}
	if code := runWithRunnerSecrets(args, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 0 || calls != 1 || !strings.Contains(out.String(), worker) || bytes.Contains(out.Bytes(), []byte(token)) {
		t.Fatalf("saved identity could not use the original single reporter: code=%d out=%s err=%s", code, &out, &stderr)
	}
	if _, err = os.Stat(filepath.Join(file+".journal", "tool-inventory.json")); err != nil {
		t.Fatal("identity did not receive its own journal", err)
	}
	for _, option := range []string{"--control=" + server.URL + "/different", "--project=" + strings.Repeat("3", 32), "--worker=other-worker"} {
		out.Reset()
		stderr.Reset()
		state := filepath.Join(t.TempDir(), "untouched")
		changed := append(append([]string{}, args...), option, "--state="+state)
		if code := runWithRunnerSecrets(changed, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 1 || calls != 1 || out.Len() != 0 {
			t.Fatalf("scope drift reached the reporter: code=%d out=%s err=%s", code, &out, &stderr)
		}
		if _, err = os.Lstat(state); !os.IsNotExist(err) {
			t.Fatal("scope drift changed a task journal", err)
		}
	}
	store.missing = true
	out.Reset()
	stderr.Reset()
	if code := runWithRunnerSecrets(args, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 1 || calls != 1 {
		t.Fatal("missing system secret fell back to an environment credential")
	}
}
