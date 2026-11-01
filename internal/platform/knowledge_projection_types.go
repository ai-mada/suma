package platform

import ic "github.com/ai-mada/suma/internal/indexcontract"

type ProjectKnowledgeIndexQuery struct {
	Query string `json:"q,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Limit int    `json:"limit,omitempty"`
}
type ProjectKnowledgeIndexHit struct {
	Reference   ProjectKnowledgeReference `json:"reference"`
	ProjectID   string                    `json:"project_id"`
	Kind        string                    `json:"kind"`
	Title       string                    `json:"title"`
	Summary     string                    `json:"summary"`
	Snippet     string                    `json:"snippet"`
	SourceCount int                       `json:"source_count"`
	PublishedAt string                    `json:"published_at"`
	Source      ic.Source                 `json:"source"`
	Locator     ic.Locator                `json:"locator"`
}
type ProjectKnowledgeIndexSearch struct {
	Items              []ProjectKnowledgeIndexHit `json:"items"`
	Mode               string                     `json:"mode"`
	Generation         string                     `json:"generation"`
	IndexWatermark     int64                      `json:"index_watermark"`
	CandidatesExamined int                        `json:"candidates_examined"`
	Discarded          int                        `json:"discarded"`
	Scope              string                     `json:"scope"`
}

// This summary intentionally excludes immutable Request title/content. A
// delivery receipt is not a proof of continuous provider/profile coverage.
type ProjectKnowledgeProjectionEvent struct {
	EventID   string    `json:"event_id"`
	Source    ic.Source `json:"source"`
	Action    string    `json:"action"`
	State     string    `json:"state"`
	CreatedAt string    `json:"created_at"`
}
type ProjectKnowledgeIndexStatus struct {
	Domain                string                           `json:"domain"`
	AsOf                  string                           `json:"as_of"`
	Configured            bool                             `json:"configured"`
	Coverage              string                           `json:"coverage"`
	Pending               int                              `json:"pending"`
	Indexed               int                              `json:"indexed"`
	Failed                int                              `json:"failed"`
	Total                 int                              `json:"total"`
	CurrentPublications   int                              `json:"current_publications"`
	UntrackedPublications int                              `json:"untracked_publications"`
	LatestEvent           *ProjectKnowledgeProjectionEvent `json:"latest_event"`
	LastAcknowledgement   *ic.Receipt                      `json:"last_ack"`
}
