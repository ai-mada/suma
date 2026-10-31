package platform

// DeliveryKnowledgeTestEvidence names a persisted test result, never raw logs,
// JUnit XML or model claims. Optional report fields exist only for real reports.
type DeliveryKnowledgeTestEvidence struct {
	RunID        string `json:"run_id"`
	NodeID       string `json:"node_id"`
	Attempt      int    `json:"attempt"`
	Fence        int64  `json:"fence"`
	WorkerID     string `json:"worker_id"`
	ResultSHA256 string `json:"result_sha256"`
	PlanID       string `json:"plan_id,omitempty"`
	PlanRevision int    `json:"plan_revision,omitempty"`
	PlanSHA256   string `json:"plan_sha256,omitempty"`
	ReportID     string `json:"report_id,omitempty"`
	ReportSHA256 string `json:"report_sha256,omitempty"`
	SourceCommit string `json:"source_commit,omitempty"`
}

// Candidate facts are optional: a formally accepted test-only delivery is not
// falsely presented as a code change. No paths, patches or credentials escape.
type DeliveryKnowledgeCandidateEvidence struct {
	Kind           string `json:"kind"` // workspace_revision or verified sealed_bundle
	RunID          string `json:"run_id"`
	NodeID         string `json:"node_id"`
	Attempt        int    `json:"attempt"`
	Fence          int64  `json:"fence"`
	WorkerID       string `json:"worker_id"`
	HeadCommit     string `json:"head_commit"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	TransferID     string `json:"transfer_id,omitempty"`
	BundleSHA256   string `json:"bundle_sha256,omitempty"`
}

// This server-resolved basis is data provenance, not a new acceptance or tool
// grant. SHA256 hashes this canonical struct with SHA256 empty; evidence lists
// are sorted by their exact run/node/attempt/fence/worker identity first.
type DeliveryKnowledgeBasis struct {
	Protocol   string                               `json:"protocol"`
	Task       ReleaseTaskBasis                     `json:"task"`
	RunVersion int                                  `json:"run_version"`
	Tests      []DeliveryKnowledgeTestEvidence      `json:"tests"`
	Candidates []DeliveryKnowledgeCandidateEvidence `json:"candidates"`
	SHA256     string                               `json:"sha256"`
}

// Read-only preview under current project authority. It never starts a model,
// creates a proposal or publishes knowledge; the existing explicit flow does.
type DeliveryKnowledgeSourceView struct {
	ProjectID string                 `json:"project_id"`
	Source    ProjectKnowledgeSource `json:"source"`
}
