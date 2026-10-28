package platform

type WorkspaceTreeEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// A page is an observation of one directory, not a recursive inventory or a
// count of all workspace files. An empty next cursor is the end of this view.
type WorkspaceTreePage struct {
	ProjectID      string               `json:"project_id"`
	Directory      string               `json:"directory"`
	Entries        []WorkspaceTreeEntry `json:"entries"`
	Limit          int                  `json:"limit"`
	NextCursor     string               `json:"next_cursor"`
	SnapshotSHA256 string               `json:"snapshot_sha256"`
	ObservedAt     string               `json:"observed_at"`
}
