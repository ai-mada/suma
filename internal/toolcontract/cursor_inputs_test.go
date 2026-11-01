package toolcontract

import (
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
)

func TestCursorInputCapabilityAndEvidenceStayWithOriginalExecution(t *testing.T) {
	p := acpTestProfile(t)
	p.Adapter, p.UserInputProtocol = "cursor", cu.InputProtocol
	p.ACPExecution.Runtime, p.ACPExecution.AuthStrategy = "cursor", "explicit_api_key_env"
	if ValidateProfile(p, true) != nil {
		t.Fatal("fixed Cursor input profile refused")
	}
	without := CloneProfile(p)
	without.UserInputProtocol = ""
	if SupportsACPExecutionProfile(without) {
		t.Fatal("missing current Cursor capability accepted")
	}
	qoder := CloneProfile(p)
	qoder.Adapter = "qoder"
	qoder.ACPExecution.Runtime = "qoder"
	qoder.ACPExecution.AuthStrategy = "existing_cli_login"
	if ValidateProfile(qoder, true) == nil {
		t.Fatal("Qoder acquired Cursor's native extensions")
	}
	input := cu.CursorInputEvidence{InputID: "input-190", SessionID: "original-session", RequestID: cu.CursorRPCRequestID{Kind: "string", Value: "request"}, PromptID: cu.CursorRPCRequestID{Kind: "number", Value: "2"}, Method: cu.CreatePlanMethod, Kind: "plan", ConfigurationSHA256: p.ACPExecution.ConfigurationSHA256, ParamsSHA256: strings.Repeat("b", 64), Observation: &delivery.Observation{Sequence: 1, ParamsSHA256: strings.Repeat("b", 64)}}
	e := Evidence{Profile: p, Terminal: "unknown", SessionID: input.SessionID, ACPExecution: &ac.Evidence{ConfigurationSHA256: p.ACPExecution.ConfigurationSHA256, RequestedModel: p.ACPExecution.Model, SelectedModel: p.ACPExecution.Model, ModelSelectionAcknowledged: true, SessionID: input.SessionID, ProtocolVersion: 1, PromptStarted: true, UserInputs: []cu.CursorInputEvidence{input}}}
	if err := validateACPEvidence(e); err != nil {
		t.Fatal(err)
	}
	e.ACPExecution.UserInputs[0].SessionID = "another-session"
	if validateACPEvidence(e) == nil {
		t.Fatal("input evidence borrowed another session")
	}
	e.ACPExecution.UserInputs[0] = input
	e.ACPExecution.UserInputs[0].Observation = &delivery.Observation{Sequence: 1, ParamsSHA256: input.ParamsSHA256, RequestResolved: true}
	if validateACPEvidence(e) == nil {
		t.Fatal("Cursor fabricated a Codex resolved ACK")
	}
	e.ACPExecution.UserInputs[0] = input
	e.ACPExecution.PermissionsRequested = 32
	if validateACPEvidence(e) == nil {
		t.Fatal("input requests bypassed the combined interaction cap")
	}
}
