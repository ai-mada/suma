package acpcontract

import (
	"strings"
	"testing"
)

func TestCursorCatalogueSeparatesCancelWriteAndManagedHostStop(t *testing.T) {
	catalogue, exit := discoveryCatalogue(), 1
	r := ACPModelDiscoveryResult{Protocol: ModelDiscoveryProtocol, Runtime: "cursor", State: "ready", ExecutableSHA256: strings.Repeat("a", 64), ToolVersion: "fixed191", Catalogue: &catalogue, Evidence: ACPModelDiscoveryEvidence{SessionCreated: true, SessionCancelRequested: true, SessionCancelWritten: true, HostProcessStopped: true, HostProcessTreeStopped: true, HostProcessExitCode: &exit, HostStoppedAfterRPC: true}}
	if err := ValidateModelDiscoveryResult(r); err != nil {
		t.Fatal("controlled host stop lost valid catalogue", err)
	}
	for _, mutate := range []func(*ACPModelDiscoveryResult){
		func(v *ACPModelDiscoveryResult) { v.Runtime = "qoder" },
		func(v *ACPModelDiscoveryResult) { v.Evidence.SessionCancelWritten = false },
		func(v *ACPModelDiscoveryResult) { v.Evidence.HostStoppedAfterRPC = false },
		func(v *ACPModelDiscoveryResult) { v.Evidence.HostProcessTreeStopped = false },
		func(v *ACPModelDiscoveryResult) { v.Evidence.HostProcessExitCode = nil },
		func(v *ACPModelDiscoveryResult) { v.Evidence.PromptRequests = 1 },
	} {
		v := r
		mutate(&v)
		if ValidateModelDiscoveryResult(v) == nil {
			t.Fatal("unproved managed stop admitted", v.Evidence)
		}
	}
	if exit != 1 {
		t.Fatal("native exit was silently normalized")
	}
	r.State, r.Catalogue, r.ErrorCode = "unknown", nil, "acp_discovery_unknown"
	r.Evidence = ACPModelDiscoveryEvidence{HostProcessExitCode: &exit}
	if ValidateModelDiscoveryResult(r) == nil {
		t.Fatal("exit status without a stopped host accepted")
	}
}
