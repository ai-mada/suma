package deployer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func (s *Server) start(op operation, ticket string) {
	s.mu.Lock()
	if s.ctx.Err() != nil || s.working[op.Request.OperationID] != nil {
		s.mu.Unlock()
		return
	}
	latest, err := s.load(op.Request.OperationID)
	if err != nil || terminal(latest.Observation.State) {
		s.mu.Unlock()
		return
	}
	op = latest
	ctx, cancel := context.WithCancel(s.ctx)
	s.working[op.Request.OperationID] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() { s.mu.Lock(); delete(s.working, op.Request.OperationID); s.mu.Unlock(); cancel() }()
		s.perform(ctx, &op, ticket)
	}()
}

func (s *Server) validGrant(op *operation, g releasecontract.DeploymentAuthorizationGrant) bool {
	r := op.Request
	p := g.Plan
	hash, _ := releasecontract.CanonicalDigest(p)
	return g.State == "authorized" && g.AuthorizationID != "" && g.ProviderID == s.cfg.ProviderID && g.OperationID == r.OperationID && g.RequestSHA256 == op.Observation.RequestSHA256 && reflect.DeepEqual(g.Assignment, r.Assignment) && hash == r.PlanSHA256 && p.ProviderID == r.ProviderID && p.EnvironmentID == r.EnvironmentID && p.ExpectedGeneration == r.ExpectedGeneration && p.Generation == r.Generation && p.ArtifactID == r.ArtifactID && p.ArtifactSHA256 == r.ArtifactSHA256 && p.ReservedExecution.RunID == r.Assignment.RunID && p.ReservedExecution.NodeID == r.Assignment.NodeID && s.addresses[p.SiteID] != "" && (p.Action == "deploy" || p.Action == "rollback") && releasecontract.ValidateHealthPolicy(p.Health, true) == nil
}
func (s *Server) ensureEnvironment(op *operation) error {
	a, err := s.active(op.Request.EnvironmentID)
	if err == sql.ErrNoRows {
		if op.Request.ExpectedGeneration != 0 {
			return fmt.Errorf("provider has no durable state for existing environment generation")
		}
		_, err = s.db.Exec(`INSERT INTO environments(id,site_id,fence_generation,active_generation,artifact_id,artifact_sha256,directory,operation_id)VALUES(?,?,0,0,'','','','')`, op.Request.EnvironmentID, op.Grant.Plan.SiteID)
		return err
	}
	if err != nil {
		return err
	}
	var site string
	if err = s.db.QueryRow(`SELECT site_id FROM environments WHERE id=?`, op.Request.EnvironmentID).Scan(&site); err != nil || site != op.Grant.Plan.SiteID {
		return fmt.Errorf("environment site binding differs")
	}
	if a.Fence == op.Request.Generation && a.OperationID == op.Request.OperationID && a.Digest == op.Request.ArtifactSHA256 {
		return nil
	}
	if a.Fence != op.Request.ExpectedGeneration {
		return fmt.Errorf("provider generation conflict")
	}
	return nil
}

