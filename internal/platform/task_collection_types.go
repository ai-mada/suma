package platform

type TaskCollectionGroup struct {
	Key   string `json:"key"`
	Total int    `json:"total"`
}

type TaskCollectionGroupOptions struct {
	GroupBy string `json:"group_by,omitempty"`
	Limit   int    `json:"group_limit,omitempty"`
	Cursor  string `json:"group_cursor,omitempty"`
}

// Complete counts and bounded group metadata for the same filtered scope as
// /task-workspace. Empty keys are genuine source-unset data, never wildcards.
type TaskCollectionSummary struct {
	ProjectID       string                `json:"project_id"`
	AsOf            string                `json:"as_of"`
	Total           int                   `json:"total"`
	GroupBy         string                `json:"group_by"`
	Groups          []TaskCollectionGroup `json:"groups"`
	GroupsTotal     int                   `json:"groups_total"`
	GroupsLimit     int                   `json:"groups_limit"`
	NextGroupCursor string                `json:"next_group_cursor"`
}
