package platform

// WorkspaceMetadata intentionally contains no host path or recursive inventory.
// A configured binding grants neither a live execution connection nor write
// authority. The directory reader applies its own current read checks.
type WorkspaceMetadata struct {
	Configured       bool   `json:"configured"`
	BindingAvailable bool   `json:"binding_available"`
	Name             string `json:"name,omitempty"`
	Reason           string `json:"reason,omitempty"`
}
