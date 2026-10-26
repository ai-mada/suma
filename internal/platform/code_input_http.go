package platform

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

// The pool endpoint is routed before human authentication, with its own exact
// machine subject. Project runners use the normal current credential transport.
func (s *Server) poolCodeInputHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	if path != "pool-runner/code-inputs/bundle" {
		return false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
		writeError(w, unauthenticated())
		return true
	}
	return s.codeInputBundleRoute(w, r, candidateSubject{pool: true, token: strings.TrimPrefix(header, "Bearer ")}, nil)
}

func (s *Server) codeInputHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, body map[string]any) bool {
	if path != "runner/code-inputs/bundle" {
		return false
	}
	if actor.kind != actorProjectRunner || actor.credential.projectID != project || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		writeError(w, forbidden())
		return true
	}
	return s.codeInputBundleRoute(w, r, projectCandidateSubject(actor, project), body)
}

func (s *Server) codeInputBundleRoute(w http.ResponseWriter, r *http.Request, subject candidateSubject, body map[string]any) bool {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	var err error
	if body == nil {
		body, err = candidateJSONRequest(r)
		if err != nil {
			writeError(w, err)
			return true
		}
	}
	var req ccmp.CodeInputRequest
	if err = projectKnowledgeRequired(body, "project_id", "assignment", "input_kind", "binding_id", "transfer_id"); err != nil {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	if err = strictAutomation(body, &req); err != nil || ccmp.ValidateCodeInputRequest(req) != nil {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	if subject.pool {
		subject.tenant, subject.project = req.Assignment.TenantID, req.ProjectID
		if req.Assignment.ProjectID != req.ProjectID || subject.tenant == "" {
			writeError(w, forbidden())
			return true
		}
	} else if req.ProjectID != subject.project || candidateHasPoolFields(req.Assignment) {
		writeError(w, forbidden())
		return true
	}
	content, err := s.Service.readCodeInputBundle(r.Context(), subject, req)
	if err != nil {
		writeError(w, err)
		return true
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
	return true
}

func (s *Service) codeInputPreflight(ctx context.Context, subject candidateSubject, req ccmp.CodeInputRequest) (out candidateTransfer, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	authority, err := s.candidateAuthorityTx(ctx, tx, subject, req.Assignment)
	if err != nil {
		return out, err
	}
	out, err = s.codeInputBindingTx(ctx, tx, authority, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// Blob IO is outside SQL locks. Current authorization, exact consumer lease,
// frozen source membership and original bytes are checked again before release.
func (s *Service) readCodeInputBundle(ctx context.Context, subject candidateSubject, req ccmp.CodeInputRequest) ([]byte, error) {
	before, err := s.codeInputPreflight(ctx, subject, req)
	if err != nil {
		return nil, err
	}
	content, err := s.readCandidateBundle(before)
	if err != nil {
		return nil, err
	}
	after, err := s.codeInputPreflight(ctx, subject, req)
	if err != nil {
		return nil, err
	}
	if after.binding != before.binding || after.path != before.path {
		return nil, compositionError(409, "source_changed", compositionSource)
	}
	return content, nil
}
