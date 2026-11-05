package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

// Bundle bytes use the existing confined artifact root, but their independent
// 64 MiB limit does not expand release-artifact file or release-worker grants.
func (s *Service) readCandidateBundle(ref candidateTransfer) ([]byte, error) {
	return s.readFixedCandidateBytes(ref.path, ref.binding.BundleSHA256, ref.binding.SizeBytes)
}

func (s *Service) readFixedCandidateBytes(path, expectedSHA string, size int64) ([]byte, error) {
	if !storageBlobPath.MatchString(path) || !cc.ValidHash(expectedSHA) || size < 1 || size > cc.MaxBundleBytes {
		return nil, conflict("Candidate bundle storage reference is invalid")
	}
	root, err := s.artifactRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, conflict("Candidate bundle is missing or changed")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Size() != size {
		return nil, conflict("Candidate bundle is missing or changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, size+1))
	if err != nil {
		return nil, err
	}
	end, err := f.Stat()
	if err != nil || !os.SameFile(opened, end) || end.Size() != size || int64(len(data)) != size {
		return nil, conflict("Candidate bundle is missing or changed")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expectedSHA {
		return nil, conflict("Candidate bundle bytes do not match the manifest")
	}
	return data, nil
}

func (s *Service) ownedCandidateTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, id string, a cc.AssignmentRef) (*candidateAuthority, candidateTransfer, error) {
	var transfer candidateTransfer
	auth, err := s.candidateAuthorityTx(ctx, tx, subject, a)
	if err != nil {
		return nil, transfer, err
	}
	if auth.spec.Type != "command" && auth.spec.Type != "agent" {
		return nil, transfer, invalid("Candidate source must be a code command or agent")
	}
	transfer, err = s.candidateTransferTx(ctx, tx, subject, id)
	if err != nil {
		return nil, transfer, err
	}
	if transfer.subjectHash != subject.hash() || transfer.binding.Source != a {
		return nil, transfer, forbidden()
	}
	return auth, transfer, nil
}

// The preflight commits only authority coordination. No file IO is performed
// while the authorization or scheduler locks are held.
func (s *Service) candidateOwnedPreflight(ctx context.Context, subject candidateSubject, id string, a cc.AssignmentRef) (candidateTransfer, error) {
	var out candidateTransfer
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	_, out, err = s.ownedCandidateTx(ctx, tx, subject, id, a)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) PutCandidateBundle(ctx context.Context, actor Actor, project, id string, a cc.AssignmentRef, op string, size int64, body io.Reader) error {
	return s.putCandidateBundle(ctx, projectCandidateSubject(actor, project), id, a, op, size, body)
}
func (s *Service) putCandidateBundle(ctx context.Context, subject candidateSubject, id string, a cc.AssignmentRef, op string, size int64, body io.Reader) error {
	if len(op) < 8 || len(op) > 128 || size < 1 || size > cc.MaxBundleBytes || body == nil {
		return invalid("Invalid candidate bundle size or operation ID")
	}
	initial, err := s.candidateOwnedPreflight(ctx, subject, id, a)
	if err != nil {
		return err
	}
	if initial.binding.SizeBytes != size {
		return conflict("Candidate bundle bytes do not match the manifest")
	}
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
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(body, size+1))
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
	if n != size || hex.EncodeToString(hash.Sum(nil)) != initial.binding.BundleSHA256 {
		return conflict("Candidate bundle bytes do not match the manifest")
	}
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	defer tx.Rollback()
	auth, current, err := s.ownedCandidateTx(ctx, tx, subject, id, a)
	if err != nil {
		return err
	}
	// State may advance to sealed while an identical PUT is outside its
	// transaction. The immutable producer manifest must remain unchanged.
	expected := initial.binding
	expected.State = current.binding.State
	if current.binding != expected || current.binding.State != "uploading" && current.binding.State != "sealed" {
		return conflict("Candidate transfer metadata changed")
	}
	fingerprint, replay, err := s.candidateReceipt(ctx, auth, op, map[string]any{"candidate_put": id, "assignment": a, "sha256": current.binding.BundleSHA256, "size_bytes": size})
	if err != nil {
		return err
	}
	if replay {
		return nil
	}
	if current.path != initial.path {
		return conflict("Candidate transfer metadata changed")
	}
	if current.path == "" {
		if current.binding.State != "uploading" {
			return conflict("Candidate transfer is not accepting bytes")
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_candidate_transfers SET storage_path=? WHERE tenant_id=? AND project_id=? AND id=? AND storage_path=''`), path, subject.tenant, subject.project, id); err != nil {
			return err
		}
		keep = true
	}
	if err = s.saveCandidateReceipt(ctx, auth, op, fingerprint, map[string]bool{"acknowledged": true}); err != nil {
		keep = false
		return err
	}
	if err = tx.Commit(); err != nil {
		// A lost COMMIT acknowledgement can leave a durable reference. Retain
		// these verified bytes until storage reconciliation; an orphan is safer
		// than deleting a blob that the committed receipt already publishes.
		return err
	}
	return nil
}

func (s *Service) SealCandidateTransfer(ctx context.Context, actor Actor, project, id string, r cc.CommandRequest) (cc.Binding, error) {
	return s.sealCandidateTransfer(ctx, projectCandidateSubject(actor, project), id, r)
}
func (s *Service) sealCandidateTransfer(ctx context.Context, subject candidateSubject, id string, r cc.CommandRequest) (cc.Binding, error) {
	var out cc.Binding
	if r.ProjectID != subject.project {
		return out, forbidden()
	}
	initial, err := s.candidateOwnedPreflight(ctx, subject, id, r.Assignment)
	if err != nil {
		return out, err
	}
	if _, err = s.readCandidateBundle(initial); err != nil {
		return out, err
	}
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	auth, current, err := s.ownedCandidateTx(ctx, tx, subject, id, r.Assignment)
	if err != nil {
		return out, err
	}
	if current != initial {
		return out, conflict("Candidate transfer metadata changed")
	}
	hash, replay, err := s.candidateReceipt(ctx, auth, r.OperationID, map[string]any{"candidate_seal": id, "body": r})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if current.binding.State != "uploading" {
		return out, conflict("Candidate transfer already sealed; recover the original operation")
	}
	out = current.binding
	out.State = "sealed"
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_candidate_transfers SET binding=? WHERE tenant_id=? AND project_id=? AND id=?`), stringify(out), subject.tenant, subject.project, id); err != nil {
		return out, err
	}
	if err = s.saveCandidateReceipt(ctx, auth, r.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) candidateConsumerRead(ctx context.Context, subject candidateSubject, id string, a cc.AssignmentRef) (candidateTransfer, error) {
	var out candidateTransfer
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	auth, err := s.candidateAuthorityTx(ctx, tx, subject, a)
	if err != nil {
		return out, err
	}
	out, err = s.resolveCandidateTx(ctx, tx, subject, auth)
	if err != nil {
		return out, err
	}
	if out.binding.TransferID != id {
		return out, forbidden()
	}
	return out, tx.Commit()
}
func (s *Service) RunnerCandidateBundle(ctx context.Context, actor Actor, project, id string, a cc.AssignmentRef) ([]byte, error) {
	return s.runnerCandidateBundle(ctx, projectCandidateSubject(actor, project), id, a)
}
func (s *Service) runnerCandidateBundle(ctx context.Context, subject candidateSubject, id string, a cc.AssignmentRef) ([]byte, error) {
	initial, err := s.candidateConsumerRead(ctx, subject, id, a)
	if err != nil {
		return nil, err
	}
	b, err := s.readCandidateBundle(initial)
	if err != nil {
		return nil, err
	}
	current, err := s.candidateConsumerRead(ctx, subject, id, a)
	if err != nil {
		return nil, err
	}
	if current != initial {
		return nil, conflict("Candidate transfer metadata changed")
	}
	return b, nil
}
