package codexcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func reviewedSkill(t *testing.T) *ExecutionSkillBundle {
	t.Helper()
	b, err := BuildExecutionSkillBundle([]ExecutionSkillDocument{{ID: "reviewed-skill", Revision: 3, SHA256: strings.Repeat("a", 64), Title: "已审阅技能", Description: "Use for \"code: review\"\nand quality gates", Content: "Keep exact content.\r\n先检查当前权限。\n---\nDo not grant access."}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNativeSkillContractBindsIndependentDocumentBytes(t *testing.T) {
	b := reviewedSkill(t)
	d := b.Documents[0]
	raw, err := RenderExecutionSkill(d)
	if err != nil || !strings.Contains(string(raw), `description: "Use for \"code: review\"\nand quality gates"`) || !strings.HasSuffix(string(raw), d.Content+"\n") {
		t.Fatal("metadata escaping or exact body changed", err)
	}
	if d.SHA256 == d.ContentSHA256 || d.ContentSHA256 == d.DocumentSHA256 || d.DocumentSHA256 == b.BundleSHA256 {
		t.Fatal("different digest objects were conflated")
	}
	c := CloneExecutionSkillBundle(b)
	c.Documents[0].Content = "changed"
	if ValidateExecutionSkillBundle(*c) == nil || b.Documents[0].Content != d.Content {
		t.Fatal("changed body accepted or mutable copy leaked")
	}
	for _, mutate := range []func(*ExecutionSkillBundle){
		func(v *ExecutionSkillBundle) { v.Documents[0].Revision = 0 },
		func(v *ExecutionSkillBundle) { v.Documents[0].Name = "../escape" },
		func(v *ExecutionSkillBundle) { v.Documents[0].ContentSHA256 = strings.Repeat("b", 64) },
		func(v *ExecutionSkillBundle) { v.Documents[0].DocumentSHA256 = strings.Repeat("b", 64) },
		func(v *ExecutionSkillBundle) { v.Documents = append(v.Documents, v.Documents[0]) },
	} {
		copy := CloneExecutionSkillBundle(b)
		mutate(copy)
		copy.BundleSHA256 = ExecutionSkillBundleDigest(*copy)
		if ValidateExecutionSkillBundle(*copy) == nil {
			t.Fatal("self-consistent envelope hid an invalid document")
		}
	}
}

func TestNativeSkillContractRejectsUnknownAndOversizedMaterial(t *testing.T) {
	b := reviewedSkill(t)
	raw, _ := json.Marshal(b)
	var wire map[string]any
	if json.Unmarshal(raw, &wire) != nil {
		t.Fatal("fixture encoding")
	}
	wire["cwd"] = "/user/home"
	if _, err := DecodeExecutionSkillBundle(wire); err == nil {
		t.Fatal("caller path entered protected bundle")
	}
	doc := b.Documents[0]
	doc.Content = strings.Repeat("x", 16000)
	second := doc
	second.ID = "second-reviewed-skill"
	if _, err := BuildExecutionSkillBundle([]ExecutionSkillDocument{doc, second}); err == nil {
		t.Fatal("native documents bypassed total byte limit")
	}
	doc.Content = string([]byte{0xff})
	if _, err := BuildExecutionSkillBundle([]ExecutionSkillDocument{doc}); err == nil {
		t.Fatal("invalid UTF8 was normalized instead of rejected")
	}
	if none, err := BuildExecutionSkillBundle(nil); err != nil || none != nil {
		t.Fatal("empty selection must retain the no-skills path")
	}
}

func TestNativeSkillEvidenceDoesNotPromoteDiscoveryToInvocation(t *testing.T) {
	b := reviewedSkill(t)
	e := ExecutionSkillEvidence{BundleSHA256: b.BundleSHA256, Documents: ExecutionSkillReceipts(*b), Materialized: true, CatalogAcknowledged: true}
	if !ExecutionSkillEvidenceMatchesBundle(*b, e) || e.InvocationRequested || e.TurnAcknowledged || e.FilesVerified {
		t.Fatal("discovery invented use or full completion")
	}
	e.Materialized = false
	if ValidateExecutionSkillEvidence(e) == nil {
		t.Fatal("discovery without host material was accepted")
	}
	e.Materialized, e.InvocationRequested, e.TurnAcknowledged, e.FilesVerified = true, true, true, true
	copy := CloneExecutionSkillEvidence(&e)
	copy.Documents[0].Revision++
	if ExecutionSkillEvidenceMatchesBundle(*b, *copy) || e.Documents[0].Revision != 3 {
		t.Fatal("receipt drift or shared mutable evidence")
	}
	if ValidateEvidence(Evidence{BindingSHA256: strings.Repeat("a", 64), RequestedModel: "model", Skills: &e}) == nil {
		t.Fatal("skill ACK bypassed parent thread configuration ACK")
	}
}
