package platform

import (
	"context"
	"github.com/ai-mada/suma/internal/toolcontract"
	"net/http"
	"net/url"
)

type AdapterCatalogPage struct {
	ProjectID string                           `json:"project_id"`
	AsOf      string                           `json:"as_of"`
	Items     []toolcontract.AdapterDescriptor `json:"items"`
}

const adapterCatalogInvalid = "Adapter catalog request is invalid"

func (s *Service) ListAdapterCatalog(ctx context.Context, actor Actor, project string) (out AdapterCatalogPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) {
		return out, invalid(adapterCatalogInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(a *authorizedCommand) error {
		out = AdapterCatalogPage{ProjectID: project, AsOf: now(), Items: toolcontract.AdapterCatalog()}
		return nil
	})
	return
}
func (s *Server) adapterCatalogHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &APIError{http.StatusMethodNotAllowed, "method_not_allowed", "不支持此请求方法", nil})
		return
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || len(r.URL.RawQuery) > 4096 || len(q) != 1 || len(q["project_id"]) != 1 || q.Get("project_id") != project || !handoffID(project, false) {
		writeError(w, invalid(adapterCatalogInvalid))
		return
	}
	out, e := s.Service.ListAdapterCatalog(r.Context(), actor, project)
	respond(w, http.StatusOK, out, e)
}
