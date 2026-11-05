package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func artifactProfile(alias string, spec CommandSpec) (releasecontract.CommandProfile, string, error) {
	kind := "build"
	if spec.ArtifactInput {
		kind = "package_test"
	}
	p := releasecontract.CommandProfile{ID: alias, Kind: kind, Executable: spec.Executable, Args: spec.Args, Env: spec.Env, EnvFrom: spec.EnvFrom, Input: spec.Input, ReadOnly: spec.ReadOnly, TestEvidence: spec.TestEvidence, ArtifactOutput: spec.ArtifactOutput, ArtifactInput: spec.ArtifactInput}
	hash, err := releasecontract.ProfileDigest(p)
	return p, hash, err
}

func assignmentRef(a Assignment) releasecontract.AssignmentRef {
	return releasecontract.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
}
func artifactOperation(a Assignment, phase string) string {
	return digest([]byte("artifact-v1/" + key(a) + "/" + a.WorkerID + "/" + phase))
}

func (c *Client) artifactRequest(ctx context.Context, method, path string, a Assignment, body io.Reader, length int64, operation string) (*http.Response, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return nil, fmt.Errorf("invalid artifact control origin")
	}
	if strings.TrimSpace(c.Token) == "" {
		return nil, fmt.Errorf("artifact worker credential required")
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+"/api/v1/runner/"+path, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = length
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Suma-Operation-ID", operation)
	identity, _ := json.Marshal(assignmentRef(a))
	req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString(identity))
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artifact request unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("artifact request rejected: HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func resolveArtifact(ctx context.Context, a Assignment, cfg ExecConfig) (releasecontract.ArtifactBinding, error) {
	var binding releasecontract.ArtifactBinding
	source, ok := a.Config["artifact_from"].(string)
	if !ok || source == "" || source == a.NodeID {
		return binding, fmt.Errorf("test config.artifact_from requires an explicit build ancestor")
	}
	err := cfg.Artifacts.post(ctx, "artifacts/resolve", releasecontract.ArtifactResolveRequest{ProjectID: cfg.Artifacts.ProjectID, Assignment: assignmentRef(a), OperationID: artifactOperation(a, "resolve"), SourceNodeID: source}, &binding)
	if err != nil {
		return binding, err
	}
	m, hash, err := releasecontract.CanonicalManifest(binding.Manifest)
	if err != nil || hash != binding.ManifestSHA256 || !releasecontract.ValidCommit(binding.SourceCommit) || binding.ArtifactID == "" || len(binding.ArtifactID) > 100 || strings.ContainsAny(binding.ArtifactID, "/\\") || binding.BuildRunID != a.RunID || binding.BuildNodeID != source || binding.BuildAttempt < 1 || binding.BuildFence < 1 {
		return binding, fmt.Errorf("invalid frozen artifact input binding")
	}
	binding.Manifest = m
	return binding, nil
}

func materializeArtifact(ctx context.Context, c *Client, a Assignment, binding releasecontract.ArtifactBinding, dir string) error {
	if err := safeMkdir(dir); err != nil {
		return err
	}
	for _, file := range binding.Manifest.Files {
		path := filepath.Join(dir, filepath.FromSlash(file.Path))
		if !within(dir, path) {
			return fmt.Errorf("artifact input escaped dedicated directory")
		}
		if err := safeMkdir(filepath.Dir(path)); err != nil {
			return err
		}
		resp, err := c.artifactRequest(ctx, http.MethodGet, "artifacts/"+url.PathEscape(binding.ArtifactID)+"/blobs/"+file.SHA256+"?project_id="+url.QueryEscape(c.ProjectID), a, nil, 0, artifactOperation(a, "download/"+file.SHA256))
		if err != nil {
			return err
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, file.SizeBytes+1))
		_ = resp.Body.Close()
		if readErr != nil || int64(len(b)) != file.SizeBytes || digest(b) != file.SHA256 {
			return fmt.Errorf("artifact input bytes do not match sealed manifest")
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("artifact input destination exists or is unavailable")
		}
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	_, hash, err := collectArtifact(dir, "")
	if err != nil || hash != binding.ManifestSHA256 {
		return fmt.Errorf("materialized artifact differs from immutable input")
	}
	return nil
}

