package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/releasecontract"
)

type artifactUpload struct {
	ID, TokenHash, ProfileID, ProfileSHA256, ManifestSHA256, State, ArtifactID string
	Assignment                                                                 releasecontract.AssignmentRef
	Manifest                                                                   releasecontract.ArtifactManifest
	SizeBytes                                                                  int64
}

func (s *Service) loadArtifactUpload(ctx context.Context, q governanceQuery, p Principal, project, id string) (artifactUpload, error) {
	var u artifactUpload
	var raw string
	err := q.QueryRowContext(ctx, s.q(`SELECT id,token_hash,worker_id,run_id,node_id,attempt,fence,profile_id,profile_sha256,manifest_sha256,manifest,size_bytes,state,artifact_id FROM suma_artifact_uploads WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&u.ID, &u.TokenHash, &u.Assignment.WorkerID, &u.Assignment.RunID, &u.Assignment.NodeID, &u.Assignment.Attempt, &u.Assignment.Fence, &u.ProfileID, &u.ProfileSHA256, &u.ManifestSHA256, &raw, &u.SizeBytes, &u.State, &u.ArtifactID)
	if err == sql.ErrNoRows {
		return u, missing()
	}
	if err != nil {
		return u, err
	}
	err = json.Unmarshal([]byte(raw), &u.Manifest)
	return u, err
}

func (s *Service) artifactClockTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	clock := `SELECT unixepoch('now')`
	if s.Dialect == "postgres" {
		clock = `SELECT CAST(EXTRACT(EPOCH FROM clock_timestamp()) AS BIGINT)`
	}
	if s.Dialect == "oceanbase" {
		clock = `SELECT UNIX_TIMESTAMP()`
	}
	var n int64
	err := tx.QueryRowContext(ctx, clock).Scan(&n)
	return n, err
}

// Call only after the organization/credential lock. Current authority and the
// same database clock used by scheduler leases govern publication and input.
func (s *Service) currentArtifactAssignmentTx(ctx context.Context, tx *sql.Tx, p Principal, project string, a releasecontract.AssignmentRef) (dag.Node, error) {
	var spec dag.Node
	if !a.Valid() {
		return spec, invalid("执行身份不完整")
	}
	if err := dag.GuardProjectTx(ctx, tx, s.Dialect, p.TenantID, project); err != nil {
		return spec, err
	}
	var state, runState, worker, raw string
	var attempt int
	var fence, lease int64
	err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT n.state,r.state,n.worker_id,n.attempt,n.fence,n.lease_until,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`, true)), p.TenantID, project, a.RunID, a.NodeID).Scan(&state, &runState, &worker, &attempt, &fence, &lease, &raw)
	if err != nil {
		return spec, missing()
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return spec, err
	}
	if state != "running" || runState != "running" && runState != "paused" || worker != a.WorkerID || attempt != a.Attempt || fence != a.Fence || lease <= clock {
		return spec, conflict("执行租约已变化，不能新增制品或证明")
	}
	err = json.Unmarshal([]byte(raw), &spec)
	return spec, err
}

