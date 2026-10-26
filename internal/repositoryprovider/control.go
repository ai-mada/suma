package repositoryprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (s *Server) control(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.cfg.ControlURL, "/")+"/api/v1/repository-provider/"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(s.cfg.ControlTokenEnv))
	req.Header.Set("Content-Type", "application/json")
	c := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("repository control request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("repository control request rejected: HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
	}
	return nil
}
func (s *Server) bundle(ctx context.Context, op *operation) ([]byte, error) {
	if op.Grant == nil {
		return nil, fmt.Errorf("repository authorization missing")
	}
	q := rc.RepositoryBundleRequest{AuthorizationID: op.Grant.AuthorizationID, OperationID: op.Request.OperationID, Assignment: op.Request.Assignment, RequestSHA256: op.Observation.RequestSHA256}
	data, _ := json.Marshal(q)
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.cfg.ControlURL, "/")+"/api/v1/repository-provider/bundle", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(s.cfg.ControlTokenEnv))
	req.Header.Set("Content-Type", "application/json")
	c := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repository bundle unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("repository bundle authorization rejected")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, cc.MaxBundleBytes+1))
	if err != nil || len(b) == 0 || int64(len(b)) > cc.MaxBundleBytes {
		return nil, fmt.Errorf("repository bundle exceeds bound")
	}
	return b, nil
}
func (s *Server) reportLocked(op *operation) error {
	if op.Grant == nil && op.Observation.Receipt == nil && op.Observation.EffectGrantID == "" {
		return nil
	}
	if err := s.control(s.ctx, "observations", op.Observation, nil); err != nil {
		return err
	}
	op.ReportedVersion = op.Observation.Version
	return s.save(op)
}
func (s *Server) reportLoop() {
	defer s.wg.Done()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
		}
		s.mu.Lock()
		rows, e := s.db.Query(`SELECT record FROM operations`)
		ops := []operation{}
		if e == nil {
			for rows.Next() {
				var raw string
				var op operation
				if rows.Scan(&raw) == nil && json.Unmarshal([]byte(raw), &op) == nil && (op.Grant != nil || op.Observation.Receipt != nil || op.Observation.EffectGrantID != "") && op.ReportedVersion < op.Observation.Version {
					ops = append(ops, op)
				}
			}
			rows.Close()
		}
		for _, op := range ops {
			_ = s.reportLocked(&op)
		}
		s.mu.Unlock()
	}
}
func (s *Server) validGrant(op *operation, g rc.RepositoryAuthorizationGrant) bool {
	return g.AuthorizationID != "" && g.OperationID == op.Request.OperationID && g.ProviderID == s.cfg.ProviderID && g.RequestSHA256 == op.Observation.RequestSHA256 && rc.RepositoryStableAssignment(g.Assignment) == rc.RepositoryStableAssignment(op.Request.Assignment) && rc.RepositoryExecutionEqual(g.Execution, op.Request.RepositoryExecution)
}
func (s *Server) process(ctx context.Context, op *operation, ticket string) error {
	e := op.Request.RepositoryExecution
	if e.Phase == "prepare" && op.Observation.State == "blocked" {
		return s.reportLocked(op)
	}
	t := s.targets[e.TargetAlias]
	if t == nil || t.config.Ref != e.TargetRef || t.configSHA != e.TargetConfigSHA256 {
		return fmt.Errorf("repository target binding mismatch")
	}
	// Existing durable intents never blindly repeat a Git effect. Recover the same
	// receipt, or remain unknown until explicit current reconciliation fences it.
	if op.Grant != nil && !(e.Phase == "prepare" && op.Observation.State == "preparing") {
		observed, err := s.inspect(ctx, t, *op)
		if err != nil {
			return err
		}
		op.Observation = observed
		if op.Observation.State == "applied" || op.Observation.State == "not_applied" || op.Observation.State == "prepared" || op.Observation.State == "blocked" {
			if err = s.save(op); err != nil {
				return err
			}
			return s.reportLocked(op)
		}
		return s.reportLocked(op)
	}
	var grant rc.RepositoryAuthorizationGrant
	var err error
	if op.Grant == nil {
		err = s.control(ctx, "inspect-authorization", rc.RepositoryInspectAuthorization{OperationID: e.OperationID, Assignment: op.Request.Assignment, RequestSHA256: op.Observation.RequestSHA256}, &grant)
		if err != nil {
			err = s.control(ctx, "consume", rc.RepositoryConsumeAuthorization{ProviderID: s.cfg.ProviderID, AuthorizationTicket: ticket, OperationID: e.OperationID, Assignment: op.Request.Assignment, RequestSHA256: op.Observation.RequestSHA256}, &grant)
		}
		if err != nil || !s.validGrant(op, grant) {
			return fmt.Errorf("repository current authorization unavailable")
		}
		op.Grant = &grant
		if err = s.save(op); err != nil {
			return err
		}
	} else {
		grant = *op.Grant
	}
	if err = s.validateTarget(ctx, t, false); err != nil {
		return s.failOperation(op, err)
	}
	if err = s.importBundle(ctx, t, op); err != nil {
		return s.failOperation(op, err)
	}
	preview, err := s.preview(ctx, t, e)
	if err != nil {
		return s.failOperation(op, err)
	}
	if e.Phase == "prepare" {
		op.Observation.Preview = preview
		if err = s.transition(op, "prepared", ""); err != nil {
			return err
		}
		return s.reportLocked(op)
	}
	if preview.PreparedSHA256 != e.PreparedSHA256 {
		return s.failOperation(op, fmt.Errorf("approved repository diff changed"))
	}
	var effect rc.RepositoryEffectGrant
	err = s.control(ctx, "begin-effect", rc.RepositoryBeginEffect{AuthorizationID: grant.AuthorizationID, OperationID: e.OperationID, Assignment: op.Request.Assignment, RequestSHA256: op.Observation.RequestSHA256, Generation: e.Generation}, &effect)
	if err != nil || effect.State != "granted" || effect.EffectGrantID == "" || effect.OperationID != e.OperationID || effect.Generation != e.Generation {
		return s.failOperation(op, fmt.Errorf("repository effect grant unavailable"))
	}
	op.Effect = &effect
	if err = s.transition(op, "applying", ""); err != nil {
		return err
	}
	if err = s.validateTarget(ctx, t, false); err != nil {
		return s.failOperation(op, err)
	}
	err = s.commitGit(ctx, t, op, false, effect.EffectGrantID)
	if err != nil {
		observed, inspectErr := s.inspect(ctx, t, *op)
		if inspectErr == nil && (observed.State == "applied" || observed.State == "not_applied") {
			op.Observation = observed
		} else {
			return s.failOperation(op, fmt.Errorf("repository Git outcome requires reconciliation"))
		}
	}
	if err = s.save(op); err != nil {
		return err
	}
	return s.reportLocked(op)
}
func (s *Server) failOperation(op *operation, err error) error {
	state := "blocked"
	if op.Request.Phase == "apply" {
		state = "unknown"
	}
	reason := err.Error()
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		reason = "repository local storage unavailable"
	}
	if strings.Contains(reason, s.cfg.StateDir) || strings.Contains(reason, s.cfg.GitExecutable) {
		reason = "repository local evidence unavailable"
	}
	for _, target := range s.targets {
		if strings.Contains(reason, target.config.BareRepository) {
			reason = "repository local evidence unavailable"
		}
	}
	if e := s.transition(op, state, reason); e != nil {
		return e
	}
	return s.reportLocked(op)
}
func (s *Server) reconcile(ctx context.Context, op *operation, q rc.RepositoryReconcileRequest) error {
	if q.OperationID != op.Request.OperationID || q.RequestSHA256 != op.Observation.RequestSHA256 {
		return fmt.Errorf("repository reconciliation binding mismatch")
	}
	t := s.targets[op.Request.TargetAlias]
	if t == nil && op.Request.Phase == "apply" {
		return fmt.Errorf("repository target unavailable")
	}
	if op.Request.Phase == "apply" {
		if err := s.validateTarget(ctx, t, false); err != nil {
			return err
		}
	}
	known := op.Observation
	var err error
	if op.Request.Phase == "apply" {
		known, err = s.inspect(ctx, t, *op)
	}
	if err != nil {
		return err
	}
	if known.State == "applied" || known.State == "not_applied" || op.Request.Phase == "prepare" && (known.State == "prepared" || known.State == "blocked") {
		op.Observation = known
		if err = s.save(op); err != nil {
			return err
		}
		return s.reportLocked(op)
	}
	var grant rc.RepositoryReconciliationGrant
	err = s.control(ctx, "consume-reconciliation", rc.RepositoryConsumeReconciliation{ProviderID: s.cfg.ProviderID, OperationID: q.OperationID, RequestSHA256: q.RequestSHA256, ReconciliationTicket: q.ReconciliationTicket, Execution: op.Request.RepositoryExecution, Assignment: op.Request.Assignment}, &grant)
	if err != nil || grant.ReconciliationID == "" || grant.State != "granted" || grant.OperationID != q.OperationID || grant.RequestSHA256 != q.RequestSHA256 || grant.Generation != op.Request.Generation || !rc.RepositoryExecutionEqual(grant.Execution, op.Request.RepositoryExecution) || rc.RepositoryStableAssignment(grant.Assignment) != rc.RepositoryStableAssignment(op.Request.Assignment) {
		return fmt.Errorf("repository reconciliation grant invalid")
	}
	if op.Request.Phase == "prepare" {
		op.Observation.EffectGrantID = grant.ReconciliationID
		if err = s.transition(op, "blocked", "preparation_closed_by_reconciliation"); err != nil {
			return err
		}
		return s.reportLocked(op)
	}
	// Creating this same op receipt and CASing the same old fence races safely
	// against an orphan's positive transaction; exactly one transaction can win.
	err = s.commitGit(ctx, t, op, true, grant.ReconciliationID)
	if err != nil {
		observed, inspectErr := s.inspect(ctx, t, *op)
		if inspectErr != nil || observed.Receipt == nil {
			return s.failOperation(op, fmt.Errorf("repository negative fence outcome unknown"))
		}
		op.Observation = observed
	}
	if err = s.save(op); err != nil {
		return err
	}
	return s.reportLocked(op)
}
