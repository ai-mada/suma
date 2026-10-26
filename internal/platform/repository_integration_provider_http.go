package platform

import (
	"net/http"
	"strings"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// This transport has no human authentication fallback. Each operation consumes
// the configured provider's independent capability and its exact persisted
// assignment; a cookie or a project API token cannot supply provider facts.
func (s *Server) repositoryProviderHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	if !strings.HasPrefix(path, "repository-provider/") {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid))
		return true
	}
	token, ctx := strings.TrimPrefix(header, "Bearer "), r.Context()
	decode := func(out any) bool {
		if err := releaseProviderBody(r, out); err != nil {
			writeError(w, err)
			return false
		}
		return true
	}
	switch path {
	case "repository-provider/consume":
		var req ri.RepositoryConsumeAuthorization
		if !decode(&req) {
			return true
		}
		out, err := s.Service.ConsumeRepositoryAuthorization(ctx, token, req)
		respond(w, 200, out, err)
	case "repository-provider/inspect-authorization":
		var req ri.RepositoryInspectAuthorization
		if !decode(&req) {
			return true
		}
		out, err := s.Service.InspectRepositoryAuthorization(ctx, token, req)
		respond(w, 200, out, err)
	case "repository-provider/begin-effect":
		var req ri.RepositoryBeginEffect
		if !decode(&req) {
			return true
		}
		out, err := s.Service.BeginRepositoryEffect(ctx, token, req)
		respond(w, 200, out, err)
	case "repository-provider/bundle":
		var req ri.RepositoryBundleRequest
		if !decode(&req) {
			return true
		}
		out, err := s.Service.ProviderRepositoryBundle(ctx, token, req)
		if err != nil {
			writeError(w, err)
			return true
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	case "repository-provider/observations":
		var req ri.RepositoryObservation
		if !decode(&req) {
			return true
		}
		out, err := s.Service.RecordRepositoryObservation(ctx, token, req)
		respond(w, 200, out, err)
	case "repository-provider/consume-reconciliation":
		var req ri.RepositoryConsumeReconciliation
		if !decode(&req) {
			return true
		}
		out, err := s.Service.ConsumeRepositoryReconciliation(ctx, token, req)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
