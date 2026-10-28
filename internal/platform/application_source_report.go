package platform

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func applicationInspectorStreamSHA(reader io.Reader, max int64) (string, error) {
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(reader, max+1))
	if err != nil || n > max {
		return "", fmt.Errorf("source inspector executable exceeds fixed allowance")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type applicationSourceFileKey struct{ snapshot, path string }

// An output from a trusted parser still has to name the exact supplied files,
// package and JSON pointer. Parser success never permits a second source, an
// invented dependency or a silently expanded coverage claim.
func validateApplicationDependencyReport(sources []ri.ProviderSourceInspection, r ri.SourceDependencyReport) error {
	bad := func() error { return fmt.Errorf("invalid fixed source dependency report") }
	if r.SchemaVersion != 1 || r.ParserID != ri.SourceDependencyParserID || r.ParserVersion != ri.SourceDependencyParserVersion || r.SourceSnapshots == nil || r.Packages == nil || r.Dependencies == nil || r.Files == nil || r.Diagnostics == nil || len(r.Packages) > 200 || len(r.Dependencies) > 8192 || len(r.Diagnostics) > 512 {
		return bad()
	}
	files := map[applicationSourceFileKey]ri.RepositorySourceManifest{}
	snapshots := []string{}
	for _, source := range sources {
		if !contentDigest.MatchString(source.SnapshotSHA256) {
			return bad()
		}
		snapshots = append(snapshots, source.SnapshotSHA256)
		for _, file := range source.Manifests {
			key := applicationSourceFileKey{source.SnapshotSHA256, file.Path}
			if _, exists := files[key]; exists {
				return bad()
			}
			files[key] = file
		}
	}
	sort.Strings(snapshots)
	if stringify(snapshots) != stringify(r.SourceSnapshots) || len(files) != len(r.Files) || r.Coverage.Files != len(files) || r.Coverage.ParsedFiles < 0 || r.Coverage.ParsedFiles > len(files) || r.Coverage.Packages != len(r.Packages) || r.Coverage.Dependencies != len(r.Dependencies) || r.Coverage.ExternalLockResolution || r.Coverage.RuntimeDependencies || r.Coverage.OtherLanguages || r.Coverage.State != "complete" && r.Coverage.State != "partial" || len(r.Coverage.Reasons) > 64 {
		return bad()
	}
	seenFiles := map[applicationSourceFileKey]bool{}
	for _, file := range r.Files {
		key := applicationSourceFileKey{file.SourceSnapshotSHA256, file.Path}
		original, ok := files[key]
		if !ok || seenFiles[key] || file.BlobOID != original.BlobOID || file.SHA256 != original.SHA256 || file.SizeBytes != original.SizeBytes || file.Format != original.Format {
			return bad()
		}
		seenFiles[key] = true
	}
	packages := map[string]ri.SourceDependencyPackage{}
	for _, pkg := range r.Packages {
		file, ok := files[applicationSourceFileKey{pkg.SourceSnapshotSHA256, pkg.Path}]
		root := path.Dir(pkg.Path)
		if root == "." {
			root = ""
		}
		if !ok || file.Format != "package-json" || !contentDigest.MatchString(pkg.ID) || pkg.Root != root || !applicationReportText(pkg.Name, 214, false) || !applicationReportText(pkg.Version, 256, true) || packages[pkg.ID].ID != "" {
			return bad()
		}
		packages[pkg.ID] = pkg
	}
	seenDependencies := map[string]bool{}
	for _, dep := range r.Dependencies {
		pkg, ok := packages[dep.FromPackageID]
		if !ok || seenDependencies[dep.ID] || !contentDigest.MatchString(dep.ID) || dep.SourceSnapshotSHA256 != pkg.SourceSnapshotSHA256 || dep.Path != pkg.Path || !applicationReportText(dep.Name, 214, false) || !applicationReportText(dep.RawRange, 4096, true) || !contentDigest.MatchString(dep.RawRangeSHA256) || len(dep.TargetPackageIDs) > 200 {
			return bad()
		}
		seenDependencies[dep.ID] = true
		section := false
		for _, name := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
			if dep.Section == name {
				section = true
			}
		}
		pointer := "/" + dep.Section + "/" + strings.ReplaceAll(strings.ReplaceAll(dep.Name, "~", "~0"), "/", "~1")
		if !section || dep.Pointer != pointer {
			return bad()
		}
		file := files[applicationSourceFileKey{dep.SourceSnapshotSHA256, dep.Path}]
		raw, err := base64.StdEncoding.Strict().DecodeString(file.ContentBase64)
		if err != nil || RejectDuplicateReleaseJSON(raw) != nil {
			return bad()
		}
		var manifest map[string]json.RawMessage
		if json.Unmarshal(raw, &manifest) != nil {
			return bad()
		}
		var declarations map[string]string
		if json.Unmarshal(manifest[dep.Section], &declarations) != nil {
			return bad()
		}
		original, exists := declarations[dep.Name]
		if !exists || digest(original) != dep.RawRangeSHA256 || !dep.RangeRedacted && dep.RawRange != original || dep.RangeRedacted && dep.RawRange != "" {
			return bad()
		}
		for _, target := range dep.TargetPackageIDs {
			if packages[target].ID == "" {
				return bad()
			}
		}
		if dep.RangeState != "valid" && dep.RangeState != "invalid" && dep.RangeState != "unsupported" && dep.RangeState != "unknown" {
			return bad()
		}
		if dep.Compatibility != "compatible" && dep.Compatibility != "incompatible" && dep.Compatibility != "unknown" {
			return bad()
		}
	}
	for _, diagnostic := range r.Diagnostics {
		if _, ok := files[applicationSourceFileKey{diagnostic.SourceSnapshotSHA256, diagnostic.Path}]; !ok || !applicationReportText(diagnostic.Code, 100, false) || !applicationReportText(diagnostic.Pointer, 2048, true) {
			return bad()
		}
	}
	return nil
}

