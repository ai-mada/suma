package codexcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexCatalogAndSelectionFreezePublicConfiguration(t *testing.T) {
	source := []ModelOption{{Model: "model-b", ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low"}, {Model: "model-a", ReasoningEfforts: []string{}, DefaultReasoningEffort: ""}}
	c, err := NewCatalog(strings.Repeat("a", 64), source)
	if err != nil {
		t.Fatal(err)
	}
	source[0].ReasoningEfforts[0] = "mutated"
	if c.Models[0].Model != "model-a" || c.Models[1].ReasoningEfforts[0] != "high" || ValidateCatalog(c) != nil {
		t.Fatal("catalog retained mutable source or noncanonical order")
	}
	limit := int64(40)
	s := Selection{CatalogSHA256: c.CatalogSHA256, Model: "model-b", ReasoningEffort: "low", TokenNoticeLimit: &limit}
	b, err := NewBinding(c, s)
	if err != nil {
		t.Fatal(err)
	}
	limit = 50
	if *b.TokenNoticeLimit != 40 || b.BudgetMode != BudgetMode || b.IdentityEvidence != IdentityEvidence || ValidateBinding(b) != nil {
		t.Fatal("selection did not freeze the public configuration")
	}
	bad := s
	bad.ReasoningEffort = "unsupported"
	if _, err = NewBinding(c, bad); err == nil {
		t.Fatal("unknown effort accepted")
	}
	bad = s
	bad.CatalogSHA256 = strings.Repeat("b", 64)
	if _, err = NewBinding(c, bad); err == nil {
		t.Fatal("old catalog accepted")
	}
	c.Models = append(c.Models, c.Models[0])
	c.CatalogSHA256 = CatalogDigest(c)
	if ValidateCatalog(c) == nil {
		t.Fatal("duplicate model accepted")
	}
	b.BudgetMode = "enforce"
	b.BindingSHA256 = BindingDigest(b)
	if ValidateBinding(b) == nil {
		t.Fatal("configuration made a hard budget claim")
	}
}
func TestCodexSelectionDecodeRejectsPrivateAndAmbiguousAmounts(t *testing.T) {
	prefix := `{"catalog_sha256":"` + strings.Repeat("a", 64) + `","model":"model-a","reasoning_effort":"low"`
	for _, ending := range []string{`,"api_key":"private"}`, `,"token_notice_limit":"40"}`, `,"token_notice_limit":1e2}`, `,"token_notice_limit":0}`, `,"token_notice_limit":9007199254740992}`, `,"model":"model-b"}`, `,"cost_limit_micros":100}`, `,"max_output_tokens":100}`} {
		if _, err := DecodeSelection(json.RawMessage(prefix + ending)); err == nil {
			t.Fatal("invalid or private selection accepted", ending)
		}
	}
	if _, err := DecodeSelection(json.RawMessage(prefix + `,"token_notice_limit":40}`)); err != nil {
		t.Fatal(err)
	}
}
func TestCodexEvidenceAcknowledgementRemainsConfigurationOnly(t *testing.T) {
	c, _ := NewCatalog(strings.Repeat("a", 64), []ModelOption{{Model: "model-a", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "low"}})
	b, _ := NewBinding(c, Selection{CatalogSHA256: c.CatalogSHA256, Model: "model-a", ReasoningEffort: "low"})
	e := Evidence{BindingSHA256: b.BindingSHA256, ThreadID: "thread-1", RequestedModel: b.Model, RequestedReasoningEffort: b.ReasoningEffort, AcknowledgedModel: b.Model, AcknowledgedReasoningEffort: b.ReasoningEffort, AcknowledgedProvider: "owned", ConfigurationAcknowledged: true}
	if ValidateEvidence(e) != nil || EvidenceMatchesBinding(b, e) {
		t.Fatal("prepare ACK was treated as an actual turn")
	}
	e.TurnID = "turn-1"
	if !EvidenceMatchesBinding(b, e) {
		t.Fatal("matching completed-turn configuration rejected")
	}
	copy := CloneEvidence(&e)
	e.ReroutedModels = []string{"different-provider-model"}
	copy = CloneEvidence(&e)
	copy.ReroutedModels[0] = "mutated"
	if e.ReroutedModels[0] != "different-provider-model" {
		t.Fatal("journal clone aliases evidence")
	}
	e.AcknowledgedModel = "other-model"
	if ValidateEvidence(e) == nil {
		t.Fatal("wrong acknowledgement accepted")
	}
	e.AcknowledgedModel = b.Model
	e.InterruptAcknowledged = true
	if ValidateEvidence(e) == nil {
		t.Fatal("interrupt receipt without original request accepted")
	}
}
