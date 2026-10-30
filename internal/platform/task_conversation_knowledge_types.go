package platform

// TaskConversationKnowledgeReference is immutable publication metadata. Only the
// server can capture the title/kind; callers submit ProjectKnowledgeReference.
type TaskConversationKnowledgeReference struct {
	ID          string `json:"id"`
	Revision    int    `json:"revision"`
	SHA256      string `json:"sha256"`
	TitleAtPost string `json:"title_at_post"`
	KindAtPost  string `json:"kind_at_post"`
}
