package repositoryprovider

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func token(r *http.Request, env string) bool {
	actual := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	expected := os.Getenv(env)
	return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && expected != "" && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": code}})
}
func decode(r *http.Request, out any) error {
	b, e := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if e != nil || len(b) > 2<<20 {
		return fmt.Errorf("request exceeds bound")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	var value func() error
	value = func() error {
		v, e := d.Token()
		if e != nil {
			return e
		}
		delim, ok := v.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				name, ok := k.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate JSON field")
				}
				seen[name] = true
				if e = value(); e != nil {
					return e
				}
			}
		} else if delim == '[' {
			for d.More() {
				if e = value(); e != nil {
					return e
				}
			}
		} else {
			return fmt.Errorf("invalid JSON")
		}
		_, e = d.Token()
		return e
	}
	if e = value(); e != nil {
		return e
	}
	if _, e = d.Token(); e != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func DecodeConfig(raw []byte) (Config, error) {
	var cfg Config
	r := &http.Request{Body: io.NopCloser(bytes.NewReader(raw))}
	err := decode(r, &cfg)
	return cfg, err
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/source-inspections" {
			s.sourceInspectionHTTP(w, r)
			return
		}
		if r.URL.Path == "/source-exports" {
			s.sourceExportHTTP(w, r)
			return
		}
		runner, reconciler := token(r, s.cfg.RunnerTokenEnv), token(r, s.cfg.ReconcileTokenEnv)
		if !runner && !reconciler {
			writeError(w, 403, "repository_forbidden")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		ctx, cancel := context.WithTimeout(s.ctx, 90*time.Second)
		defer cancel()
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/targets/") {
			alias := strings.TrimPrefix(r.URL.Path, "/targets/")
			o, e := s.Target(ctx, alias)
			if e != nil {
				o.Available = false
				if o.Reason == "" {
					o.Reason = "repository target unavailable"
				}
			}
			writeJSON(w, 200, o)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/operations" {
			if !runner {
				writeError(w, 403, "repository_forbidden")
				return
			}
			var q rc.RepositoryRequest
			if decode(r, &q) != nil {
				writeError(w, 400, "repository_invalid_request")
				return
			}
			hash, e := rc.RepositoryRequestDigest(q)
			if e != nil || q.ProviderID != s.cfg.ProviderID || q.TenantID != s.cfg.TenantID || q.ProjectID != s.cfg.ProjectID {
				writeError(w, 403, "repository_binding_invalid")
				return
			}
			op, e := s.load(q.OperationID)
			if e == sql.ErrNoRows {
				op = operation{Request: rc.RepositoryNonSecretRequest{RepositoryExecution: q.RepositoryExecution, Assignment: q.Assignment}, Observation: rc.RepositoryObservation{Protocol: rc.RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: hash, Phase: q.Phase, State: "preparing", Version: 1, Execution: q.RepositoryExecution, ObservedAt: now()}}
				op.Observation.ObservationSHA256, _ = rc.RepositoryObservationDigest(op.Observation)
				e = s.save(&op)
			}
			if e != nil {
				writeError(w, 503, "repository_state_unavailable")
				return
			}
			if op.Observation.RequestSHA256 != hash {
				writeError(w, 409, "repository_operation_conflict")
				return
			}
			if e = s.process(ctx, &op, q.AuthorizationTicket); e != nil {
				writeError(w, 503, "repository_authorization_or_report_unavailable")
				return
			}
			writeJSON(w, 200, op.Observation)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/operations/") {
			writeError(w, 404, "repository_not_found")
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/operations/"), "/")
		if !rc.RepositoryValidOperationID(parts[0]) {
			writeError(w, 404, "repository_not_found")
			return
		}
		var reconcileRequest *rc.RepositoryReconcileRequest
		if r.Method == "POST" && len(parts) == 2 && parts[1] == "reconcile" && reconciler {
			var q rc.RepositoryReconcileRequest
			if decode(r, &q) != nil || q.OperationID != parts[0] {
				writeError(w, 400, "repository_invalid_reconciliation")
				return
			}
			reconcileRequest = &q
		}
		op, e := s.load(parts[0])
		if e == sql.ErrNoRows && reconcileRequest != nil && reconcileRequest.Request != nil {
			q := reconcileRequest
			request := q.Request
			hash, err := rc.RepositoryRequestDigest(rc.RepositoryRequest{RepositoryExecution: request.RepositoryExecution, Assignment: request.Assignment})
			t := s.targets[request.TargetAlias]
			if err != nil || hash != q.RequestSHA256 || request.OperationID != q.OperationID || request.ProviderID != s.cfg.ProviderID || request.TenantID != s.cfg.TenantID || request.ProjectID != s.cfg.ProjectID || request.Phase == "apply" && (t == nil || t.configSHA != request.TargetConfigSHA256 || t.config.Ref != request.TargetRef) {
				writeError(w, 409, "repository_reconciliation_binding_invalid")
				return
			}
			op = operation{Request: *request, Observation: rc.RepositoryObservation{Protocol: rc.RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: hash, Phase: request.Phase, State: "unknown", Version: 1, Execution: request.RepositoryExecution, ObservedAt: now()}}
			op.Observation.ObservationSHA256, _ = rc.RepositoryObservationDigest(op.Observation)
			e = s.save(&op)
		}
		if e != nil {
			writeError(w, 404, "repository_not_found")
			return
		}
		t := s.targets[op.Request.TargetAlias]
		if t == nil && !(reconcileRequest != nil && op.Request.Phase == "prepare") {
			writeError(w, 409, "repository_target_unavailable")
			return
		}
		if r.Method == "GET" && len(parts) == 1 {
			if e = s.validateTarget(ctx, t, false); e != nil {
				writeError(w, 409, "repository_target_drift")
				return
			}
			observed, e := s.inspect(ctx, t, op)
			if e != nil {
				writeError(w, 409, "repository_receipt_invalid")
				return
			}
			writeJSON(w, 200, observed)
			return
		}
		if reconcileRequest != nil {
			q := *reconcileRequest
			if q.Request != nil {
				hash, err := rc.RepositoryRequestDigest(rc.RepositoryRequest{RepositoryExecution: q.Request.RepositoryExecution, Assignment: q.Request.Assignment})
				if err != nil || hash != op.Observation.RequestSHA256 {
					writeError(w, 409, "repository_reconciliation_binding_invalid")
					return
				}
			}
			if e = s.reconcile(ctx, &op, q); e != nil {
				writeError(w, 409, "repository_reconciliation_unavailable")
				return
			}
			writeJSON(w, 200, op.Observation)
			return
		}
		writeError(w, 405, "repository_method_not_allowed")
	})
}