func (s *Server) perform(ctx context.Context, op *operation, ticket string) {
	if op.Grant == nil {
		if ticket == "" {
			return
		}
		var grant releasecontract.DeploymentAuthorizationGrant
		err := s.control(ctx, "POST", "deployment-authorizations/consume", releasecontract.ConsumeDeploymentAuthorization{ProviderID: s.cfg.ProviderID, AuthorizationTicket: ticket, OperationID: op.Request.OperationID, Assignment: op.Request.Assignment, CallerWorkerID: op.Request.Assignment.WorkerID, RequestSHA256: op.Observation.RequestSHA256}, &grant)
		ticket = ""
		if s.cfg.afterConsume != nil {
			s.cfg.afterConsume()
		}
		if err != nil {
			if recovered, inspectErr := s.inspectAuthorization(ctx, op); inspectErr == nil {
				grant = recovered
				err = nil
			}
		}
		if err != nil || !s.validGrant(op, grant) {
			op.Observation.Error = &releasecontract.DeploymentError{Code: "authorization_invalid", Message: "online deployment authorization rejected or unavailable"}
			_ = s.transition(op, "failed")
			return
		}
		op.Grant = &grant
		if s.transition(op, "authorized") != nil {
			return
		}
	}
	if !s.validGrant(op, *op.Grant) {
		return
	}
	lock, _ := s.environmentLocks.LoadOrStore(op.Request.EnvironmentID, &sync.Mutex{})
	environmentLock := lock.(*sync.Mutex)
	environmentLock.Lock()
	defer environmentLock.Unlock()
	if err := s.ensureEnvironment(op); err != nil {
		s.fail(op, "environment_generation_conflict", false)
		return
	}
	active, err := s.active(op.Request.EnvironmentID)
	if err != nil {
		s.fail(op, "provider_state_unavailable", false)
		return
	}
	if active.OperationID == op.Request.OperationID && active.Generation == op.Request.Generation {
		op.Observation.Effect = releasecontract.DeploymentEffect{State: "applied", PreviousActiveGeneration: op.Observation.Effect.PreviousActiveGeneration, ActiveGeneration: active.Generation, ActiveArtifactSHA256: active.Digest, ObservedAt: now()}
		op.Observation.FenceGeneration = active.Fence
	} else {
		if op.Grant.Plan.Action == "rollback" {
			source, e := s.load(op.Grant.Plan.RollbackSourceOperationID)
			if e != nil || source.Observation.State != "succeeded" || source.Request.EnvironmentID != op.Request.EnvironmentID || source.Request.ArtifactID != op.Request.ArtifactID || source.Request.ArtifactSHA256 != op.Request.ArtifactSHA256 {
				s.fail(op, "rollback_source_invalid", false)
				return
			}
		}
		var requested int
		_ = s.db.QueryRow(`SELECT cancel_requested FROM operations WHERE id=?`, op.Request.OperationID).Scan(&requested)
		if requested != 0 || ctx.Err() != nil {
			s.fail(op, "cancel_requested", requested != 0)
			return
		}
		if err = s.stage(ctx, op); err != nil {
			s.fail(op, "artifact_staging_failed", ctx.Err() != nil && s.ctx.Err() == nil)
			return
		}
		if op.EffectGrant == nil {
			var grant releasecontract.DeploymentEffectGrant
			err = s.control(ctx, "POST", "deployment-authorizations/begin-effect", releasecontract.BeginDeploymentEffect{AuthorizationID: op.Grant.AuthorizationID, OperationID: op.Request.OperationID, Assignment: op.Request.Assignment, RequestSHA256: op.Observation.RequestSHA256, Generation: op.Request.Generation}, &grant)
			if err != nil || grant.State != "granted" || grant.EffectGrantID == "" || grant.OperationID != op.Request.OperationID || grant.Generation != op.Request.Generation {
				s.fail(op, "effect_authorization_rejected", false)
				return
			}
			op.EffectGrant = &grant
			if s.transition(op, "applying") != nil {
				return
			}
		}
		_ = s.db.QueryRow(`SELECT cancel_requested FROM operations WHERE id=?`, op.Request.OperationID).Scan(&requested)
		if requested != 0 || ctx.Err() != nil {
			s.fail(op, "cancel_requested", requested != 0)
			return
		}
		if err = s.activate(op); err != nil {
			s.fail(op, "activation_generation_conflict", false)
			return
		}
		if s.cfg.afterActivate != nil {
			s.cfg.afterActivate()
		}
	}
	if op.Manifest == nil || verifyRevision(op.RevisionDirectory, *op.Manifest) != nil {
		s.fail(op, "active_revision_changed", false)
		return
	}
	if err = s.health(ctx, op); err != nil {
		s.fail(op, "http_health_failed", false)
		return
	}
	op.Observation.Error = nil
	_ = s.transition(op, "succeeded")
	s.report(op)
}

