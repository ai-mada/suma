package platform

// RequirementTraceability projects current requirement criteria through an
// approved design and protected adoption to exact execution and review facts.
// It does not declare test-case coverage or grant mutation authority.
type RequirementTraceability struct {
	ProjectID          string                             `json:"project_id"`
	RequirementID      string                             `json:"requirement_id"`
	RequirementVersion int                                `json:"requirement_version"`
	RequirementStatus  string                             `json:"requirement_status"`
	DesignVersion      int                                `json:"design_version"`
	PublishedRevision  int                                `json:"published_revision"`
	DesignSHA256       string                             `json:"design_sha256"`
	Eligibility        string                             `json:"eligibility"`
	Criteria           []RequirementTraceabilityCriterion `json:"criteria"`
	Tasks              []RequirementTraceabilityTask      `json:"tasks"`
}

// Index is 1-based within this response's requirement ID and version. It is not
// a criterion identity across requirement revisions.
type RequirementTraceabilityCriterion struct {
	Index    int      `json:"index"`
	Text     string   `json:"text"`
	TaskKeys []string `json:"task_keys"`
}

type RequirementTraceabilityTask struct {
	Key              string           `json:"key"`
	Title            string           `json:"title"`
	Covers           []int            `json:"covers"`
	TaskID           string           `json:"task_id"`
	TaskVersion      int              `json:"task_version"`
	TaskStatus       string           `json:"task_status"`
	BindingState     string           `json:"binding_state"`
	RunBasisState    string           `json:"run_basis_state"`
	FormalAcceptance bool             `json:"formal_acceptance"`
	Run              *TaskDeliveryRun `json:"run"`
}
