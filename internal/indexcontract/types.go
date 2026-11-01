// Package indexcontract is the single current domain-aware internal indexing
// wire. Publication data is not a documents Record and never grants authority.
package indexcontract

type Source struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Sequence is the monotonic business-event version recorded by the outbox.
	// A later unpublished draft must not replace a publication's source tuple.
	Sequence int `json:"sequence"`
	// Revision identifies immutable content: a document version or the exact
	// published knowledge revision, independently of later aggregate edits.
	Revision int `json:"revision"`
	// SHA256 binds that immutable content/publication, not provider-generated text.
	SHA256 string `json:"sha256"`
}
type Locator struct {
	// Unit is unicode_codepoints over canonical content, not UTF-16 or bytes;
	// Start is zero-based and End is exclusive (Go []rune / JS Array.from).
	Unit  string `json:"unit"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}
type Request struct {
	Protocol  string `json:"protocol"`
	EventID   string `json:"event_id"`
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	Source    Source `json:"source"`
	Action    string `json:"action"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}
type Receipt struct {
	Protocol       string `json:"protocol"`
	EventID        string `json:"event_id"`
	TenantID       string `json:"tenant_id"`
	ProjectID      string `json:"project_id"`
	Source         Source `json:"source"`
	Action         string `json:"action"`
	Disposition    string `json:"disposition"`
	Generation     string `json:"generation"`
	IndexWatermark int64  `json:"index_watermark"`
	RequestSHA256  string `json:"request_sha256"`
}
type SearchRequest struct {
	Protocol   string `json:"protocol"`
	TenantID   string `json:"tenant_id"`
	ProjectID  string `json:"project_id"`
	SourceKind string `json:"source_kind"`
	Query      string `json:"query"`
	Limit      int    `json:"limit"`
}
type SearchHit struct {
	Source  Source  `json:"source"`
	Locator Locator `json:"locator"`
}
type SearchResult struct {
	Protocol       string      `json:"protocol"`
	Items          []SearchHit `json:"items"`
	Mode           string      `json:"mode"`
	Generation     string      `json:"generation"`
	IndexWatermark int64       `json:"index_watermark"`
}
