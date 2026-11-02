package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const poolIdentity208 = "33333333333333333333333333333333"
const poolPair208 = "44444444444444444444444444444444"

type poolIdentityStore208 struct {
	*identityMemoryStore
	gets int
}

func (s *poolIdentityStore208) Get(service, account string) (string, error) {
	s.gets++
	return s.identityMemoryStore.Get(service, account)
}

func TestPoolIdentityPurposeSecretAndScopeAreIndependent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pool", "identity.json")
	store := &poolIdentityStore208{identityMemoryStore: newIdentityMemoryStore()}
	store.beforeSet = func(service, account, secret string) {
		if service != PoolRunnerIdentityProtocol || len(account) != 64 || len(secret) != 64 {
			t.Fatal("pool credential account leaked project purpose")
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("pool public metadata preceded secret persistence")
		}
	}
	h, err := PreparePoolRunnerPairingIdentity(file, "https://control.example/suma/", poolIdentity208, poolPair208, " owned Pool machine ", store)
	if err != nil {
		t.Fatal(err)
	}
	original, token := h.Identity(), h.Token()
	raw, _ := os.ReadFile(file)
	if original.Purpose != "pool_runner" || original.ControlURL != "https://control.example/suma" || bytes.Contains(raw, []byte(token)) || bytes.Contains(raw, []byte("project_id")) {
		t.Fatal("pool identity purpose or public secret boundary failed")
	}
	if _, err = LoadRunnerIdentity(file, store); err == nil {
		t.Fatal("Pool identity loaded as project identity")
	}
	_ = h.Close()
	h, err = PreparePoolRunnerPairingIdentity(file, "https://control.example/suma", poolIdentity208, poolPair208, "", store)
	if err != nil || h.Identity() != original || h.Token() != token || store.sets != 1 {
		t.Fatal("restart changed original Pool request/secret", err)
	}
	approved := poolPairingFixture208(original, 3, "approved")
	if err = h.SaveApproved(approved); err != nil {
		t.Fatal(err)
	}
	_ = h.Close()
	for _, scope := range [][3]string{{"https://other.example", poolIdentity208, approved.RunnerID}, {original.ControlURL, strings.Repeat("5", 32), approved.RunnerID}, {original.ControlURL, poolIdentity208, strings.Repeat("6", 32)}} {
		before, gets := len(store.values), store.gets
		if handle, e := LoadPoolRunnerIdentity(file, scope[0], scope[1], scope[2], store); e == nil {
			_ = handle.Close()
			t.Fatal("identity adopted foreign scope")
		}
		if len(store.values) != before || store.gets != gets {
			t.Fatal("scope failure accessed a secret")
		}
	}
	h, err = LoadPoolRunnerIdentity(file, original.ControlURL, poolIdentity208, approved.RunnerID, store)
	if err != nil || h.Token() != token || h.Identity().CredentialVersion != 1 || h.Identity().MaxSlots != 2 {
		t.Fatal("approved scope did not use original secret", err)
	}
	_ = h.Close()
	before := store.sets
	store.values = map[string]string{}
	if handle, e := LoadPoolRunnerIdentity(file, original.ControlURL, poolIdentity208, approved.RunnerID, store); e == nil {
		_ = handle.Close()
		t.Fatal("missing secret was silently replaced")
	}
	if store.sets != before {
		t.Fatal("secret loss issued replacement")
	}
}

func TestPoolIdentityCorruptAndOrphanMetadataNeverReissuesSecret(t *testing.T) {
	for _, kind := range []string{"unknown", "null", "duplicate", "half_approval", "wrong_purpose", "orphan", "replaced_parent"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "owned", "identity.json")
			store := newIdentityMemoryStore()
			h, err := PreparePoolRunnerPairingIdentity(file, "https://control.example", poolIdentity208, poolPair208, "machine", store)
			if err != nil {
				t.Fatal(err)
			}
			_ = h.Close()
			raw, _ := os.ReadFile(file)
			var value map[string]any
			_ = json.Unmarshal(raw, &value)
			switch kind {
			case "unknown":
				value["token"] = "never-authority"
			case "null":
				value["purpose"] = nil
			case "half_approval":
				value["runner_id"] = strings.Repeat("7", 32)
			case "wrong_purpose":
				value["purpose"] = "project_runner"
			case "duplicate":
				raw = append([]byte(`{"pool_id":"`+poolIdentity208+`",`), bytes.TrimSpace(raw)[1:]...)
			case "orphan":
				if err = os.Remove(file); err != nil {
					t.Fatal(err)
				}
			case "replaced_parent":
				// A live handle pins the original parent, even when copied bytes match.
				h, err = PreparePoolRunnerPairingIdentity(file, "https://control.example", poolIdentity208, poolPair208, "machine", store)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.Rename(filepath.Dir(file), filepath.Join(root, "moved")); err != nil {
					if runtime.GOOS == "windows" && os.IsPermission(err) {
						// Windows may prevent the parent exchange while the exact
						// identity lock is open. That is a prevented substitution,
						// not evidence that an exchanged directory was accepted.
						current, e := os.ReadFile(file)
						if e != nil || !bytes.Equal(current, raw) {
							t.Fatal("blocked parent exchange changed original metadata")
						}
						_ = h.Close()
						return
					}
					_ = h.Close()
					t.Fatal(err)
				}
				if err = os.Mkdir(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err = h.SaveApproved(poolPairingFixture208(h.Identity(), 3, "approved")); err == nil {
					t.Fatal("substituted parent accepted")
				}
				_ = h.Close()
				return
			}
			if kind != "orphan" {
				if kind != "duplicate" {
					raw, _ = json.Marshal(value)
				}
				if err = os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if handle, e := PreparePoolRunnerPairingIdentity(file, "https://control.example", poolIdentity208, poolPair208, "machine", store); e == nil {
				_ = handle.Close()
				t.Fatal("invalid metadata/orphan was silently replaced")
			}
			if store.sets != 1 {
				t.Fatal("failure reissued original Pool secret")
			}
		})
	}
}
