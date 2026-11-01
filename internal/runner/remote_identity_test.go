package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const identityTestProject = "11111111111111111111111111111111"
const identityTestPair = "22222222222222222222222222222222"

type identityMemoryStore struct {
	values    map[string]string
	sets      int
	getError  error
	setError  error
	beforeSet func(string, string, string)
	afterSet  func(string, string, string)
}

func newIdentityMemoryStore() *identityMemoryStore {
	return &identityMemoryStore{values: map[string]string{}}
}
func (s *identityMemoryStore) Get(service, account string) (string, error) {
	if s.getError != nil {
		return "", s.getError
	}
	value, ok := s.values[service+"/"+account]
	if !ok {
		return "", ErrRunnerSecretNotFound
	}
	return value, nil
}
func (s *identityMemoryStore) Set(service, account, secret string) error {
	if s.beforeSet != nil {
		s.beforeSet(service, account, secret)
	}
	s.sets++
	s.values[service+"/"+account] = secret
	if s.afterSet != nil {
		s.afterSet(service, account, secret)
	}
	return s.setError
}

func prepareIdentityFixture(t *testing.T, file string, store RunnerSecretStore) *RunnerIdentityHandle {
	t.Helper()
	h, err := PrepareRunnerPairingIdentity(file, "https://control.example/suma/", identityTestProject, identityTestPair, "  owned machine  ", store)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRunnerIdentityPersistsSecretBeforeMetadataAndResumesOriginalRequest(t *testing.T) {
	file := filepath.Join(t.TempDir(), "private", "identity.json")
	store := newIdentityMemoryStore()
	store.beforeSet = func(service, account, secret string) {
		if _, err := os.Lstat(file); !os.IsNotExist(err) {
			t.Fatal("metadata existed before system secret was stored", err)
		}
		if service != "suma-runner-identity-v1" || len(account) != 64 || len(secret) != 64 {
			t.Fatal("unexpected secret storage scope")
		}
	}
	h := prepareIdentityFixture(t, file, store)
	identity, token := h.Identity(), h.Token()
	if identity.Protocol != "suma-runner-identity-v1" || identity.ControlURL != "https://control.example/suma" || identity.DeviceName != "owned machine" || len(identity.ProposalOperationID) != 32 || len(identity.CredentialSHA256) != 64 || identity.CredentialSHA256 == token {
		t.Fatalf("invalid public identity: %+v", identity)
	}
	raw, err := os.ReadFile(h.Path())
	if err != nil || bytes.Contains(raw, []byte(token)) {
		t.Fatal("public metadata contains a secret", err)
	}
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 7 {
		t.Fatal("incorrect public metadata fields")
	}
	if _, err = LoadRunnerIdentity(file, store); err == nil {
		t.Fatal("same identity acquired a second owner")
	}
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = PrepareRunnerPairingIdentity(file, "https://control.example/suma", identityTestProject, identityTestPair, "", store)
	if err != nil {
		t.Fatal("omitted device name did not resume the original saved request", err)
	}
	defer h.Close()
	if h.Identity() != identity || h.Token() != token || store.sets != 1 {
		t.Fatal("resume replaced the original token or proposal operation")
	}
	worker := "remote-33333333333333333333333333333333"
	// Current authority is checked by the server, not the local wall clock.
	if err = h.SaveApproved(worker, "2020-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if h.Identity().WorkerID != worker {
		t.Fatal("approved public identity was not saved")
	}
	if err = h.SaveApproved(worker, "2020-01-01T00:00:00Z"); err != nil {
		t.Fatal("exact approval replay rejected", err)
	}
	if err = h.SaveApproved("remote-44444444444444444444444444444444", "2020-01-01T00:00:00Z"); err == nil {
		t.Fatal("approval rewrote the original worker")
	}
}

func TestRunnerIdentityMissingSecretMismatchAndInvalidMetadataFailClosed(t *testing.T) {
	for _, kind := range []string{"missing_secret", "wrong_secret", "unknown_field", "duplicate_field", "null_field", "half_approval", "invalid_json"} {
		t.Run(kind, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "identity.json")
			store := newIdentityMemoryStore()
			h := prepareIdentityFixture(t, file, store)
			if err := h.Close(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing_secret":
				store.values = map[string]string{}
			case "wrong_secret":
				for k := range store.values {
					store.values[k] = strings.Repeat("a", 64)
				}
			case "unknown_field":
				raw = append(bytes.TrimSpace(raw[:len(bytes.TrimSpace(raw))-1]), []byte(`,"token":"secret"}`)...)
			case "duplicate_field":
				raw = append(bytes.TrimSpace(raw[:len(bytes.TrimSpace(raw))-1]), []byte(`,"project_id":"`+identityTestProject+`"}`)...)
			case "null_field":
				var m map[string]any
				_ = json.Unmarshal(raw, &m)
				m["device_name"] = nil
				raw, _ = json.Marshal(m)
			case "half_approval":
				var m map[string]any
				_ = json.Unmarshal(raw, &m)
				m["worker_id"] = "remote-33333333333333333333333333333333"
				raw, _ = json.Marshal(m)
			case "invalid_json":
				raw = []byte("{")
			}
			if err = os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if opened, err := LoadRunnerIdentity(file, store); err == nil {
				_ = opened.Close()
				t.Fatal("unsafe identity was loaded")
			}
			if opened, err := PrepareRunnerPairingIdentity(file, "https://control.example/suma", identityTestProject, identityTestPair, "owned machine", store); err == nil {
				_ = opened.Close()
				t.Fatal("unsafe identity was silently replaced")
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(raw, after) || store.sets != 1 {
				t.Fatal("failure changed original metadata or issued another secret", err)
			}
		})
	}
}