func (s *Server) stage(ctx context.Context, op *operation) error {
	if op.Manifest == nil {
		var artifact struct {
			ID             string                           `json:"id"`
			ManifestSHA256 string                           `json:"manifest_sha256"`
			Manifest       releasecontract.ArtifactManifest `json:"manifest"`
			State          string                           `json:"state"`
		}
		path := "artifacts/" + url.PathEscape(op.Request.ArtifactID) + "?operation_id=" + url.QueryEscape(op.Request.OperationID)
		if err := s.control(ctx, "GET", path, nil, &artifact); err != nil {
			return err
		}
		m, hash, err := releasecontract.CanonicalManifest(artifact.Manifest)
		if err != nil || artifact.ID != op.Request.ArtifactID || artifact.State != "sealed" || hash != op.Request.ArtifactSHA256 || artifact.ManifestSHA256 != hash {
			return fmt.Errorf("sealed artifact identity differs")
		}
		op.Manifest = &m
	}
	revision := filepath.Join(s.cfg.Root, "revisions", op.Request.OperationID)
	if _, err := os.Lstat(revision); err == nil {
		if err = verifyRevision(revision, *op.Manifest); err != nil {
			return err
		}
		op.RevisionDirectory = revision
		return s.transition(op, "staging")
	}
	stage := filepath.Join(s.cfg.Root, "staging", op.Request.OperationID)
	if err := safeDirectory(stage); err != nil {
		return err
	}
	for _, file := range op.Manifest.Files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		path := filepath.Join(stage, filepath.FromSlash(file.Path))
		if _, err := safeFile(stage, path, file.SizeBytes, file.SHA256); err == nil {
			continue
		}
		if err := safeDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		body, err := s.controlBlob(ctx, op, file.SHA256, file.SizeBytes)
		if err != nil {
			return err
		}
		tmpRoot := filepath.Join(s.cfg.Root, "tmp", op.Request.OperationID)
		if err = safeDirectory(tmpRoot); err != nil {
			return err
		}
		temp, err := os.CreateTemp(tmpRoot, "artifact-*.tmp")
		if err != nil {
			return err
		}
		name := temp.Name()
		_, err = temp.Write(body)
		if err == nil {
			err = temp.Sync()
		}
		closeErr := temp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = replaceProviderFile(name, path)
		}
		_ = os.Remove(name)
		if err != nil {
			return err
		}
	}
	if err := verifyRevision(stage, *op.Manifest); err != nil {
		return err
	}
	if err := safeDirectory(filepath.Dir(revision)); err != nil {
		return err
	}
	if err := os.Rename(stage, revision); err != nil {
		return err
	}
	op.RevisionDirectory = revision
	return s.transition(op, "staging")
}

func (s *Server) activate(op *operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current activeRevision
	err = tx.QueryRow(`SELECT fence_generation,active_generation,artifact_id,artifact_sha256,directory,operation_id FROM environments WHERE id=?`, op.Request.EnvironmentID).Scan(&current.Fence, &current.Generation, &current.ArtifactID, &current.Digest, &current.Directory, &current.OperationID)
	if err != nil || current.Fence != op.Request.ExpectedGeneration {
		return fmt.Errorf("activation fence differs")
	}
	result, err := tx.Exec(`UPDATE environments SET fence_generation=?,active_generation=?,artifact_id=?,artifact_sha256=?,directory=?,operation_id=? WHERE id=? AND fence_generation=?`, op.Request.Generation, op.Request.Generation, op.Request.ArtifactID, op.Request.ArtifactSHA256, op.RevisionDirectory, op.Request.OperationID, op.Request.EnvironmentID, op.Request.ExpectedGeneration)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return fmt.Errorf("activation CAS lost")
	}
	op.Observation.FenceGeneration = op.Request.Generation
	op.Observation.Effect = releasecontract.DeploymentEffect{State: "applied", PreviousActiveGeneration: current.Generation, ActiveGeneration: op.Request.Generation, ActiveArtifactSHA256: op.Request.ArtifactSHA256, ObservedAt: now()}
	op.Observation.State = "checking_health"
	op.Observation.Version++
	op.Observation.EvidenceSHA256, _ = releasecontract.ObservationDigest(op.Observation)
	raw, _ := json.Marshal(op)
	if _, err = tx.Exec(`UPDATE operations SET record=? WHERE id=?`, string(raw), op.Request.OperationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) fail(op *operation, code string, cancelled bool) {
	op.Observation.Error = &releasecontract.DeploymentError{Code: code, Message: code}
	state := "failed"
	active, err := s.active(op.Request.EnvironmentID)
	if err == nil {
		op.Observation.FenceGeneration = active.Fence
		op.Observation.Effect.ActiveGeneration = active.Generation
		op.Observation.Effect.ActiveArtifactSHA256 = active.Digest
		op.Observation.Effect.ObservedAt = now()
		if active.OperationID == op.Request.OperationID {
			op.Observation.Effect.State = "applied"
			if op.Observation.Health.State != "failed" {
				op.Observation.Health.State = "unknown"
				state = "reconciling"
			}
		} else {
			op.Observation.Effect.State = "not_started"
			if op.Grant != nil && active.Fence == op.Request.ExpectedGeneration && s.ctx.Err() == nil {
				result, e := s.db.Exec(`UPDATE environments SET fence_generation=? WHERE id=? AND fence_generation=?`, op.Request.Generation, op.Request.EnvironmentID, op.Request.ExpectedGeneration)
				if e == nil {
					count, _ := result.RowsAffected()
					if count == 1 {
						op.Observation.FenceGeneration = op.Request.Generation
					}
				}
			}
		}
	}
	if cancelled && op.Observation.Effect.State == "not_started" {
		state = "cancelled"
	}
	if s.ctx.Err() != nil {
		state = "reconciling"
	}
	if err = s.transition(op, state); err == nil && op.Grant != nil {
		s.report(op)
	}
}
