package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestModelGatewayJournalRecoversKnownBillingWithoutInventingACPStop(t *testing.T) {
	for _, kind := range []string{"open-unknown", "closed-known", "closed-zero"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			journal, err := OpenJournal(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			binding := testModelBinding(t, "https://models.example")
			profile := tc.Profile{CommandID: "octop-journal", Adapter: "octop", Protocol: tc.OctopProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: tc.OctopHarnessVersion, WorkspaceToolsVersion: "attempt-workspace-v1", ModelExecutionProtocol: mc.Protocol, OctopExecution: &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion}}
			a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "gateway-run", NodeID: "develop", Attempt: 1, Fence: 7, WorkerID: "worker", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"tool_profile_sha256": profile.ProfileSHA256, "model_execution": modelBindingValue(t, binding)}}
			if err = journal.Accept(a); err != nil {
				t.Fatal(err)
			}
			snapshot := mg.Snapshot{Version: 1, RequestID: digest([]byte(key(a))), Binding: binding, Records: []mg.Record{}}
			if err = journal.checkpointModelGateway(a, profile, snapshot); err != nil {
				t.Fatal("initial zero-request checkpoint failed", err)
			}
			foreign := a
			foreign.ProjectID = "another-project"
			if journal.checkpointModelGateway(foreign, profile, snapshot) == nil {
				t.Fatal("private key collision admitted a different project")
			}
			started := mg.Record{RequestEvidence: mc.RequestEvidence{State: "started", Step: 0, RequestSHA256: strings.Repeat("c", 64), OutputAllowance: 10, UsageState: "unreported"}, ClientSHA256: strings.Repeat("d", 64)}
			// A failed durable admission must not leave a newer in-memory receipt.
			path := journal.path
			journal.path = filepath.Join(t.TempDir(), "missing", "journal.json")
			failed := snapshot
			failed.Records = []mg.Record{started}
			if journal.checkpointModelGateway(a, profile, failed) == nil || len(journal.entries[key(a)].ModelGateway.Snapshot.Records) != 0 {
				t.Fatal("failed save replaced the original durable checkpoint")
			}
			journal.path = path
			input, output := int64(2), int64(3)
			if kind != "closed-zero" {
				completed := started
				completed.State, completed.ResponseSHA256, completed.ProviderModel, completed.HTTPStatus = "completed", strings.Repeat("e", 64), binding.Deployment.Model, 200
				completed.UsageState, completed.Usage = "complete", &mc.RequestUsage{InputTokens: &input, OutputTokens: &output}
				snapshot.Records = []mg.Record{completed}
			}
			if kind == "open-unknown" {
				pending := started
				pending.Step, pending.RequestSHA256, pending.ClientSHA256 = 1, strings.Repeat("f", 64), strings.Repeat("a", 64)
				snapshot.Records = append(snapshot.Records, pending)
			} else {
				snapshot.Closed = true
			}
			if err = journal.checkpointModelGateway(a, profile, snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Closed {
				late := snapshot
				request := started
				request.State, request.Step = "unknown", len(snapshot.Records)
				late.Records = append(append([]mg.Record{}, snapshot.Records...), request)
				if journal.checkpointModelGateway(a, profile, late) == nil {
					t.Fatal("closed gateway receipt admitted another request")
				}
			}
			// Caller reuse cannot rewrite counters after the journal accepted them.
			output = 999
			if err = journal.Close(); err != nil {
				t.Fatal(err)
			}
			journal, err = OpenJournal(directory)
			if err != nil {
				t.Fatal("recovery rejected the original host checkpoint", err)
			}
			defer journal.Close()
			results := journal.Pending()
			if len(results) != 1 || !results[0].OutcomeUnknown || results[0].Stopped || results[0].ExitCode == 0 || results[0].ToolEvidence == nil {
				t.Fatal("restart became code success or lost known model facts", results)
			}
			evidence := results[0].ToolEvidence
			if err = tc.ValidateEvidence(*evidence); err != nil {
				t.Fatal("recovered facts do not meet the public contract", err)
			}
			proof := evidence.OctopExecution
			if proof == nil || proof.BindingSHA256 != binding.BindingSHA256 || proof.SessionID != "" || proof.StopReason != "" || proof.CompletedAt != "" || proof.ProcessStopped || proof.CancelRequested {
				t.Fatal("restart fabricated ACP session, cancellation or process-stop evidence", proof)
			}
			if evidence.ModelExecution.GatewayClosed != snapshot.Closed || evidence.Usage == nil || evidence.Usage.InputTokens == nil || evidence.Usage.OutputTokens == nil {
				t.Fatal("restart changed gateway coverage or lost known counters", evidence)
			}
			wantInput, wantOutput := int64(2), int64(3)
			if kind == "closed-zero" {
				wantInput, wantOutput = 0, 0
			}
			if *evidence.Usage.InputTokens != wantInput || *evidence.Usage.OutputTokens != wantOutput {
				t.Fatal("caller mutation or restart erased original counters", evidence.Usage)
			}
			if kind == "open-unknown" {
				if evidence.UsageState != tc.UsagePartial || evidence.ModelExecution.Requests[1].State != "unknown" {
					t.Fatal("in-flight request was completed or forgotten", evidence)
				}
			} else if evidence.UsageState != tc.UsageComplete {
				t.Fatal("known closed billing was degraded because code outcome is unknown", evidence)
			}
			raw, err := os.ReadFile(filepath.Join(directory, "journal.json"))
			if err != nil || strings.Contains(string(raw), "api_key") || strings.Contains(string(raw), "Bearer ") {
				t.Fatal("checkpoint contains credential carrier data", err)
			}
		})
	}
}
