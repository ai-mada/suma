package platform

// TaskConversationMention is the immutable identity and display name resolved
// by the server when a human/API message was posted.
type TaskConversationMention struct {
	ResourceID string `json:"resource_id"`
	UserID     string `json:"user_id"`
	NameAtPost string `json:"name_at_post"`
}

type TaskConversationMentionTarget struct {
	ResourceID      string `json:"resource_id"`
	UserID          string `json:"user_id"`
	Name            string `json:"name"`
	ResourceVersion int    `json:"resource_version"`
}

type TaskConversationMentionTargetQuery struct {
	Q           string   `json:"q,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Cursor      string   `json:"cursor,omitempty"`
	ResourceIDs []string `json:"resource_ids,omitempty"`
}

type TaskConversationMentionTargetPage struct {
	ProjectID  string                          `json:"project_id"`
	TaskID     string                          `json:"task_id"`
	Limit      int                             `json:"limit"`
	Items      []TaskConversationMentionTarget `json:"items"`
	Total      int                             `json:"total"`
	NextCursor string                          `json:"next_cursor"`
}

type TaskConversationMessageDetail struct {
	ProjectID   string                  `json:"project_id"`
	TaskID      string                  `json:"task_id"`
	TaskVersion int                     `json:"task_version"`
	CanPost     bool                    `json:"can_post"`
	Message     TaskConversationMessage `json:"message"`
}
