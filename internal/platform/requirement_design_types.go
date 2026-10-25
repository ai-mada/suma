package platform

// RequirementDesignContent is reviewed design text, not executable authority.
// Product, experience and technical sections share one 16,000 UTF-8 byte budget.
type RequirementDesignContent struct {
	Product    string `json:"product"`
	Experience string `json:"experience"`
	Technical  string `json:"technical"`
}

// Keys remain stable across revisions. DependsOn refers only to other keys in
// this complete proposal; Covers uses 1-based criteria of its fixed requirement.
type RequirementDesignTask struct {
	Key                string   `json:"key"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority"`
	OwnerID            string   `json:"owner_id"`
	DependsOn          []string `json:"depends_on"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Covers             []int    `json:"covers"`
}

type RequirementDesignDiagnostic struct {
	Code           string `json:"code"`
	Field          string `json:"field"`
	TaskKey        string `json:"task_key,omitempty"`
	CriterionIndex int    `json:"criterion_index,omitempty"`
}

// Every saved draft and publication is immutable and includes the actual
// requirement snapshot read by the server, never a client-supplied source proof.
type RequirementDesignRevision struct {
	Revision    int                      `json:"revision"`
	State       string                   `json:"state"`
	Requirement TaskContextRecord        `json:"requirement"`
	Content     RequirementDesignContent `json:"content"`
	Tasks       []RequirementDesignTask  `json:"tasks"`
	SHA256      string                   `json:"sha256"`
	CreatedBy   string                   `json:"created_by"`
	CreatedAt   string                   `json:"created_at"`
}

type RequirementDesignView struct {
	ProjectID         string                        `json:"project_id"`
	RequirementID     string                        `json:"requirement_id"`
	Version           int                           `json:"version"`
	Requirement       TaskContextRecord             `json:"requirement"`
	LatestRevision    int                           `json:"latest_revision"`
	PublishedRevision int                           `json:"published_revision"`
	HasDraft          bool                          `json:"has_draft"`
	Eligibility       string                        `json:"eligibility"`
	Latest            *RequirementDesignRevision    `json:"latest,omitempty"`
	Published         *RequirementDesignRevision    `json:"published,omitempty"`
	Validation        []RequirementDesignDiagnostic `json:"validation"`
	LastAdoption      *RequirementDesignAdoption    `json:"last_adoption,omitempty"`
}

type RequirementDesignSaveRequest struct {
	ProjectID                  string                   `json:"project_id"`
	ExpectedVersion            int                      `json:"expected_version"`
	ExpectedRequirementVersion int                      `json:"expected_requirement_version"`
	Content                    RequirementDesignContent `json:"content"`
	Tasks                      []RequirementDesignTask  `json:"tasks"`
	IdempotencyKey             string                   `json:"idempotency_key"`
}

type RequirementDesignCommandRequest struct {
	ProjectID                  string `json:"project_id"`
	Action                     string `json:"action"`
	ExpectedVersion            int    `json:"expected_version"`
	ExpectedRequirementVersion int    `json:"expected_requirement_version"`
	IdempotencyKey             string `json:"idempotency_key"`
}

// Preview is a read, not a reservation. PlanSHA256 binds the full current
// requirement/design/task scope and must be recomputed inside the adoption tx.
type RequirementDesignPreviewRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Revision        int    `json:"revision"`
	SHA256          string `json:"sha256"`
}

type RequirementDesignChange struct {
	Key                 string                 `json:"key"`
	Action              string                 `json:"action"`
	TaskID              string                 `json:"task_id"`
	ExpectedTaskVersion int                    `json:"expected_task_version"`
	Current             *Record                `json:"current,omitempty"`
	Proposed            *RequirementDesignTask `json:"proposed,omitempty"`
}

type RequirementDesignPreview struct {
	ProjectID          string                    `json:"project_id"`
	RequirementID      string                    `json:"requirement_id"`
	Version            int                       `json:"version"`
	Revision           int                       `json:"revision"`
	SHA256             string                    `json:"sha256"`
	RequirementVersion int                       `json:"requirement_version"`
	PlanSHA256         string                    `json:"plan_sha256"`
	Changes            []RequirementDesignChange `json:"changes"`
	CreateCount        int                       `json:"create_count"`
	UpdateCount        int                       `json:"update_count"`
	KeepCount          int                       `json:"keep_count"`
	ExcludeCount       int                       `json:"exclude_count"`
	HasChanges         bool                      `json:"has_changes"`
}

type RequirementDesignAdoptRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Revision        int    `json:"revision"`
	SHA256          string `json:"sha256"`
	PlanSHA256      string `json:"plan_sha256"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type RequirementDesignAdoptedTask struct {
	Key     string `json:"key"`
	TaskID  string `json:"task_id"`
	Version int    `json:"version"`
	Action  string `json:"action"`
}

type RequirementDesignAdoption struct {
	ID                 string                         `json:"id"`
	ProjectID          string                         `json:"project_id"`
	RequirementID      string                         `json:"requirement_id"`
	Version            int                            `json:"version"`
	Revision           int                            `json:"revision"`
	SHA256             string                         `json:"sha256"`
	RequirementVersion int                            `json:"requirement_version"`
	PlanSHA256         string                         `json:"plan_sha256"`
	Tasks              []RequirementDesignAdoptedTask `json:"tasks"`
	CreatedBy          string                         `json:"created_by"`
	CreatedAt          string                         `json:"created_at"`
}

type RequirementDesignRevisionList struct {
	Items      []RequirementDesignRevision `json:"items"`
	Total      int                         `json:"total"`
	NextCursor string                      `json:"next_cursor"`
}

type RequirementDesignAdoptionList struct {
	Items      []RequirementDesignAdoption `json:"items"`
	Total      int                         `json:"total"`
	NextCursor string                      `json:"next_cursor"`
}

// Only this compact reviewed design, not every task proposal, is passed once
// to a development node. Current bindings are stored outside editable Record.Data.
type RequirementDesignContext struct {
	RequirementID      string                   `json:"requirement_id"`
	RequirementVersion int                      `json:"requirement_version"`
	Revision           int                      `json:"revision"`
	SHA256             string                   `json:"sha256"`
	TaskKey            string                   `json:"task_key"`
	AdoptionID         string                   `json:"adoption_id"`
	Content            RequirementDesignContent `json:"content"`
}
