package platform

// TaskDesignContextView qualifies current eligibility without changing the
// historical approved snapshot. It confers no command or approval authority.
type TaskDesignContextView struct {
	ProjectID                 string                    `json:"project_id"`
	TaskID                    string                    `json:"task_id"`
	TaskVersion               int                       `json:"task_version"`
	Bound                     bool                      `json:"bound"`
	Eligibility               string                    `json:"eligibility"`
	Context                   *RequirementDesignContext `json:"context,omitempty"`
	Requirement               *TaskContextRecord        `json:"requirement,omitempty"`
	AdoptedTask               *RequirementDesignTask    `json:"adopted_task,omitempty"`
	AdoptedBy                 string                    `json:"adopted_by,omitempty"`
	AdoptedAt                 string                    `json:"adopted_at,omitempty"`
	CurrentRequirementVersion int                       `json:"current_requirement_version"`
	CurrentDesignVersion      int                       `json:"current_design_version"`
	CurrentPublishedRevision  int                       `json:"current_published_revision"`
}
