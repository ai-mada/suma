package platform

import "github.com/ai-mada/suma/internal/toolcontract"

// Pricing identifies the immutable configured model, never the model named by
// an untrusted CLI result. Specialized read-only purposes share the same quota
// and accounting transaction; adding a purpose must not create a free lane.
func usagePricedModel(profile ToolProfile) (model, origin string, ok bool) {
	p := profile.Profile
	if profile.State != "enabled" || p.Adapter != "suma" || !p.ReadOnly || p.WorkspaceToolsVersion != "" || p.Planning != nil && p.KnowledgeExtraction != nil {
		return "", "", false
	}
	if p.Planning != nil && p.Planning.Protocol == toolcontract.PlanningProtocol {
		return p.Planning.Model, p.Planning.EndpointOrigin, true
	}
	if p.KnowledgeExtraction != nil && p.KnowledgeExtraction.Protocol == toolcontract.KnowledgeExtractionProtocol {
		return p.KnowledgeExtraction.Model, p.KnowledgeExtraction.EndpointOrigin, true
	}
	return "", "", false
}