func applicationReportText(value string, max int, empty bool) bool {
	if !utf8.ValidString(value) || len(value) > max || !empty && strings.TrimSpace(value) == "" {
		return false
	}
	for _, c := range value {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

// Map immutable report facts back to each explicitly inspected code scope.
// Unsupported or unparsed manifests remain visible metadata with an explicit
// gap. This never turns lockfile discovery into external version resolution.
func (s *Service) parseApplicationSources(ctx context.Context, sources []ri.ProviderSourceInspection) ([]ApplicationSourceInspection, error) {
	report, err := s.runApplicationSourceInspector(ctx, sources)
	if err != nil {
		return nil, err
	}
	packages := map[applicationSourceFileKey]ri.SourceDependencyPackage{}
	dependencies := map[applicationSourceFileKey][]ri.SourceDependencyDeclaration{}
	diagnostics := map[applicationSourceFileKey][]string{}
	for _, pkg := range report.Packages {
		packages[applicationSourceFileKey{pkg.SourceSnapshotSHA256, pkg.Path}] = pkg
	}
	for _, dep := range report.Dependencies {
		key := applicationSourceFileKey{dep.SourceSnapshotSHA256, dep.Path}
		dependencies[key] = append(dependencies[key], dep)
	}
	for _, diagnostic := range report.Diagnostics {
		key := applicationSourceFileKey{diagnostic.SourceSnapshotSHA256, diagnostic.Path}
		diagnostics[key] = append(diagnostics[key], diagnostic.Code)
	}
	out := []ApplicationSourceInspection{}
	for _, source := range sources {
		for _, scope := range source.Scopes {
			inspection := ApplicationSourceInspection{SnapshotSHA256: source.SnapshotSHA256, Commit: source.ExactCommit, TreeOID: source.TreeOID, RelativeRoot: scope.RelativeRoot, ScopeTreeOID: scope.TreeOID, ParserSHA256: s.ApplicationSourceInspector.parserSHA, Manifests: []ApplicationManifestObservation{}, Coverage: ApplicationSourceCoverage{Complete: source.Coverage.State == "complete", MaxManifests: ri.RepositorySourceMaxManifests}}
			for _, file := range source.Manifests {
				if scope.RelativeRoot != "" && !strings.HasPrefix(file.Path, scope.RelativeRoot+"/") {
					continue
				}
				key := applicationSourceFileKey{source.SnapshotSHA256, file.Path}
				pkg, parsed := packages[key]
				if file.Format == "pnpm-workspace-yaml" && report.Coverage.WorkspaceDeclarations && len(diagnostics[key]) == 0 {
					parsed = true
				}
				manifest := ApplicationManifestObservation{Path: file.Path, BlobOID: file.BlobOID, SHA256: file.SHA256, Parser: report.ParserID, ParserVersion: report.ParserVersion, Ecosystem: "npm", PackageName: pkg.Name, DeclaredVersion: pkg.Version, Dependencies: []ApplicationManifestDependency{}, Complete: parsed && len(diagnostics[key]) == 0}
				for _, dep := range dependencies[key] {
					manifest.Dependencies = append(manifest.Dependencies, ApplicationManifestDependency{Name: dep.Name, DeclaredRange: dep.RawRange, DeclaredRangeSHA256: dep.RawRangeSHA256, RangeRedacted: dep.RangeRedacted, Kind: dep.Section, RangeState: dep.RangeState, Compatibility: dep.Compatibility})
				}
				if !manifest.Complete {
					manifest.Reason = "unsupported_or_invalid_manifest"
					inspection.Coverage.Complete = false
				}
				inspection.Manifests = append(inspection.Manifests, manifest)
			}
			inspection.Coverage.ManifestCount = len(inspection.Manifests)
			inspection.Coverage.ManifestTotal = len(inspection.Manifests)
			if len(inspection.Manifests) == 0 {
				inspection.Coverage.Complete = false
				inspection.Coverage.Reason = "no_supported_manifests"
			} else if !inspection.Coverage.Complete {
				inspection.Coverage.Reason = "declarations_only_with_gaps"
			} else {
				inspection.Coverage.Reason = "npm_manifest_declarations_only"
			}
			out = append(out, inspection)
		}
	}
	return out, nil
}
