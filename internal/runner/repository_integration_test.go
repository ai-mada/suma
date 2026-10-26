package runner

import (
	"context"
	"encoding/json"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func bridgeAssignment() Assignment {
	source := cc.CandidateBinding{SchemaVersion: 1, TransferID: "transfer", ProjectID: "project", State: "sealed", Source: cc.CandidateAssignmentRef{RunID: "source", NodeID: "develop", Attempt: 1, Fence: 1, WorkerID: "source-worker"}, BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("c", 64), BundleSHA256: strings.Repeat("d", 64), SizeBytes: 1}
	e := rc.RepositoryExecution{Protocol: rc.RepositoryProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "provider", TargetID: "target", TargetAlias: "main", TargetRef: "refs/heads/main", TargetConfigSHA256: strings.Repeat("e", 64), PlanID: "plan", PlanVersion: 1, Phase: "prepare", OperationID: "repository-00001", Source: source, ExpectedTargetHead: source.BaseCommit, Generation: 1}
	return Assignment{RunID: "run", NodeID: "prepare", Attempt: 1, Fence: 1, WorkerID: "worker", Type: "external_workflow", Config: map[string]any{"purpose": "repository_integration"}, RepositoryIntegration: &e}
}
func TestRepositoryBridgeTypedEvidenceAndRecovery(t *testing.T) {
	for _, state := range []string{"prepared", "unknown", "blocked", "applied", "not_applied", "apply_blocked", "recovered_prepare", "existing_apply_unknown"} {
		t.Run(state, func(t *testing.T) {
			a := bridgeAssignment()
			if state == "applied" || state == "not_applied" || state == "apply_blocked" || state == "existing_apply_unknown" {
				a.RepositoryIntegration.Phase = "apply"
				a.RepositoryIntegration.PreparedSHA256 = strings.Repeat("f", 64)
			}
			q := rc.RepositoryRequest{RepositoryExecution: *a.RepositoryIntegration, Assignment: candidateRef(a)}
			hash, _ := rc.RepositoryRequestDigest(q)
			o := rc.RepositoryObservation{Protocol: rc.RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: hash, Phase: q.Phase, State: state, Version: 1, Execution: q.RepositoryExecution}
			if state == "recovered_prepare" {
				o.State = "prepared"
			}
			if state == "existing_apply_unknown" {
				o.State = "unknown"
			}
			if state == "apply_blocked" {
				o.State = "blocked"
			}
			if state == "applied" || state == "not_applied" {
				result := ""
				if state == "applied" {
					result = q.Source.HeadCommit
				}
				receipt := rc.RepositoryReceipt{Protocol: rc.RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: hash, TargetID: q.TargetID, TargetAlias: q.TargetAlias, TargetRef: q.TargetRef, TargetConfigSHA256: q.TargetConfigSHA256, BeforeCommit: q.ExpectedTargetHead, ResultCommit: result, SourceBundleSHA256: q.Source.BundleSHA256, PreparedSHA256: q.PreparedSHA256, Generation: q.Generation, EffectGrantID: "fixed-effect", State: state}
				o.Receipt = &receipt
				o.ReceiptOID, _ = rc.RepositoryReceiptOID(receipt, "sha1")
				o.ReceiptSHA256, _ = rc.RepositoryReceiptDigest(receipt)
				o.EffectGrantID = receipt.EffectGrantID
			}
			if state == "prepared" || state == "recovered_prepare" {
				p := rc.RepositoryPreview{TargetHead: q.ExpectedTargetHead, CandidateHead: q.Source.HeadCommit, Files: []rc.RepositoryFileChange{}, Diff: "", DiffSHA256: digest(nil), SourceBundleSHA256: q.Source.BundleSHA256, ConfigSHA256: q.TargetConfigSHA256, FastForward: true}
				p.PreparedSHA256, _ = rc.RepositoryPreviewDigest(p)
				o.Preview = &p
			}
			o.ObservationSHA256, _ = rc.RepositoryObservationDigest(o)
			posts := 0
			authorizations := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if state == "recovered_prepare" {
						preparing := o
						preparing.State = "preparing"
						preparing.Preview = nil
						preparing.ObservationSHA256, _ = rc.RepositoryObservationDigest(preparing)
						_ = json.NewEncoder(w).Encode(preparing)
						return
					}
					if state == "existing_apply_unknown" {
						_ = json.NewEncoder(w).Encode(o)
						return
					}
					w.WriteHeader(404)
					return
				}
				posts++
				if state == "recovered_prepare" {
					var sent rc.RepositoryRequest
					_ = json.NewDecoder(r.Body).Decode(&sent)
					if sent.AuthorizationTicket != "" {
						t.Error("historical consumed intent fetched a new ticket")
					}
				}
				_ = json.NewEncoder(w).Encode(o)
			}))
			defer provider.Close()
			control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authorizations++
				if r.URL.Path != "/api/v1/runner/repository-integrations/authorize" {
					t.Error(r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(rc.RepositoryAuthorization{AuthorizationID: "auth", AuthorizationTicket: "ticket", ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339Nano), OperationID: q.OperationID, RequestSHA256: hash})
			}))
			defer control.Close()
			t.Setenv("REPO_BRIDGE_TOKEN", strings.Repeat("r", 32))
			r := Execute(context.Background(), a, ExecConfig{Artifacts: &Client{BaseURL: control.URL, Token: "runner", ProjectID: "project"}, External: map[string]ExternalSpec{"provider": {URL: provider.URL, TokenEnv: "REPO_BRIDGE_TOKEN", Protocol: rc.RepositoryProtocol}}})
			if state == "existing_apply_unknown" {
				if !r.OutcomeUnknown || posts != 0 || authorizations != 0 {
					t.Fatal("existing unknown apply was redispatched")
				}
				return
			}
			if state == "recovered_prepare" && authorizations != 0 {
				t.Fatal("consumed prepare requested another authorization")
			}
			if posts != 1 || r.IntegrationEvidence == nil || r.ReleaseEvidence != nil || r.ToolEvidence != nil || r.OutcomeUnknown != (state == "unknown" || state == "apply_blocked") {
				t.Fatalf("bridge %#v posts%d", r, posts)
			}
			if state == "prepared" && r.ExitCode != 0 {
				t.Fatal(r.Error)
			}
			raw, _ := json.Marshal(r)
			if !strings.Contains(string(raw), "integration_evidence") {
				t.Fatal("wire omitted evidence")
			}
		})
	}
	a := bridgeAssignment()
	dir := filepath.Join(t.TempDir(), "journal")
	j, e := OpenJournal(dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Accept(a); e != nil {
		t.Fatal(e)
	}
	j.Close()
	j, e = OpenJournal(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	if len(j.pendingReleases()) != 1 || len(j.Pending()) != 0 {
		t.Fatal("restart flattened managed repository operation to unknown")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "journal.json"))
	if e != nil || strings.Contains(string(raw), "authorization_ticket") {
		t.Fatal("ticket persisted in journal")
	}
}
func TestRepositoryBridgePoolProtocolBoundary(t *testing.T) {
	c := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant", ProjectID: "project", Workspace: ".", Commands: map[string]CommandSpec{}, External: map[string]ExternalSpec{"repo": {Protocol: rc.RepositoryProtocol, URL: "http://127.0.0.1:4351", TokenEnv: "LOCAL_REPO_TOKEN"}}}}}
	if e := validatePoolLocalConfig(c); e != nil {
		t.Fatal(e)
	}
	for _, protocol := range []string{"", "suma.release.v1"} {
		c.Scopes[0].External["repo"] = ExternalSpec{Protocol: protocol, URL: "http://127.0.0.1:4351", TokenEnv: "LOCAL_REPO_TOKEN"}
		if validatePoolLocalConfig(c) == nil {
			t.Fatal("generic/release pool external enabled")
		}
	}
}
