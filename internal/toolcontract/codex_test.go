package toolcontract

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"strings"
	"testing"
)

func TestCodexCatalogAndEvidenceCannotAcquireNativeAuthority(t *testing.T) {
	profile := Profile{CommandID: "codex", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "installed"}
	catalog, _ := cc.NewCatalog(strings.Repeat("c", 64), []cc.ModelOption{{Model: "model-a", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "low"}})
	o := Observation{Profile: profile, Available: true, CodexCatalog: &catalog}
	if ValidateObservation(o) != nil {
		t.Fatal("actual catalog rejected")
	}
	o.CodexCatalog = nil
	if ValidateObservation(o) == nil {
		t.Fatal("configured-only app-server marked available")
	}
	o.CodexCatalog = &catalog
	o.WorkspaceObservationProtocol = "workspace-observation-v1"
	if ValidateObservation(o) == nil {
		t.Fatal("external CLI acquired native broker")
	}
	b, _ := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "model-a", ReasoningEffort: "low"})
	proof := &cc.Evidence{BindingSHA256: b.BindingSHA256, ThreadID: "thread-1", TurnID: "turn-1", RequestedModel: "model-a", RequestedReasoningEffort: "low", AcknowledgedModel: "model-a", AcknowledgedReasoningEffort: "low", AcknowledgedProvider: "owned", ConfigurationAcknowledged: true}
	in, out := int64(23), int64(17)
	e := Evidence{Profile: profile, SessionID: "thread-1", Terminal: "completed", EventCount: 3, Usage: &Usage{InputTokens: &in, OutputTokens: &out}, UsageState: UsagePartial, CodexExecution: proof}
	if ValidateEvidence(e) != nil {
		t.Fatal("partial configuration evidence rejected")
	}
	e.UsageState = UsageComplete
	if ValidateEvidence(e) == nil {
		t.Fatal("external aggregate claimed complete native coverage")
	}
	e.UsageState = UsagePartial
	e.SessionID = "other-thread"
	if ValidateEvidence(e) == nil {
		t.Fatal("cross-thread evidence accepted")
	}
}
