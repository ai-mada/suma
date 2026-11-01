package platform

// Discovery reads only currently eligible published revisions. Its management
// counterpart deliberately keeps the existing latest/draft metadata semantics.
type ProjectKnowledgeDiscoveryQuery struct {
	Query  string `json:"q,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type ProjectKnowledgeDiscoveryHit struct {
	Reference    ProjectKnowledgeReference `json:"reference"`
	ProjectID    string                    `json:"project_id"`
	Kind         string                    `json:"kind"`
	Title        string                    `json:"title"`
	Summary      string                    `json:"summary"`
	Snippet      string                    `json:"snippet"`
	MatchedField string                    `json:"matched_field"`
	SourceCount  int                       `json:"source_count"`
	PublishedAt  string                    `json:"published_at"`
}
type ProjectKnowledgeDiscovery struct {
	Items      []ProjectKnowledgeDiscoveryHit `json:"items"`
	NextCursor string                         `json:"next_cursor"`
	Mode       string                         `json:"mode"`
	Scope      string                         `json:"scope"`
	Scanned    int                            `json:"scanned"`
	ScanLimit  int                            `json:"scan_limit"`
	Exhausted  bool                           `json:"exhausted"`
}