func TestRunnerIdentitySeparatePathAccountsAndOrphanCannotBeOverwritten(t *testing.T) {
	root := t.TempDir()
	store := newIdentityMemoryStore()
	h := prepareIdentityFixture(t, filepath.Join(root, "first.json"), store)
	firstToken := h.Token()
	_ = h.Close()
	h = prepareIdentityFixture(t, filepath.Join(root, "second.json"), store)
	defer h.Close()
	if len(store.values) != 2 || h.Token() == firstToken {
		t.Fatal("different identity paths reused a secret account")
	}
	for _, kind := range []string{"set_uncertain", "readback_mismatch", "backend_unavailable"} {
		t.Run(kind, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "identity.json")
			store := newIdentityMemoryStore()
			switch kind {
			case "set_uncertain":
				store.setError = errors.New("uncertain store write")
			case "readback_mismatch":
				store.afterSet = func(service, account, secret string) { store.values[service+"/"+account] = strings.Repeat("a", 64) }
			case "backend_unavailable":
				store.getError = errors.New("credential backend unavailable")
			}
			if opened, err := PrepareRunnerPairingIdentity(file, "https://control.example/suma", identityTestProject, identityTestPair, "owned machine", store); err == nil {
				_ = opened.Close()
				t.Fatal("unconfirmed system secret produced an identity")
			}
			if _, err := os.Lstat(file); !os.IsNotExist(err) {
				t.Fatal("unconfirmed secret created public metadata", err)
			}
			priorSets := store.sets
			store.setError = nil
			store.afterSet = nil
			if opened, err := PrepareRunnerPairingIdentity(file, "https://control.example/suma", identityTestProject, identityTestPair, "owned machine", store); err == nil {
				_ = opened.Close()
				t.Fatal("orphan secret was overwritten or insecure backend accepted")
			}
			if store.sets != priorSets {
				t.Fatal("retry overwrote an orphan secret")
			}
		})
	}
}

func TestRunnerIdentityRejectsLinkedPathsAndChangedResumeSource(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip(err)
		}
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); junctionErr != nil {
			t.Skip(err, junctionErr, string(output))
		}
	}
	store := newIdentityMemoryStore()
	if opened, err := PrepareRunnerPairingIdentity(filepath.Join(link, "identity.json"), "https://control.example/suma", identityTestProject, identityTestPair, "owned machine", store); err == nil {
		_ = opened.Close()
		t.Fatal("identity followed a linked parent")
	}
	if store.sets != 0 {
		t.Fatal("unsafe path reached the system credential store")
	}
	file := filepath.Join(target, "identity.json")
	h := prepareIdentityFixture(t, file, store)
	_ = h.Close()
	for _, changed := range []string{"control", "project", "pair", "device"} {
		control, project, pair, device := "https://control.example/suma", identityTestProject, identityTestPair, "owned machine"
		switch changed {
		case "control":
			control += "/other"
		case "project":
			project = strings.Repeat("3", 32)
		case "pair":
			pair = strings.Repeat("4", 32)
		case "device":
			device = "another machine"
		}
		if opened, err := PrepareRunnerPairingIdentity(file, control, project, pair, device, store); err == nil {
			_ = opened.Close()
			t.Fatal("resume source changed", changed)
		}
	}
	if store.sets != 1 {
		t.Fatal("source mismatch issued another secret")
	}
}

