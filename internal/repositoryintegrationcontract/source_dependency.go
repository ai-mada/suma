package repositoryintegrationcontract

const SourceDependencyParserID = "suma-node-workspace"
const SourceDependencyParserVersion = "1.0.0"

// SourceDependencyReport is the sole shared parser output contract. It records
// npm/pnpm declarations, never an external lock, runtime or all-language graph.
type SourceDependencyReport struct {
	SchemaVersion   int                           `json:"schema_version"`
	ParserID        string                        `json:"parser_id"`
	ParserVersion   string                        `json:"parser_version"`
	SourceSnapshots []string                      `json:"source_snapshots"`
	Packages        []SourceDependencyPackage     `json:"packages"`
	Dependencies    []SourceDependencyDeclaration `json:"dependencies"`
	Files           []SourceDependencyFile        `json:"files"`
	Coverage        SourceDependencyCoverage      `json:"coverage"`
	Diagnostics     []SourceDependencyDiagnostic  `json:"diagnostics"`
}
type SourceDependencyPackage struct {
	ID                   string `json:"id"`
	SourceSnapshotSHA256 string `json:"source_snapshot_sha256"`
	Path                 string `json:"path"`
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Root                 string `json:"root"`
	WorkspaceMember      bool   `json:"workspace_member"`
}
type SourceDependencyDeclaration struct {
	ID                   string   `json:"id"`
	SourceSnapshotSHA256 string   `json:"source_snapshot_sha256"`
	FromPackageID        string   `json:"from_package_id"`
	Section              string   `json:"section"`
	Name                 string   `json:"name"`
	RawRange             string   `json:"raw_range"`
	RawRangeSHA256       string   `json:"raw_range_sha256"`
	RangeRedacted        bool     `json:"range_redacted"`
	Workspace            bool     `json:"workspace"`
	TargetPackageIDs     []string `json:"target_package_ids"`
	RangeState           string   `json:"range_state"`
	Compatibility        string   `json:"compatibility"`
	Path                 string   `json:"path"`
	Pointer              string   `json:"pointer"`
}
type SourceDependencyFile struct {
	SourceSnapshotSHA256 string `json:"source_snapshot_sha256"`
	Path                 string `json:"path"`
	BlobOID              string `json:"blob_oid"`
	SHA256               string `json:"sha256"`
	SizeBytes            int64  `json:"size_bytes"`
	Format               string `json:"format"`
}
type SourceDependencyCoverage struct {
	State                  string   `json:"state"`
	Files                  int      `json:"files"`
	ParsedFiles            int      `json:"parsed_files"`
	Packages               int      `json:"packages"`
	Dependencies           int      `json:"dependencies"`
	WorkspaceDeclarations  bool     `json:"workspace_declarations"`
	ExternalLockResolution bool     `json:"external_lock_resolution"`
	RuntimeDependencies    bool     `json:"runtime_dependencies"`
	OtherLanguages         bool     `json:"other_languages"`
	Reasons                []string `json:"reasons"`
}
type SourceDependencyDiagnostic struct {
	SourceSnapshotSHA256 string `json:"source_snapshot_sha256"`
	Path                 string `json:"path"`
	Pointer              string `json:"pointer"`
	Code                 string `json:"code"`
}