func (s *Service) StartArtifactUpload(ctx context.Context, actor Actor, project string, r releasecontract.ArtifactUploadRequest) (releasecontract.ArtifactUploadState, error) {
	p := actor.Projection()

	var out releasecontract.ArtifactUploadState
	m, hash, err := releasecontract.CanonicalManifest(r.Manifest)
	if err != nil {
		return out, invalid(err.Error())
	}
	r.Manifest = m
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.releaseWorkerTx(ctx, tx, actor, project, r.Assignment.WorkerID, r.ProfileID, r.ProfileSHA256, "build")
	if err != nil {
		return out, err
	}
	fingerprint, replay, err := s.commandReceipt(ctx, auth, r.OperationID, map[string]any{"artifact_upload": r})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(fingerprint), &out)
		return out, err
	}
	spec, err := s.currentArtifactAssignmentTx(ctx, tx, p, project, r.Assignment)
	if err != nil {
		return out, err
	}
	if spec.Type != "command" && spec.Type != "agent" || len(spec.Command) != 1 || spec.Command[0] != r.ProfileID {
		return out, invalid("制品构建需绑定管理员登记命令")
	}
	var total int64
	for _, f := range m.Files {
		total += f.SizeBytes
	}
	var reserved int64
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(SUM(size_bytes),0) FROM suma_artifact_uploads WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&reserved); err != nil {
		return out, err
	}
	if total > 1<<30-reserved {
		return out, &APIError{413, "artifact_quota_exceeded", "当前项目制品保留配额为 1 GiB；需要管理员扩展存储策略", nil}
	}
	var existing int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_artifact_uploads WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), p.TenantID, project, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt, r.Assignment.Fence).Scan(&existing); err != nil {
		return out, err
	}
	if existing > 0 {
		return out, conflict("本次构建已有上传会话，请恢复原操作")
	}
	id := newID()
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_artifact_uploads(id,tenant_id,project_id,worker_id,token_hash,run_id,node_id,attempt,fence,profile_id,profile_sha256,manifest_sha256,manifest,size_bytes,state,artifact_id,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'receiving','',?)`), id, p.TenantID, project, r.Assignment.WorkerID, actor.credential.digest, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt, r.Assignment.Fence, r.ProfileID, r.ProfileSHA256, hash, stringify(m), total, now())
	if err != nil {
		return out, err
	}
	missing := []string{}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if !seen[f.SHA256] {
			missing = append(missing, f.SHA256)
			seen[f.SHA256] = true
		}
	}
	out = releasecontract.ArtifactUploadState{UploadID: id, State: "receiving", ManifestSHA256: hash, MissingBlobs: missing}
	if err = s.saveCommandReceipt(ctx, auth, r.OperationID, fingerprint, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) artifactRoot() (*os.Root, error) {
	root := filepath.Join(s.DataDir, "release-artifacts")
	if err := os.MkdirAll(filepath.Join(root, "blobs"), 0700); err != nil {
		return nil, err
	}
	return os.OpenRoot(root)
}

var storageBlobPath = regexp.MustCompile(`^blobs/[0-9a-f]{32}$`)

func (s *Service) readArtifactBlob(path, digest string, size int64) ([]byte, error) {
	if !storageBlobPath.MatchString(path) || size < 0 || size > releasecontract.MaxFileBytes {
		return nil, conflict("制品存储定位或长度无效")
	}
	root, err := s.artifactRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, conflict("制品文件缺失或已变化")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, conflict("制品文件身份已变化")
	}
	b, err := io.ReadAll(io.LimitReader(f, size+1))
	if err != nil {
		return nil, err
	}
	end, err := f.Stat()
	if err != nil || !os.SameFile(opened, end) || end.Size() != size || int64(len(b)) != size {
		return nil, conflict("制品文件长度已变化")
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != digest {
		return nil, conflict("制品真实内容摘要不匹配")
	}
	return b, nil
}

func (s *Service) PutArtifactBlob(ctx context.Context, actor Actor, project, uploadID, blobHash, operation string, size int64, body io.Reader) error {
	p := actor.Projection()

	if !releasecontract.ValidDigest(blobHash) || len(operation) < 8 || len(operation) > 128 || size < 0 || size > releasecontract.MaxFileBytes {
		return invalid("制品摘要、长度或操作标识无效")
	}
	// Spool a bounded, server-named file before taking database locks. Nothing is
	// published until current authority, lease, manifest and bytes are committed.
	root, err := s.artifactRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	path := "blobs/" + newID()
	keep := false
	defer func() {
		if !keep {
			_ = root.Remove(path)
		}
	}()
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(body, releasecontract.MaxFileBytes+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != size || n > releasecontract.MaxFileBytes || hex.EncodeToString(hash.Sum(nil)) != blobHash {
		return conflict("收到的实际文件长度或 SHA-256 不匹配")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	u, err := s.loadArtifactUpload(ctx, tx, p, project, uploadID)
	if err != nil {
		return err
	}
	if u.TokenHash != actor.credential.digest {
		return forbidden()
	}
	auth, err := s.releaseWorkerTx(ctx, tx, actor, project, u.Assignment.WorkerID, u.ProfileID, u.ProfileSHA256, "build")
	if err != nil {
		return err
	}
	allowed := false
	for _, v := range u.Manifest.Files {
		if v.SHA256 == blobHash && v.SizeBytes == size {
			allowed = true
		}
	}
	if !allowed {
		return invalid("文件不属于已固定的制品清单")
	}
	fingerprint, replay, err := s.commandReceipt(ctx, auth, operation, map[string]any{"artifact_blob": uploadID, "sha256": blobHash, "size_bytes": size})
	if err != nil {
		return err
	}
	if replay {
		return nil
	}
	var oldPath string
	var oldSize int64
	err = tx.QueryRowContext(ctx, s.q(`SELECT storage_path,size_bytes FROM suma_artifact_blobs WHERE upload_id=? AND digest=?`), uploadID, blobHash).Scan(&oldPath, &oldSize)
	if err == nil {
		_, err = s.readArtifactBlob(oldPath, blobHash, oldSize)
		if err != nil {
			return err
		}
		if err = s.saveCommandReceipt(ctx, auth, operation, fingerprint, map[string]bool{"acknowledged": true}); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	if u.State != "receiving" {
		return conflict("上传已封存，不能新增文件")
	}
	if _, err = s.currentArtifactAssignmentTx(ctx, tx, p, project, u.Assignment); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_artifact_blobs(upload_id,digest,size_bytes,storage_path)VALUES(?,?,?,?)`), uploadID, blobHash, size, path); err != nil {
		return err
	}
	if err = s.saveCommandReceipt(ctx, auth, operation, fingerprint, map[string]bool{"acknowledged": true}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	keep = true
	return nil
}

