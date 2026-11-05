package repositoryprovider

import (
	"context"
	"net/http"
	"strings"
	"time"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

const sourceExportBasisPath = "/source-export-basis/"

func (s *Server) sourceExportBasisHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !token(r, s.cfg.ReconcileTokenEnv) {
		writeError(w, http.StatusForbidden, "repository_source_export_forbidden")
		return
	}
	if r.Method != http.MethodGet || r.URL.ForceQuery || r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "repository_source_export_invalid")
		return
	}
	alias := strings.TrimPrefix(r.URL.Path, sourceExportBasisPath)
	target := s.targets[alias]
	if target == nil || target.config.DeliveryBaseAlias == "" {
		writeError(w, http.StatusNotFound, "repository_source_export_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	// Match target observation locking so the two reads do not straddle a
	// provider-managed apply. Binary export rechecks the exact requested pair.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "repository_source_export_unavailable")
		return
	}
	source, err := s.Target(ctx, alias)
	if err != nil {
		writeError(w, http.StatusConflict, "repository_source_export_unavailable")
		return
	}
	baseline, err := s.Target(ctx, target.config.DeliveryBaseAlias)
	basis := ri.RepositorySourceExportBasis{Source: source, Baseline: baseline}
	if err != nil || ri.ValidateRepositorySourceExportBasis(alias, basis) != nil {
		writeError(w, http.StatusConflict, "repository_source_export_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, basis)
}
