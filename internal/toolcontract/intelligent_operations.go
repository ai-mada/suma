package toolcontract

import "fmt"

// IntelligentOperationsCapability is a fixed, probed, text-only model purpose.
// It proposes a closed local-draft plan; it never grants tools or write access.
type IntelligentOperationsCapability struct {
	Protocol       string `json:"protocol"`
	Model          string `json:"model"`
	EndpointOrigin string `json:"endpoint_origin"`
}

const IntelligentOperationsProtocol = "suma-intelligent-operations-v1"

func ValidateIntelligentOperationsCapability(p IntelligentOperationsCapability) error {
	if p.Protocol != IntelligentOperationsProtocol || !validFixedModel(p.Model, p.EndpointOrigin) {
		return fmt.Errorf("invalid fixed intelligent operations model or endpoint origin")
	}
	return nil
}
