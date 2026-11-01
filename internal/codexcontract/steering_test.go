package codexcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSteeringFixesOriginalTurnAndSeparatesWriteFromAcceptance(t *testing.T) {
	source := CodexSteeringSource{Protocol: SteeringProtocol, ThreadID: "thread", TurnID: "turn", BindingSHA256: strings.Repeat("a", 64)}
	p, err := BuildSteeringParams(source, "instruction", "  Keep the current API.\nUse the existing parser.  ")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	if string(raw) != `{"threadId":"thread","expectedTurnId":"turn","input":[{"type":"text","text":"  Keep the current API.\nUse the existing parser.  "}],"clientUserMessageId":"instruction"}` {
		t.Fatal("native input was rewritten or gained execution overrides", string(raw))
	}
	digest, err := SteeringParamsDigest(source, "instruction", p.Input[0].Text)
	if err != nil {
		t.Fatal(err)
	}
	changed, _ := SteeringParamsDigest(source, "instruction", "Different message")
	if digest == changed {
		t.Fatal("changed human input retained the old native digest")
	}
	o := CodexSteeringObservation{Sequence: 1, RequestID: CodexRPCRequestID{Kind: "number", Value: "20"}, ParamsSHA256: digest, Observed: true, WriteStarted: true, NativeAccepted: true}
	if ValidateSteeringObservation(o) != nil {
		t.Fatal("native ACK before full-write callback must remain representable")
	}
	o.NativeRejected = true
	if ValidateSteeringObservation(o) == nil {
		t.Fatal("accepted and rejected are different native outcomes")
	}
	o.NativeRejected = false
	o.WriteStarted = false
	if ValidateSteeringObservation(o) == nil {
		t.Fatal("native acceptance without a write was invented")
	}
	o.WriteStarted = true
	e := Evidence{BindingSHA256: source.BindingSHA256, ThreadID: source.ThreadID, TurnID: source.TurnID, RequestedModel: "fixture", AcknowledgedModel: "fixture", AcknowledgedProvider: "owned", ConfigurationAcknowledged: true, Steering: &CodexSteeringEvidence{SessionID: "session", Source: source, Closed: true, Instructions: []CodexSteeringInstructionEvidence{{InstructionID: "instruction", MessageSHA256: strings.Repeat("b", 64), Observation: &o}}}}
	if err = ValidateEvidence(e); err != nil {
		t.Fatal(err)
	}
	copy := CloneEvidence(&e)
	copy.Steering.Instructions[0].Observation.Sequence = 2
	if e.Steering.Instructions[0].Observation.Sequence != 1 {
		t.Fatal("journal copy aliases native delivery evidence")
	}
	e.Steering.Source.TurnID = "different"
	if ValidateEvidence(e) == nil {
		t.Fatal("steering escaped its acknowledged original turn")
	}
}
