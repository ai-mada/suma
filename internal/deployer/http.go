package deployer

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func (s *Server) worker(r *http.Request) (string, bool) {
	value := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if value == "" || value == r.Header.Get("Authorization") {
		return "", false
	}
	for worker, env := range s.cfg.Workers {
		expected := secret(env)
		if expected != "" && subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1 {
			return worker, true
		}
	}
	return "", false
}
func (s *Server) isReconciler(r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return false
	}
	actual := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	expected := secret(s.cfg.ReconcileTokenEnv)
	return expected != "" && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": code}})
}
func decodeRequest(r *http.Request, out any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return fmt.Errorf("body exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	var readValue func() error
	readValue = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate JSON key")
				}
				seen[key] = true
				if err = readValue(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err = readValue(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err = readValue(); err != nil {
		return err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	decoder = json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		worker, workerOK := s.worker(r)
		if r.Method == "POST" && r.URL.Path == "/v2/deployments" {
			if !workerOK {
				writeError(w, 403, "forbidden")
				return
			}
			var request releasecontract.DeploymentRequest
			if err := decodeRequest(r, &request); err != nil {
				writeError(w, 400, "invalid_request")
				return
			}
			hash, err := releasecontract.DeploymentRequestDigest(request)
			if err != nil || request.ProviderID != s.cfg.ProviderID || request.Assignment.WorkerID != worker {
				writeError(w, 403, "authorization_invalid")
				return
			}
			s.mu.Lock()
			op, err := s.load(request.OperationID)
			created := err == sql.ErrNoRows
			if created {
				op = operation{Request: releasecontract.NonSecretDeploymentRequest{ReleaseExecution: request.ReleaseExecution, Assignment: request.Assignment}, Observation: releasecontract.DeploymentObservation{Protocol: request.Protocol, OperationID: request.OperationID, RequestSHA256: hash, ProviderID: request.ProviderID, PlanID: request.PlanID, PlanRevision: request.PlanRevision, PlanSHA256: request.PlanSHA256, EnvironmentID: request.EnvironmentID, Generation: request.Generation, State: "received", Version: 1, Effect: releasecontract.DeploymentEffect{State: "not_started", ObservedAt: now()}, Health: releasecontract.HealthObservation{State: "not_checked", Generation: request.Generation, ArtifactSHA256: request.ArtifactSHA256, Checks: []releasecontract.HealthCheckResult{}}}}
				op.Observation.EvidenceSHA256, _ = releasecontract.ObservationDigest(op.Observation)
				raw, _ := json.Marshal(op)
				_, err = s.db.Exec(`INSERT INTO operations(id,record)VALUES(?,?)`, request.OperationID, string(raw))
			}
			s.mu.Unlock()
			if err != nil {
				writeError(w, 503, "provider_state_unavailable")
				return
			}
			if op.Observation.RequestSHA256 != hash || op.Request.Assignment.WorkerID != worker {
				writeError(w, 409, "idempotency_payload_conflict")
				return
			}
			if !terminal(op.Observation.State) {
				s.start(op, request.AuthorizationTicket)
			}
			status := 200
			if created {
				status = 202
			}
			writeJSON(w, status, op.Observation)
			return
		}
		prefix := "/v2/operations/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			writeError(w, 404, "not_found")
			return
		}
		if !workerOK && !s.isReconciler(r) {
			writeError(w, 403, "forbidden")
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
		if len(parts) < 1 || !releasecontract.ValidOperationID(parts[0]) {
			writeError(w, 404, "not_found")
			return
		}
		op, err := s.load(parts[0])
		if err != nil {
			writeError(w, 404, "not_found")
			return
		}
		if !s.isReconciler(r) && (!workerOK || worker != op.Request.Assignment.WorkerID) {
			writeError(w, 403, "forbidden")
			return
		}
		if r.Method == "GET" && len(parts) == 1 {
			writeJSON(w, 200, op.Observation)
			return
		}
		if r.Method == "POST" && len(parts) == 2 && parts[1] == "cancel" && workerOK && worker == op.Request.Assignment.WorkerID {
			var body struct {
				OperationID string `json:"operation_id"`
			}
			if decodeRequest(r, &body) != nil || !releasecontract.ValidOperationID(body.OperationID) {
				writeError(w, 400, "invalid_request")
				return
			}
			_, err = s.db.Exec(`UPDATE operations SET cancel_requested=1 WHERE id=?`, parts[0])
			if err != nil {
				writeError(w, 503, "provider_state_unavailable")
				return
			}
			s.mu.Lock()
			if cancel := s.working[parts[0]]; cancel != nil {
				cancel()
			}
			s.mu.Unlock()
			writeJSON(w, 202, op.Observation)
			return
		}
		writeError(w, 405, "method_not_allowed")
	})
}