// collectArtifact verifies opened file identities and snapshots only bounded,
// regular, singly linked files after all command descendants have exited.
func collectArtifact(root, blobDir string) (releasecontract.ArtifactManifest, string, error) {
	m := releasecontract.ArtifactManifest{SchemaVersion: 1, Kind: releasecontract.StaticSiteKind, Files: []releasecontract.ArtifactFile{}}
	abs, err := filepath.Abs(root)
	if err != nil {
		return m, "", err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return m, "", fmt.Errorf("artifact output directory unavailable or linked")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || !strings.EqualFold(resolved, abs) {
		return m, "", fmt.Errorf("artifact output traverses a link")
	}
	if blobDir != "" {
		if err = safeMkdir(blobDir); err != nil {
			return m, "", err
		}
	}
	entries := 0
	var total int64
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 4096 {
			return fmt.Errorf("artifact directory entry limit exceeded")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact links are forbidden")
		}
		if d.IsDir() {
			r, e := filepath.EvalSymlinks(path)
			if e != nil || !strings.EqualFold(r, path) {
				return fmt.Errorf("artifact directory link is forbidden")
			}
			return nil
		}
		rel, e := filepath.Rel(abs, path)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(rel)
		if !releasecontract.ValidPath(name) || len(m.Files) >= releasecontract.MaxFiles {
			return fmt.Errorf("unsafe artifact path or file count")
		}
		parents, e := artifactParents(abs, path)
		if e != nil {
			return e
		}
		before, e := os.Lstat(path)
		if e != nil || !before.Mode().IsRegular() || before.Size() > releasecontract.MaxFileBytes {
			return fmt.Errorf("artifact must be a bounded regular file")
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		opened, e := f.Stat()
		if e != nil || !os.SameFile(before, opened) || !artifactSingleLink(f) {
			_ = f.Close()
			return fmt.Errorf("artifact identity changed or hardlink forbidden")
		}
		b, e := io.ReadAll(io.LimitReader(f, releasecontract.MaxFileBytes+1))
		after, e2 := f.Stat()
		_ = f.Close()
		last, e3 := os.Lstat(path)
		if e != nil || e2 != nil || e3 != nil || int64(len(b)) != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(opened, last) || last.Mode()&os.ModeSymlink != 0 || !artifactParentsStable(parents) {
			return fmt.Errorf("artifact changed during collection")
		}
		total += int64(len(b))
		if total > releasecontract.MaxArtifactBytes {
			return fmt.Errorf("artifact total size limit exceeded")
		}
		hash := digest(b)
		if blobDir != "" {
			if e = writeEvidence(filepath.Join(blobDir, hash), b); e != nil {
				return e
			}
		}
		m.Files = append(m.Files, releasecontract.ArtifactFile{Path: name, SizeBytes: int64(len(b)), SHA256: hash})
		return nil
	})
	if err != nil {
		return m, "", err
	}
	return releasecontract.CanonicalManifest(m)
}

type artifactTransfer struct {
	Assignment       Assignment                             `json:"assignment"`
	Result           Result                                 `json:"result"`
	Manifest         releasecontract.ArtifactManifest       `json:"manifest"`
	ManifestSHA256   string                                 `json:"manifest_sha256"`
	ProfileID        string                                 `json:"profile_id"`
	ProfileSHA256    string                                 `json:"profile_sha256"`
	BlobDirectory    string                                 `json:"blob_directory"`
	ApplicationBuild *releasecontract.ApplicationBuildBasis `json:"application_build,omitempty"`
}

type artifactParent struct {
	path string
	info os.FileInfo
}

