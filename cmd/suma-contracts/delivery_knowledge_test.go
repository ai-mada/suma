package main

import "testing"

func TestDeliveryKnowledgeContractPreservesTypedSourceAndReadOnlyPreview(t *testing.T) {
	api, _ := generate()
	models := api["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{"DeliveryKnowledgeSourceView", "DeliveryKnowledgeBasis", "DeliveryKnowledgeTestEvidence", "DeliveryKnowledgeCandidateEvidence"} {
		value, ok := models[name].(map[string]any)
		if !ok || value["additionalProperties"] != false {
			t.Fatalf("missing closed provenance schema %s", name)
		}
	}
	input := toolContractMap(models, "ProjectKnowledgeSourceInput", "properties")
	if input["basis_sha256"] == nil || input["delivery"] != nil {
		t.Fatal("client proof was accepted or exact preview digest was lost")
	}
	for _, name := range []string{"ProjectKnowledgeSource", "ProjectKnowledgeContextSource"} {
		if toolContractMap(models, name, "properties")["delivery"] == nil {
			t.Fatalf("provenance was dropped from %s", name)
		}
	}
	preview := toolContractMap(api, "paths", "/tasks/{id}/delivery-knowledge-source", "get")
	if preview["x-read-only"] != true || preview["x-no-agent-effects"] != true || preview["x-response-cache-control"] != "no-store" {
		t.Fatal("source preview became a write or agent operation")
	}
}