func TestRunnerIdentityDefaultLocationAndInvalidInputs(t *testing.T) {
	config := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", config)
	} else {
		t.Setenv("XDG_CONFIG_HOME", config)
		t.Setenv("HOME", config)
	}
	file, err := DefaultRunnerIdentityPath(identityTestPair)
	expectedBase := config
	if runtime.GOOS == "darwin" {
		expectedBase = filepath.Join(config, "Library", "Application Support")
	}
	if err != nil || file != filepath.Join(expectedBase, "suma", "runner", identityTestPair, "identity.json") {
		t.Fatal("wrong default identity location", file, err)
	}
	if _, err = os.Lstat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatal("default location helper created state", err)
	}
	store := newIdentityMemoryStore()
	if h, err := PrepareRunnerPairingIdentity(file, "http://remote.example", identityTestProject, identityTestPair, "owned", store); err == nil {
		_ = h.Close()
		t.Fatal("insecure control URL accepted")
	}
	if h, err := PrepareRunnerPairingIdentity(file, "https://control.example", identityTestProject, identityTestPair, "bad\nname", store); err == nil {
		_ = h.Close()
		t.Fatal("control characters accepted in device name")
	}
	if store.sets != 0 {
		t.Fatal("invalid input reached secret persistence")
	}
	h, err := PrepareRunnerPairingIdentity(file, "https://control.example", identityTestProject, identityTestPair, "", store)
	if err != nil {
		t.Fatal("omitted device name did not use the local hostname", err)
	}
	defer h.Close()
	hostname, err := os.Hostname()
	if err != nil || h.Identity().DeviceName != strings.TrimSpace(hostname) {
		t.Fatal("default hostname was not saved exactly", err)
	}
}

func TestRunnerIdentityRejectsUnboundedApprovalAndMetadataChangedAfterOpening(t *testing.T) {
	file := filepath.Join(t.TempDir(), "identity.json")
	store := newIdentityMemoryStore()
	h := prepareIdentityFixture(t, file, store)
	defer h.Close()
	if err := h.SaveApproved("remote-33333333333333333333333333333333", "2026-11-01T00:00:00."+strings.Repeat("0", runnerIdentityLimit)+"Z"); err == nil {
		t.Fatal("unbounded RFC3339 value made identity impossible to reload")
	}
	raw := []byte("{\"external\":\"change\"}")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.SaveApproved("remote-33333333333333333333333333333333", "2026-11-01T00:00:00Z"); err == nil {
		t.Fatal("approval overwrote metadata changed after opening")
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("changed metadata was overwritten", err)
	}
}

func TestRunnerIdentityWindowsCanonicalLeafRejectsAliasesBeforeKeyring(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filename normalization")
	}
	root := t.TempDir()
	store := newIdentityMemoryStore()
	for _, leaf := range []string{"identity.json.", "identity.json ", "identity.json:stream"} {
		if h, err := PrepareRunnerPairingIdentity(filepath.Join(root, leaf), "https://control.example/suma", identityTestProject, identityTestPair, "owned", store); err == nil {
			_ = h.Close()
			t.Fatal("ambiguous Windows filename was accepted", leaf)
		}
		if store.sets != 0 {
			t.Fatal("ambiguous Windows path persisted an unresumable secret", leaf)
		}
	}
}

func TestRunnerIdentityApprovalRefusesRenamedParentWithCopiedMetadata(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "original")
	file := filepath.Join(parent, "identity.json")
	h := prepareIdentityFixture(t, file, newIdentityMemoryStore())
	defer h.Close()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "unknown-target")
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(target, "identity.json")
	if err = os.WriteFile(targetFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(parent, filepath.Join(root, "moved-original")); err != nil {
		if runtime.GOOS == "windows" {
			t.Log("Windows held identity lock prevents parent rename before link substitution:", err)
			return
		}
		t.Fatal(err)
	}
	if err = os.Symlink(target, parent); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", parent, target).CombinedOutput(); junctionErr != nil {
			t.Fatal(err, junctionErr, string(output))
		}
	}
	if err = h.SaveApproved("remote-33333333333333333333333333333333", "2026-11-01T00:00:00Z"); err == nil {
		t.Fatal("approval followed a substituted parent into an unknown target")
	}
	after, err := os.ReadFile(targetFile)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("approval wrote through the substituted parent", err)
	}
}
