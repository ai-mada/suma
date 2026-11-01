package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
	"github.com/zalando/go-keyring"
)

// This opt-in check uses only an owned ephemeral project, loopback server and
// credential account. It never lists, reads or deletes another user's entries.
type actualPairingVault struct {
	provider runner.RunnerSecretStore
	missing  map[string]bool
	created  map[string][2]string
}

func (v *actualPairingVault) Get(service, account string) (string, error) {
	value, err := v.provider.Get(service, account)
	if errors.Is(err, runner.ErrRunnerSecretNotFound) {
		v.missing[service+":"+account] = true
	}
	return value, err
}

func (v *actualPairingVault) Set(service, account, secret string) error {
	key := service + ":" + account
	if !v.missing[key] {
		return fmt.Errorf("refusing to overwrite an unowned system credential")
	}
	// Track the exact initially absent entry before Set, including a write whose
	// acknowledgement could fail. Cleanup never enumerates credential storage.
	v.created[key] = [2]string{service, account}
	return v.provider.Set(service, account, secret)
}

func TestActualRunnerPairingSystemVaultAndOriginalInventory(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("SUMA_TEST_RUNNER_PAIRING_VAULT") != "1" {
		t.Skip("explicit owned Windows system-credential acceptance only")
	}
	vault := &actualPairingVault{provider: runner.SystemRunnerSecretStore(), missing: map[string]bool{}, created: map[string][2]string{}}
	t.Cleanup(func() {
		for _, entry := range vault.created {
			if err := keyring.Delete(entry[0], entry[1]); err != nil && !errors.Is(err, keyring.ErrNotFound) {
				t.Error("owned system credential cleanup was not confirmed")
			}
			if _, err := vault.provider.Get(entry[0], entry[1]); !errors.Is(err, runner.ErrRunnerSecretNotFound) {
				t.Error("owned system credential still exists after cleanup")
			}
		}
	})
	f := newFixture(t)
	server := &Server{Service: f.s, Engine: f.e}
	host := httptest.NewUnstartedServer(server.Handler())
	server.PublicURL = "http://" + host.Listener.Addr().String()
	host.Start()
	defer host.Close()
	f.handler = host.Config.Handler
	p := pairingCreate(t, f)
	file := filepath.Join(t.TempDir(), "identity.json")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	approved := false
	result, err := runner.PairRemoteRunner(ctx, runner.RunnerPairingOptions{
		PairingID: p["id"].(string), ControlURL: server.PublicURL, ProjectID: f.project,
		IdentityPath: file, DeviceName: "Owned system-vault fixture", Secrets: vault,
		Observe: func(value runner.RunnerPairingResult) error {
			if value.State == "proposed" {
				view := f.call("GET", "/runner-pairings/"+value.PairingID+queryProject(f.project), nil, 200)
				if view["can_approve"] != true || value.Prepared {
					return fmt.Errorf("a proposal claimed registration before browser approval")
				}
				pairingApprove(t, f, view)
				approved = true
			}
			return nil
		},
	})
	if err != nil || !approved || !result.Prepared || result.State != "approved" || len(vault.created) != 1 {
		t.Fatal("real system storage and pairing did not produce one reviewed identity")
	}
	handle, err := runner.LoadRunnerIdentity(file, vault)
	if err != nil {
		t.Fatal("system credential could not be reloaded through the production provider")
	}
	defer handle.Close()
	identity := handle.Identity()
	raw, err := os.ReadFile(file)
	if err != nil || strings.Contains(string(raw), handle.Token()) || digest(handle.Token()) != identity.CredentialSHA256 || result.CredentialSHA256 != identity.CredentialSHA256 {
		t.Fatal("identity metadata contains a secret or does not match the system credential")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), handle.Token()) || identity.WorkerID != result.WorkerID {
		t.Fatal("pairing output exposed or replaced its original credential")
	}
	journal, err := runner.OpenJournal(file + ".journal")
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	worker := runner.Worker{ID: identity.WorkerID, Client: &runner.Client{BaseURL: identity.ControlURL, ProjectID: identity.ProjectID, Token: handle.Token()}, Journal: journal}
	report, recovered, err := worker.ReportTools(ctx)
	if err != nil || recovered || report.WorkerID != identity.WorkerID || len(report.Items) != 0 {
		t.Fatal("approved identity did not authorize the original one-time empty inventory report")
	}
	if domainACount(t, f.s, "suma_release_workers") != 1 || domainACount(t, f.s, "suma_credential_grants") != 1 || domainACount(t, f.s, "dag_runs") != 0 {
		t.Fatal("pairing/report duplicated authority or unexpectedly started a task")
	}
	t.Log("real Windows system provider: one owned credential set/reloaded; actual loopback proposal/status and original empty inventory ACK; one fixture browser approval; zero task/model; exact owned credential cleanup required")
}