func artifactParents(root, path string) ([]artifactParent, error) {
	parents := []artifactParent{}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if !within(root, dir) {
			return nil, fmt.Errorf("artifact parent escaped dedicated root")
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("artifact parent linked or unavailable")
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil || !strings.EqualFold(resolved, dir) {
			return nil, fmt.Errorf("artifact parent traverses a link")
		}
		parents = append(parents, artifactParent{dir, info})
		if strings.EqualFold(dir, root) {
			return parents, nil
		}
	}
}
func artifactParentsStable(parents []artifactParent) bool {
	for _, parent := range parents {
		info, err := os.Lstat(parent.path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, parent.info) {
			return false
		}
		resolved, err := filepath.EvalSymlinks(parent.path)
		if err != nil || !strings.EqualFold(resolved, parent.path) {
			return false
		}
	}
	return true
}

func uploadArtifact(ctx context.Context, c *Client, p artifactTransfer) (releasecontract.ArtifactSealReceipt, error) {
	var state releasecontract.ArtifactUploadState
	var receipt releasecontract.ArtifactSealReceipt
	_, hash, err := releasecontract.CanonicalManifest(p.Manifest)
	if err != nil || hash != p.ManifestSHA256 {
		return receipt, fmt.Errorf("durable artifact manifest changed")
	}
	err = c.post(ctx, "artifact-uploads", releasecontract.ArtifactUploadRequest{ProjectID: c.ProjectID, Assignment: assignmentRef(p.Assignment), OperationID: artifactOperation(p.Assignment, "upload"), ProfileID: p.ProfileID, ProfileSHA256: p.ProfileSHA256, Manifest: p.Manifest}, &state)
	if err != nil {
		return receipt, err
	}
	if state.UploadID == "" || len(state.UploadID) > 100 || strings.ContainsAny(state.UploadID, "/\\") || state.ManifestSHA256 != hash {
		return receipt, fmt.Errorf("artifact upload identity mismatch")
	}
	allowed := map[string]int64{}
	for _, file := range p.Manifest.Files {
		allowed[file.SHA256] = file.SizeBytes
	}
	for _, blob := range state.MissingBlobs {
		size, ok := allowed[blob]
		if !ok {
			return receipt, fmt.Errorf("server requested blob outside manifest")
		}
		path := filepath.Join(p.BlobDirectory, blob)
		b, err := os.ReadFile(path)
		if err != nil || int64(len(b)) != size || digest(b) != blob {
			return receipt, fmt.Errorf("durable artifact blob changed")
		}
		resp, err := c.artifactRequest(ctx, http.MethodPut, "artifact-uploads/"+url.PathEscape(state.UploadID)+"/blobs/"+blob+"?project_id="+url.QueryEscape(c.ProjectID), p.Assignment, strings.NewReader(string(b)), int64(len(b)), artifactOperation(p.Assignment, "blob/"+blob))
		if err != nil {
			return receipt, err
		}
		_ = resp.Body.Close()
	}
	err = c.post(ctx, "artifact-uploads/"+url.PathEscape(state.UploadID)+"/seal", releasecontract.ArtifactSealRequest{ProjectID: c.ProjectID, Assignment: assignmentRef(p.Assignment), OperationID: artifactOperation(p.Assignment, "seal")}, &receipt)
	if err != nil {
		return receipt, err
	}
	var total int64
	for _, file := range p.Manifest.Files {
		total += file.SizeBytes
	}
	if receipt.State != "sealed" || receipt.ArtifactID == "" || receipt.ManifestSHA256 != hash || receipt.FileCount != len(p.Manifest.Files) || receipt.SizeBytes != total {
		return receipt, fmt.Errorf("sealed artifact receipt mismatch")
	}
	return receipt, nil
}

