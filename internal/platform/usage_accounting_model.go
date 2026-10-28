package platform

import "github.com/ai-mada/suma/internal/toolcontract"

// Pricing identifies the immutable configured model, never the model named by
// an untrusted CLI result. Specialized read-only purposes share the same quota
// and accounting transaction; adding a purpose must not create a free lane.
func usagePricedModel(profile ToolProfile) (model, origin string, ok bool) {
	p := profile.Profile
	capabilities := 0
	if p.IntelligentOperations != nil {
		capabilities++
	}
	if p.Planning != nil {
		capabilities++
	}
	if p.KnowledgeExtraction != nil {
		capabilities++
	}
	if p.FailureDiagnosis != nil {
		capabilities++
	}
	if profile.State != "enabled" || p.Adapter != "suma" || !p.ReadOnly || p.WorkspaceToolsVersion != "" || capabilities != 1 {
		return "", "", false
	}
	if p.IntelligentOperations != nil && p.IntelligentOperations.Protocol == toolcontract.IntelligentOperationsProtocol {
		return p.IntelligentOperations.Model, p.IntelligentOperations.EndpointOrigin, true
	}
	if p.Planning != nil && p.Planning.Protocol == toolcontract.PlanningProtocol {
		return p.Planning.Model, p.Planning.EndpointOrigin, true
	}
	if p.KnowledgeExtraction != nil && p.KnowledgeExtraction.Protocol == toolcontract.KnowledgeExtractionProtocol {
		return p.KnowledgeExtraction.Model, p.KnowledgeExtraction.EndpointOrigin, true
	}
	if p.FailureDiagnosis != nil && p.FailureDiagnosis.Protocol == toolcontract.FailureDiagnosisProtocol {
		return p.FailureDiagnosis.Model, p.FailureDiagnosis.EndpointOrigin, true
	}
	return "", "", false
}
