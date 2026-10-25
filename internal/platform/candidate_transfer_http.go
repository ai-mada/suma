package platform

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

func candidateAssignmentHeader(r *http.Request) (cc.AssignmentRef, error) {
	var a cc.AssignmentRef
	encoded := r.Header.Get("X-Suma-Assignment")
	if len(encoded) > 16384 {
		return a, invalid("Invalid candidate transfer assignment")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) > 8192 {
		return a, invalid("Invalid candidate transfer assignment")
	}
	if err = RejectDuplicateReleaseJSON(raw); err != nil {
		return a, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&a); err != nil || !a.Valid() {
		return a, invalid("Invalid candidate transfer assignment")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return a, invalid("Invalid candidate transfer assignment")
	}
	return a, nil
}
func candidateJSONRequest(r *http.Request) (map[string]any, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return nil, invalid("Invalid candidate transfer request")
	}
	if err = RejectDuplicateReleaseJSON(raw); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var m map[string]any
	if err = d.Decode(&m); err != nil || m == nil {
		return nil, invalid("Invalid candidate transfer request")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, invalid("Invalid candidate transfer request")
	}
	return m, nil
}
func candidateEnvelope(subject candidateSubject, project, tenant, runner string, a cc.AssignmentRef) error {
	if project != subject.project {
		return forbidden()
	}
	if subject.pool {
		if tenant != subject.tenant || tenant != a.TenantID || project != a.ProjectID || runner != a.PoolRunnerID {
			return forbidden()
		}
	} else if tenant != "" || runner != "" || candidateHasPoolFields(a) {
		return forbidden()
	}
	return nil
}

func (s *Server) candidateTransferHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {
	if path != "runner/candidate-transfers" && !strings.HasPrefix(path, "runner/candidate-transfers/") {
		return false
	}
	if actor.kind != actorProjectRunner || actor.credential.projectID != project || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		writeError(w, forbidden())
		return true
	}
	return s.candidateTransferRoute(w, r, projectCandidateSubject(actor, project), path, "runner/candidate-transfers", m)
}

// This entry point runs before human authentication and before the generic
// pool callback router, which intentionally only admits POST fact callbacks.
func (s *Server) poolCandidateTransferHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	const prefix = "pool-runner/candidate-transfers"
	if path != prefix && !strings.HasPrefix(path, prefix+"/") {
		return false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
		writeError(w, &APIError{401, "pool_runner_token_required", "资源池执行节点需要独立 Bearer 凭证", nil})
		return true
	}
	subject := candidateSubject{pool: true, token: strings.TrimPrefix(header, "Bearer ")}
	var m map[string]any
	if r.Method == http.MethodPost {
		var err error
		m, err = candidateJSONRequest(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		subject.tenant = str(m, "tenant_id")
		subject.project = str(m, "project_id")
	} else {
		a, err := candidateAssignmentHeader(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		subject.tenant = a.TenantID
		subject.project = a.ProjectID
		if subject.project == "" || r.URL.Query().Get("project_id") != subject.project {
			writeError(w, forbidden())
			return true
		}
	}
	return s.candidateTransferRoute(w, r, subject, path, prefix, m)
}

func (s *Server) candidateTransferRoute(w http.ResponseWriter, r *http.Request, subject candidateSubject, path, prefix string, m map[string]any) bool {
	ctx := r.Context()
	relative := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	if relative == "" && r.Method == http.MethodPost {
		var req cc.StartRequest
		if err := releaseDecode(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		if err := candidateEnvelope(subject, req.ProjectID, req.TenantID, req.RunnerID, req.Assignment); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.startCandidateTransfer(ctx, subject, req)
		respond(w, http.StatusCreated, out, err)
		return true
	}
	if relative == "/resolve" && r.Method == http.MethodPost {
		var req cc.CommandRequest
		if err := releaseDecode(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		if err := candidateEnvelope(subject, req.ProjectID, req.TenantID, req.RunnerID, req.Assignment); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.resolveCandidateTransfer(ctx, subject, req)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "seal" && r.Method == http.MethodPost {
		var req cc.CommandRequest
		if err := releaseDecode(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		if err := candidateEnvelope(subject, req.ProjectID, req.TenantID, req.RunnerID, req.Assignment); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.sealCandidateTransfer(ctx, subject, parts[0], req)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "bundle" && (r.Method == http.MethodPut || r.Method == http.MethodGet) {
		a, err := candidateAssignmentHeader(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		if subject.pool {
			if err = candidateEnvelope(subject, subject.project, a.TenantID, a.PoolRunnerID, a); err != nil {
				writeError(w, err)
				return true
			}
		} else if candidateHasPoolFields(a) {
			writeError(w, forbidden())
			return true
		}
		op := r.Header.Get("X-Suma-Operation-ID")
		if len(op) < 8 || len(op) > 128 {
			writeError(w, invalid("Invalid candidate transfer operation ID"))
			return true
		}
		if r.Method == http.MethodPut {
			err = s.Service.putCandidateBundle(ctx, subject, parts[0], a, op, r.ContentLength, r.Body)
			respond(w, http.StatusOK, map[string]bool{"acknowledged": err == nil}, err)
		} else {
			var b []byte
			b, err = s.Service.runnerCandidateBundle(ctx, subject, parts[0], a)
			if err != nil {
				writeError(w, err)
			} else {
				artifactBytes(w, b)
			}
		}
		return true
	}
	writeError(w, missing())
	return true
}
