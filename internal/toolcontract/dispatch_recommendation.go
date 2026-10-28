package toolcontract

import (
	"fmt"

	"github.com/ai-mada/suma/internal/modelcontract"
)

// The purpose fixes its policy, not another provider identity. Its model,
// parameters and budget come exclusively from the host ModelExecution binding.
type DispatchRecommendationCapability struct {
	Protocol string `json:"protocol"`
}

const DispatchRecommendationProtocol = "suma-dispatch-recommendation-v1"

func ValidateDispatchRecommendationCapability(p DispatchRecommendationCapability) error {
	if p.Protocol != DispatchRecommendationProtocol {
		return fmt.Errorf("invalid dispatch recommendation protocol")
	}
	return nil
}

func SupportsDispatchRecommendationProfile(p Profile) bool {
	return p.Adapter == "suma" && p.Protocol == "suma-jsonl-v1" && p.ReadOnly && p.ModelExecutionProtocol == modelcontract.Protocol && p.WorkspaceToolsVersion == "" && p.TestReportProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil && p.DispatchRecommendation != nil && ValidateDispatchRecommendationCapability(*p.DispatchRecommendation) == nil
}
