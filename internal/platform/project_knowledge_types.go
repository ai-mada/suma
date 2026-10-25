package platform

// ProjectKnowledgeReference identifies reviewed immutable text, not executable
// authority. A fresh dispatch also verifies its current publication and sources.
type ProjectKnowledgeReference struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	SHA256   string `json:"sha256"`
}

// Source inputs are always resolved against the authorized current project.
// An empty quote selects the complete visible source text; it is never truncated.
type ProjectKnowledgeSourceInput struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int    `json:"version"`
	Quote   string `json:"quote,omitempty"`
}

type ProjectKnowledgeSource struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	Quote   string `json:"quote"`
	SHA256  string `json:"sha256"`
}

// Consumption keeps source provenance without repeating each verbatim quote.
// Full review evidence remains in immutable ProjectKnowledgeRevision.Sources.
type ProjectKnowledgeContextSource struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	SHA256  string `json:"sha256"`
}

type ProjectKnowledgeContextItem struct {
	ID       string                          `json:"id"`
	Revision int                             `json:"revision"`
	Kind     string                          `json:"kind"`
	Title    string                          `json:"title"`
	Summary  string                          `json:"summary"`
	Content  string                          `json:"content"`
	SHA256   string                          `json:"sha256"`
	Sources  []ProjectKnowledgeContextSource `json:"sources"`
}

type ProjectKnowledgeRevision struct {
	Revision        int                      `json:"revision"`
	BasedOnRevision int                      `json:"based_on_revision"`
	State           string                   `json:"state"`
	Kind            string                   `json:"kind"`
	Title           string                   `json:"title"`
	Summary         string                   `json:"summary"`
	Content         string                   `json:"content"`
	SHA256          string                   `json:"sha256"`
	Sources         []ProjectKnowledgeSource `json:"sources"`
	Method          string                   `json:"method"`
	CreatedBy       string                   `json:"created_by"`
	CreatedAt       string                   `json:"created_at"`
}

type ProjectKnowledgeSummary struct {
	ID                string `json:"id"`
	ProjectID         string `json:"project_id"`
	Version           int    `json:"version"`
	Kind              string `json:"kind"`
	State             string `json:"state"`
	Title             string `json:"title"`
	Summary           string `json:"summary"`
	LatestRevision    int    `json:"latest_revision"`
	PublishedRevision int    `json:"published_revision"`
	PublishedSHA256   string `json:"published_sha256"`
	PublishedTitle    string `json:"published_title"`
	PublishedSummary  string `json:"published_summary"`
	HasDraft          bool   `json:"has_draft"`
	PublishedEligible bool   `json:"published_eligible"`
	Eligibility       string `json:"eligibility"` // none/current/source_changed/retired; historical receipt may be publication_changed
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type ProjectKnowledgeDetail struct {
	ProjectKnowledgeSummary
	Latest    ProjectKnowledgeRevision  `json:"latest"`
	Published *ProjectKnowledgeRevision `json:"published,omitempty"`
}

type ProjectKnowledgeList struct {
	Items      []ProjectKnowledgeSummary `json:"items"`
	Total      int                       `json:"total"`
	NextCursor string                    `json:"next_cursor"`
}

type ProjectKnowledgeListQuery struct {
	Kind   string
	State  string
	Query  string
	Limit  int
	Cursor string
}

// Full draft replacement makes the reviewed content and source set explicit.
// Updating a published asset preserves its older approved revision until publish.
type ProjectKnowledgeWriteRequest struct {
	ProjectID       string                        `json:"project_id"`
	Kind            string                        `json:"kind"`
	Title           string                        `json:"title"`
	Summary         string                        `json:"summary"`
	Content         string                        `json:"content"`
	Sources         []ProjectKnowledgeSourceInput `json:"sources"`
	ExpectedVersion int                           `json:"expected_version"`
	IdempotencyKey  string                        `json:"idempotency_key"`
}

type ProjectKnowledgeExtractionRequest struct {
	ProjectID      string                        `json:"project_id"`
	Kind           string                        `json:"kind"`
	Title          string                        `json:"title,omitempty"`
	Summary        string                        `json:"summary,omitempty"`
	Sources        []ProjectKnowledgeSourceInput `json:"sources"`
	IdempotencyKey string                        `json:"idempotency_key"`
}

type ProjectKnowledgeCommandRequest struct {
	ProjectID       string `json:"project_id"`
	Action          string `json:"action"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// History is current-authorized project data. It is not exposed by the Agent
// published-only tool and never changes an existing revision's snapshot.
type ProjectKnowledgeRevisionList struct {
	Items      []ProjectKnowledgeRevision `json:"items"`
	Total      int                        `json:"total"`
	NextCursor string                     `json:"next_cursor"`
}
