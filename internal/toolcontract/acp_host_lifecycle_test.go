package toolcontract

import (
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
)

func TestCursorCompletionRequiresRPCAndIndependentHostStopFacts(t *testing.T) {
	p := acpTestProfile(t)
	p.Adapter, p.UserInputProtocol = "cursor", cu.InputProtocol
	p.ACPExecution.Runtime, p.ACPExecution.AuthStrategy = "cursor", "explicit_api_key_env"
	exit := 1
	e := Evidence{Profile: p, Terminal: "completed", SessionID: "original191", ACPExecution: &ac.Evidence{ConfigurationSHA256: p.ACPExecution.ConfigurationSHA256, RequestedModel: p.ACPExecution.Model, SelectedModel: p.ACPExecution.Model, ModelSelectionAcknowledged: true, SessionID: "original191", ProtocolVersion: 1, PromptStarted: true, PromptCompleted: true, StopReason: "end_turn", ToolsComplete: true, HostProcessStopped: true, HostProcessTreeStopped: true, HostProcessExitCode: &exit, HostStoppedAfterRPC: true}}
	if err := validateACPEvidence(e); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ac.Evidence){
		func(v *ac.Evidence) { v.HostStoppedAfterRPC = false },
		func(v *ac.Evidence) { v.HostProcessTreeStopped = false },
		func(v *ac.Evidence) { v.HostProcessExitCode = nil },
		func(v *ac.Evidence) { v.CancelRequested = true },
		func(v *ac.Evidence) { v.PromptCompleted = false },
		func(v *ac.Evidence) { v.StopReason = "refusal" },
	} {
		v := e
		copy := *e.ACPExecution
		v.ACPExecution = &copy
		mutate(v.ACPExecution)
		if validateACPEvidence(v) == nil {
			t.Fatal("unsafe completion accepted", v.ACPExecution)
		}
	}
	if *e.ACPExecution.HostProcessExitCode != 1 {
		t.Fatal("true host exit was lost")
	}
	e.Terminal = "unknown"
	e.ACPExecution.HostStoppedAfterRPC = false
	e.ACPExecution.HostProcessStopped = false
	e.ACPExecution.HostProcessTreeStopped = false
	if validateACPEvidence(e) == nil {
		t.Fatal("exit status without a stopped host accepted")
	}
}
