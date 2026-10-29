package platform

// MyWorkOptions selects current tasks assigned to the authenticated human.
// Q is literal text, including spaces; it is never an owner selector.
type MyWorkOptions struct {
	Q      string
	Status string
	Limit  int
	Cursor string
}

type MyWorkStatusCount struct {
	Status string `json:"status"`
	Total  int    `json:"total"`
}

// MyWorkPage is a current read, not an immutable snapshot or personal approval
// assignment. StatusCounts applies Q before the selected status filter.
type MyWorkPage struct {
	ProjectID      string              `json:"project_id"`
	UserID         string              `json:"user_id"`
	ResourceID     string              `json:"resource_id"`
	ResourceName   string              `json:"resource_name"`
	ResourceStatus string              `json:"resource_status"`
	Items          []Record            `json:"items"`
	Total          int                 `json:"total"`
	Limit          int                 `json:"limit"`
	NextCursor     string              `json:"next_cursor"`
	StatusCounts   []MyWorkStatusCount `json:"status_counts"`
	AsOf           string              `json:"as_of"`
}
