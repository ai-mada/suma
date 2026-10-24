package platform

type AutomationIngressSpec struct {
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	Events       []string `json:"events"`
	Repository   string   `json:"repository"`
	RepositoryID string   `json:"repository_id"`
	Ref          string   `json:"ref"`
	Conclusions  []string `json:"conclusions"`
}
type AutomationIngress struct {
	AutomationIngressSpec
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	RuleID         string `json:"rule_id"`
	RuleRevision   int    `json:"rule_revision"`
	Version        int    `json:"version"`
	Revision       int    `json:"revision"`
	State          string `json:"state"`
	ActivatedBy    string `json:"activated_by"`
	KeyConfigured  bool   `json:"key_configured"`
	KeyGeneration  int    `json:"key_generation"`
	KeyFingerprint string `json:"key_fingerprint"`
	EndpointPath   string `json:"endpoint_path"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	LastError      string `json:"last_error"`
}
type AutomationIngressCreateRequest struct {
	ProjectID            string                `json:"project_id"`
	ExpectedRuleVersion  int                   `json:"expected_rule_version"`
	ExpectedRuleRevision int                   `json:"expected_rule_revision"`
	IdempotencyKey       string                `json:"idempotency_key"`
	Spec                 AutomationIngressSpec `json:"spec"`
}
type AutomationIngressCommandRequest struct {
	ProjectID            string                 `json:"project_id"`
	Action               string                 `json:"action"`
	ExpectedVersion      int                    `json:"expected_version"`
	IdempotencyKey       string                 `json:"idempotency_key"`
	ExpectedRuleVersion  int                    `json:"expected_rule_version,omitempty"`
	ExpectedRuleRevision int                    `json:"expected_rule_revision,omitempty"`
	Spec                 *AutomationIngressSpec `json:"spec,omitempty"`
	Secret               string                 `json:"secret,omitempty"`
}
type AutomationIngressRevision struct {
	Revision     int                   `json:"revision"`
	RuleRevision int                   `json:"rule_revision"`
	Spec         AutomationIngressSpec `json:"spec"`
	CreatedAt    string                `json:"created_at"`
}
type AutomationIngressList struct {
	Items      []AutomationIngress `json:"items"`
	Total      int                 `json:"total"`
	NextCursor string              `json:"next_cursor"`
}
type AutomationIngressRevisions struct {
	Items      []AutomationIngressRevision `json:"items"`
	Total      int                         `json:"total"`
	NextCursor string                      `json:"next_cursor"`
}
type AutomationIngressEvent struct {
	SchemaVersion int    `json:"schema_version"`
	Event         string `json:"event"`
	Repository    string `json:"repository"`
	RepositoryID  string `json:"repository_id"`
	Ref           string `json:"ref"`
	Commit        string `json:"commit"`
	Conclusion    string `json:"conclusion"`
	ExternalID    string `json:"external_id"`
}
type AutomationIngressDelivery struct {
	ID              string                 `json:"id"`
	IngressID       string                 `json:"ingress_id"`
	IngressRevision int                    `json:"ingress_revision"`
	RuleID          string                 `json:"rule_id"`
	RuleRevision    int                    `json:"rule_revision"`
	DeliveryID      string                 `json:"delivery_id"`
	Provider        string                 `json:"provider"`
	BodySHA256      string                 `json:"body_sha256"`
	KeyGeneration   int                    `json:"key_generation"`
	ReceivedAt      string                 `json:"received_at"`
	LastReceivedAt  string                 `json:"last_received_at"`
	DuplicateCount  int                    `json:"duplicate_count"`
	State           string                 `json:"state"`
	Reason          string                 `json:"reason"`
	OccurrenceID    string                 `json:"occurrence_id"`
	OccurrenceState string                 `json:"occurrence_state,omitempty"`
	RunID           string                 `json:"run_id,omitempty"`
	RunState        string                 `json:"run_state,omitempty"`
	Event           AutomationIngressEvent `json:"event"`
}
type AutomationIngressDeliveries struct {
	Items      []AutomationIngressDelivery `json:"items"`
	Total      int                         `json:"total"`
	NextCursor string                      `json:"next_cursor"`
}
type AutomationIngressReceipt struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	Duplicate    bool   `json:"duplicate"`
	OccurrenceID string `json:"occurrence_id"`
}
