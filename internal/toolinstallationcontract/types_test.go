package toolinstallationcontract

import (
	"strings"
	"testing"
)

func TestInstallationScanContractPreservesClosedLifecycleAndExactCoverage(t *testing.T) {
	cap := Capability{Protocol: Protocol, AdapterIDs: AdapterIDs()}
	if ValidateCapability(cap) != nil {
		t.Fatal("fixed directory rejected")
	}
	for _, ids := range [][]string{nil, {}, {"claude"}, {"codex-jsonl", "codex-jsonl"}} {
		if ValidateCapability(Capability{Protocol: Protocol, AdapterIDs: ids}) == nil {
			t.Fatal("invalid capability accepted", ids)
		}
	}
	scan := Scan{ID: "scan", ProjectID: "project", WorkerID: "worker", AdapterIDs: []string{"codebuddy-cli"}, InstanceID: "instance", Version: 1, State: "pending", RequestedBy: "user", CreatedAt: "2026-11-02T08:00:00Z", ExpiresAt: "2026-11-02T08:05:00Z"}
	scan.SourceSHA256 = ScanSourceSHA256(scan)
	if ValidateScan(scan) != nil {
		t.Fatal("valid pending source rejected")
	}
	source := scan.SourceSHA256
	scan.State = "claimed"
	scan.Version++
	scan.Fence = 1
	scan.ClaimID = "claim"
	scan.ClaimedAt = scan.CreatedAt
	scan.LeaseUntil = "2026-11-02T08:01:00Z"
	if ScanSourceSHA256(scan) != source || ValidateScan(scan) != nil {
		t.Fatal("claim changed original source")
	}
	proof := StopProof{Protocol: StopProtocol, Scan: Reference{ScanID: scan.ID, ClaimID: scan.ClaimID, Fence: scan.Fence}, SourceSHA256: source, ReceiptSHA256: strings.Repeat("a", 64), ProbeReturned: true, AllNativeTreesStopped: true, ObservedAt: scan.CreatedAt}
	item := Item{AdapterID: "codebuddy-cli", Protocol: AdapterProtocol("codebuddy-cli"), ReasonCode: "not_installed"}
	req := ResultRequest{ProjectID: scan.ProjectID, WorkerID: scan.WorkerID, Scan: proof.Scan, IdempotencyKey: "result", Outcome: "completed", Items: []Item{item}, StopProof: &proof}
	if ValidateResultRequest(req) != nil {
		t.Fatal("complete zero-native scan rejected")
	}
	scan.State = "completed"
	scan.Version++
	scan.FinishedAt = "2026-11-02T08:00:02Z"
	scan.ResultCode = "scan_completed"
	scan.Result = &Result{Items: req.Items, StopProof: &proof}
	if ValidateScan(scan) != nil {
		t.Fatal("complete source rejected")
	}
	wrong := scan
	wrong.Result = &Result{Items: []Item{}, StopProof: &proof}
	if ValidateScan(wrong) == nil {
		t.Fatal("partial coverage claimed complete")
	}
	wrong = scan
	copy := proof
	copy.Scan.Fence++
	wrong.Result = &Result{Items: req.Items, StopProof: &copy}
	if ValidateScan(wrong) == nil {
		t.Fatal("proof borrowed another fence")
	}
	for _, mutate := range []func(*StopProof){func(v *StopProof) { v.AllNativeTreesStopped = false }, func(v *StopProof) { v.ProbeReturned = false }, func(v *StopProof) { v.NativeProcessCount = 1 }, func(v *StopProof) { v.ReceiptSHA256 = "bad" }} {
		copy := proof
		mutate(&copy)
		if ValidateStopProof(copy) == nil {
			t.Fatal("false stopped proof accepted")
		}
	}
	req.Outcome = "unknown"
	req.FailureCode = "worker_restarted"
	req.Items = []Item{}
	req.StopProof = nil
	if ValidateResultRequest(req) != nil {
		t.Fatal("unknown observation rejected")
	}
	req.Outcome = "closed_unknown"
	if ValidateResultRequest(req) == nil {
		t.Fatal("unknown closed without proof")
	}
	req.StopProof = &proof
	if ValidateResultRequest(req) != nil {
		t.Fatal("original stopped proof could not close unknown")
	}
	for _, bad := range []Item{{AdapterID: "codebuddy-cli", Protocol: "claude-sdk-v1", ReasonCode: "not_installed"}, {AdapterID: "codebuddy-cli", Protocol: AdapterProtocol("codebuddy-cli"), LaunchSupported: true, ReasonCode: "not_installed"}, {AdapterID: "claude-code-sdk", Protocol: AdapterProtocol("claude-code-sdk"), Installed: true, ReasonCode: "configuration_required"}} {
		if ValidateItem(bad) == nil {
			t.Fatal("contradictory installation facts accepted")
		}
	}
}
