package platform

// Catalog revisions contain reviewed metadata and immutable project-knowledge
// pointers. The existing knowledge revision remains the only source body store.
type SkillCatalogOrigin struct {
	ProjectID        string `json:"project_id"`
	KnowledgeID      string `json:"knowledge_id"`
	KnowledgeVersion int    `json:"knowledge_version"`
	Revision         int    `json:"revision"`
	SHA256           string `json:"sha256"`
}
type SkillCatalogSummary struct {
	ID             string             `json:"id"`
	Version        int                `json:"version"`
	State          string             `json:"state"`
	Title          string             `json:"title"`
	Summary        string             `json:"summary"`
	Domain         string             `json:"domain"`
	Scenarios      []string           `json:"scenarios"`
	Tags           []string           `json:"tags"`
	Origin         SkillCatalogOrigin `json:"origin"`
	SourceCurrent  bool               `json:"source_current"`
	Eligibility    string             `json:"eligibility"`
	CreatedBy      string             `json:"created_by"`
	CreatedAt      string             `json:"created_at"`
	ReviewedBy     string             `json:"reviewed_by"`
	UpdatedAt      string             `json:"updated_at"`
	Reason         string             `json:"reason"`
	CanManage      bool               `json:"can_manage"`
	CanAdopt       bool               `json:"can_adopt"`
	AllowedActions []string           `json:"allowed_actions"`
}
type SkillCatalogDetail struct {
	SkillCatalogSummary
	Content string `json:"content"`
}
type SkillCatalogPage struct {
	Items      []SkillCatalogSummary `json:"items"`
	Total      int                   `json:"total"`
	NextCursor string                `json:"next_cursor"`
	CanManage  bool                  `json:"can_manage"`
	CanAdopt   bool                  `json:"can_adopt"`
}
type SkillCatalogListQuery struct {
	ProjectID string
	Domain    string
	Scenario  string
	Tag       string
	State     string
	Query     string
	Limit     int
	Cursor    string
}
type SkillCatalogWriteRequest struct {
	ProjectID                string   `json:"project_id"`
	KnowledgeID              string   `json:"knowledge_id"`
	ExpectedKnowledgeVersion int      `json:"expected_knowledge_version"`
	PublishedRevision        int      `json:"published_revision"`
	PublishedSHA256          string   `json:"published_sha256"`
	ExpectedVersion          int      `json:"expected_version"`
	Domain                   string   `json:"domain"`
	Scenarios                []string `json:"scenarios"`
	Tags                     []string `json:"tags"`
	Reason                   string   `json:"reason"`
	AcknowledgeSharing       bool     `json:"acknowledge_sharing"`
	IdempotencyKey           string   `json:"idempotency_key"`
}
type SkillCatalogCommandRequest struct {
	ProjectID       string `json:"project_id"`
	Action          string `json:"action"`
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type SkillCatalogAdoptRequest struct {
	ProjectID        string `json:"project_id"`
	ExpectedVersion  int    `json:"expected_version"`
	OriginSHA256     string `json:"origin_sha256"`
	Reason           string `json:"reason"`
	AcknowledgeDraft bool   `json:"acknowledge_draft"`
	IdempotencyKey   string `json:"idempotency_key"`
}
type SkillCatalogRevisionPage struct {
	Items      []SkillCatalogDetail `json:"items"`
	Total      int                  `json:"total"`
	NextCursor string               `json:"next_cursor"`
}
type SkillCatalogAdoption struct {
	CatalogID      string             `json:"catalog_id"`
	CatalogVersion int                `json:"catalog_version"`
	Origin         SkillCatalogOrigin `json:"origin"`
	ProjectID      string             `json:"project_id"`
	KnowledgeID    string             `json:"knowledge_id"`
	Revision       int                `json:"revision"`
	SHA256         string             `json:"sha256"`
	AdoptedBy      string             `json:"adopted_by"`
	AdoptedAt      string             `json:"adopted_at"`
	Reason         string             `json:"reason"`
}