func finishArtifact(ctx context.Context, a Assignment, cfg ExecConfig, spec CommandSpec, w *attemptWorkspace, scope *applicationBuildWorkspace, inputDir string, r *Result) error {
	if w == nil || r.WorkspaceEvidence["clean"] != true || !releasecontract.ValidCommit(w.HeadCommit) {
		return fmt.Errorf("artifact proof requires a clean known source commit")
	}
	if w.HeadCommit != w.BaseCommit {
		return fmt.Errorf("artifact build/test must preserve its pinned source commit")
	}
	p, profileHash, err := artifactProfile(a.Command[0], spec)
	if err != nil {
		return err
	}
	if spec.ArtifactInput {
		binding := cfg.resolvedArtifact
		_, hash, err := collectArtifact(inputDir, "")
		if err != nil || hash != binding.ManifestSHA256 || w.BaseCommit != binding.SourceCommit || w.HeadCommit != binding.SourceCommit {
			return fmt.Errorf("tested artifact or pinned source changed")
		}
		r.ArtifactEvidence = &releasecontract.ArtifactEvidence{Kind: "package_test", ArtifactID: binding.ArtifactID, ManifestSHA256: hash, SourceCommit: binding.SourceCommit, ProfileID: p.ID, ProfileSHA256: profileHash}
		return nil
	}
	root := w.Source
	var applicationBuild *releasecontract.ApplicationBuildBasis
	if scope != nil {
		if err = scope.verify(ctx, w); err != nil {
			return err
		}
		root = scope.root
		applicationBuild = scope.basis
	}
	dir := filepath.Join(root, filepath.FromSlash(spec.ArtifactOutput))
	if !within(root, dir) {
		return fmt.Errorf("artifact output escaped attempt")
	}
	blobs := filepath.Join(w.Directory, "artifact-blobs")
	manifest, hash, err := collectArtifact(dir, blobs)
	if err != nil {
		return err
	}
	pending := artifactTransfer{Assignment: a, Result: *r, Manifest: manifest, ManifestSHA256: hash, ProfileID: p.ID, ProfileSHA256: profileHash, BlobDirectory: blobs, ApplicationBuild: applicationBuild}
	b, _ := json.MarshalIndent(pending, "", "  ")
	if err = writeEvidence(filepath.Join(w.Directory, "artifact-transfer.json"), b); err != nil {
		return err
	}
	if cfg.Journal != nil {
		if err = cfg.Journal.checkpointArtifact(pending); err != nil {
			return err
		}
	}
	receipt, err := uploadArtifact(ctx, cfg.Artifacts, pending)
	if err != nil {
		r.artifactPending = cfg.Journal != nil
		return err
	}
	r.ArtifactEvidence = &releasecontract.ArtifactEvidence{Kind: "build", ArtifactID: receipt.ArtifactID, ManifestSHA256: hash, SourceCommit: w.HeadCommit, ProfileID: p.ID, ProfileSHA256: profileHash, ApplicationBuild: applicationBuild}
	return nil
}

func resumeArtifact(ctx context.Context, c *Client, j *Journal, p artifactTransfer) error {
	if p.Result.ExitCode != 0 || p.Result.Stopped || p.Result.OutcomeUnknown || p.Result.Error != "" || p.Result.WorkspaceEvidence["clean"] != true {
		return fmt.Errorf("artifact checkpoint is not a known successful process")
	}
	head, ok := p.Result.WorkspaceEvidence["head_commit"].(string)
	if !ok || !releasecontract.ValidCommit(head) || !releasecontract.ValidProfileID(p.ProfileID) || !releasecontract.ValidDigest(p.ProfileSHA256) {
		return fmt.Errorf("artifact checkpoint source/profile invalid")
	}
	applicationBuild, err := applicationBuildAssignment(p.Assignment)
	if err != nil || (applicationBuild == nil) != (p.ApplicationBuild == nil) || applicationBuild != nil && (*applicationBuild != *p.ApplicationBuild || applicationBuild.SourceCommit != head) {
		return fmt.Errorf("artifact checkpoint application build does not match its original source")
	}
	receipt, err := uploadArtifact(ctx, c, p)
	if err != nil {
		return err
	}
	r := p.Result
	r.Assignment = p.Assignment
	r.ArtifactEvidence = &releasecontract.ArtifactEvidence{Kind: "build", ArtifactID: receipt.ArtifactID, ManifestSHA256: receipt.ManifestSHA256, SourceCommit: head, ProfileID: p.ProfileID, ProfileSHA256: p.ProfileSHA256, ApplicationBuild: applicationBuild}
	return j.Finish(r)
}

func ReplayArtifacts(ctx context.Context, c *Client, j *Journal) error {
	for _, p := range j.pendingArtifacts() {
		if err := resumeArtifact(ctx, c, j, p); err != nil {
			return err
		}
	}
	return nil
}
