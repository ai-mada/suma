package platform

// TaskConversationMessage is an immutable collaboration note, not an execution
// instruction. Its author and original task context are captured by the server.
type TaskConversationMessage struct {
	ID                 string `json:"id"`
	Sequence           int64  `json:"sequence"`
	TaskVersion        int    `json:"task_version"`
	Body               string `json:"body"`
	AuthorID           string `json:"author_id"`
	AuthorName         string `json:"author_name"`
	ActorKind          string `json:"actor_kind"`
	CreatedAt          string `json:"created_at"`
	RequirementID      string `json:"requirement_id"`
	RequirementVersion int    `json:"requirement_version"`
	RunID              string `json:"run_id"`
}

// TaskConversationPage returns the latest bounded window in chronological
// order. NextBefore is the exclusive oldest sequence for fetching older notes;
// Total counts the full authorized task conversation, not only this window.
type TaskConversationPage struct {
	ProjectID   string                    `json:"project_id"`
	TaskID      string                    `json:"task_id"`
	TaskVersion int                       `json:"task_version"`
	CanPost     bool                      `json:"can_post"`
	Items       []TaskConversationMessage `json:"items"`
	Total       int64                     `json:"total"`
	Limit       int                       `json:"limit"`
	NextBefore  int64                     `json:"next_before"`
}

type TaskConversationAppendRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	Body                string `json:"body"`
	IdempotencyKey      string `json:"idempotency_key"`
}