func (s *Service) artifactUploadStateTx(ctx context.Context, tx *sql.Tx, u artifactUpload) (releasecontract.ArtifactUploadState, error) {
	out := releasecontract.ArtifactUploadState{UploadID: u.ID, State: u.State, ManifestSHA256: u.ManifestSHA256, MissingBlobs: []string{}}
	seen := map[string]bool{}
	for _, f := range u.Manifest.Files {
		if seen[f.SHA256] {
			continue
		}
		seen[f.SHA256] = true
		var count int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_artifact_blobs WHERE upload_id=? AND digest=? AND size_bytes=?`), u.ID, f.SHA256, f.SizeBytes).Scan(&count); err != nil {
			return out, err
		}
		if count == 0 {
			out.MissingBlobs = append(out.MissingBlobs, f.SHA256)
		}
	}
	return out, nil
}
func (s *Service) InspectArtifactUpload(ctx context.Context, actor Actor, project, id string) (out releasecontract.ArtifactUploadState, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeRunnerRead, func(auth *authorizedCommand) error {
		u, e := s.loadArtifactUpload(ctx, auth.tx, auth.currentPrincipal, project, id)
		if e != nil {
			return e
		}
		if u.TokenHash != actor.credential.digest {
			return forbidden()
		}
		if e = s.releaseWorkerBindingTx(ctx, auth.tx, auth, u.Assignment.WorkerID, u.ProfileID, u.ProfileSHA256, "build"); e != nil {
			return e
		}
		out, e = s.artifactUploadStateTx(ctx, auth.tx, u)
		return e
	})
	return out, err
}

func (s *Service) SealArtifactUpload(ctx context.Context, actor Actor, project, id string, r releasecontract.ArtifactSealRequest) (releasecontract.ArtifactSealReceipt, error) {
	p := actor.Projection()

	var out releasecontract.ArtifactSealReceipt
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.releaseWorkerTx(ctx, tx, actor, project, r.Assignment.WorkerID, "", "", "identity")
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, r.OperationID, map[string]any{"seal": id, "body": r})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	u, err := s.loadArtifactUpload(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if u.TokenHash != actor.credential.digest || u.Assignment != r.Assignment {
		return out, forbidden()
	}
	if u.State != "receiving" {
		return out, conflict("已封存，请恢复原封存操作")
	}
	if _, err = s.currentArtifactAssignmentTx(ctx, tx, p, project, r.Assignment); err != nil {
		return out, err
	}
	for _, f := range u.Manifest.Files {
		var path string
		var size int64
		if err = tx.QueryRowContext(ctx, s.q(`SELECT storage_path,size_bytes FROM suma_artifact_blobs WHERE upload_id=? AND digest=?`), id, f.SHA256).Scan(&path, &size); err != nil {
			return out, conflict("文件尚未完整收到")
		}
		if size != f.SizeBytes {
			return out, conflict("文件长度与清单不一致")
		}
		if _, err = s.readArtifactBlob(path, f.SHA256, size); err != nil {
			return out, err
		}
	}
	artifactID := newID()
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_artifacts(id,tenant_id,project_id,upload_id,manifest_sha256,manifest,size_bytes,created_at)VALUES(?,?,?,?,?,?,?,?)`), artifactID, p.TenantID, project, id, u.ManifestSHA256, stringify(u.Manifest), u.SizeBytes, now()); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_artifact_uploads SET state='sealed',artifact_id=? WHERE id=?`), artifactID, id); err != nil {
		return out, err
	}
	out = releasecontract.ArtifactSealReceipt{ArtifactID: artifactID, ManifestSHA256: u.ManifestSHA256, State: "sealed", SizeBytes: u.SizeBytes, FileCount: len(u.Manifest.Files)}
	if err = s.saveCommandReceipt(ctx, auth, r.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) releaseArtifactManifestTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (releasecontract.ArtifactManifest, error) {
	var m releasecontract.ArtifactManifest
	var raw, hash, upload string
	err := tx.QueryRowContext(ctx, s.q(`SELECT manifest,manifest_sha256,upload_id FROM suma_release_artifacts WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&raw, &hash, &upload)
	if err != nil {
		return m, missing()
	}
	if err = json.Unmarshal([]byte(raw), &m); err != nil {
		return m, err
	}
	canonical, digest, err := releasecontract.CanonicalManifest(m)
	if err != nil || digest != hash {
		return m, conflict("封存清单已变化")
	}
	refs := []artifactReadRef{}
	for _, f := range canonical.Files {
		var path string
		var size int64
		if err = tx.QueryRowContext(ctx, s.q(`SELECT storage_path,size_bytes FROM suma_artifact_blobs WHERE upload_id=? AND digest=?`), upload, f.SHA256).Scan(&path, &size); err != nil || size != f.SizeBytes {
			return m, conflict("制品不完整")
		}
		refs = append(refs, artifactReadRef{path: path, hash: f.SHA256, size: size})
	}
	if batch, ok := ctx.Value(artifactReadKey{}).(*artifactReadBatch); ok {
		if err = batch.record(refs); err != nil {
			return m, err
		}
	} else {
		for _, r := range refs {
			if _, err = s.readArtifactBlob(r.path, r.hash, r.size); err != nil {
				return m, err
			}
		}
	}
	return canonical, nil
}
func (s *Service) artifactBlobReferenceTx(ctx context.Context, tx *sql.Tx, p Principal, project, id, blobHash string) (artifactReadRef, error) {
	if !releasecontract.ValidDigest(blobHash) {
		return artifactReadRef{}, invalid("文件摘要无效")
	}
	var raw, upload string
	err := tx.QueryRowContext(ctx, s.q(`SELECT manifest,upload_id FROM suma_release_artifacts WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&raw, &upload)
	if err != nil {
		return artifactReadRef{}, missing()
	}
	var manifest releasecontract.ArtifactManifest
	if err = json.Unmarshal([]byte(raw), &manifest); err != nil {
		return artifactReadRef{}, err
	}
	allowed := false
	for _, f := range manifest.Files {
		if f.SHA256 == blobHash {
			allowed = true
		}
	}
	if !allowed {
		return artifactReadRef{}, missing()
	}
	var path string
	var size int64
	if err = tx.QueryRowContext(ctx, s.q(`SELECT storage_path,size_bytes FROM suma_artifact_blobs WHERE upload_id=? AND digest=?`), upload, blobHash).Scan(&path, &size); err != nil {
		return artifactReadRef{}, missing()
	}
	return artifactReadRef{path: path, hash: blobHash, size: size}, nil
}

func artifactRef(r dag.Result) releasecontract.AssignmentRef {
	return releasecontract.AssignmentRef{RunID: r.RunID, NodeID: r.NodeID, Attempt: r.Attempt, Fence: r.Fence, WorkerID: r.WorkerID}
}
func artifactBindingKey(a releasecontract.AssignmentRef) string {
	return fmt.Sprintf("%s/%s/%d/%d/%s", a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID)

}
func (s *Service) releaseArtifactBlobTx(ctx context.Context, tx *sql.Tx, p Principal, project, id, blobHash string) ([]byte, error) {
	ref, err := s.artifactBlobReferenceTx(ctx, tx, p, project, id, blobHash)
	if err != nil {
		return nil, err
	}
	return s.readArtifactBlob(ref.path, ref.hash, ref.size)
}

// Blob transport snapshots its exact storage reference under current authority,
// reads bounded immutable bytes without locks, then rechecks scope and reference.
func (s *Service) ReadArtifactBlob(ctx context.Context, actor Actor, project, id, hash string) ([]byte, error) {
	var ref artifactReadRef
	read := func(auth *authorizedCommand) error {
		var err error
		ref, err = s.artifactBlobReferenceTx(ctx, auth.tx, auth.currentPrincipal, project, id, hash)
		return err
	}
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	if err := s.withCurrentRead(ctx, actor, target, purposeProjectRead, read); err != nil {
		return nil, err
	}
	initial := ref
	b, err := s.readArtifactBlob(ref.path, ref.hash, ref.size)
	if err != nil {
		return nil, err
	}
	if err = s.withCurrentRead(ctx, actor, target, purposeProjectRead, read); err != nil {
		return nil, err
	}
	if ref != initial {
		return nil, conflict("制品读取依据已变化，请重试")
	}
	return b, nil
}
