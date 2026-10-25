package platform

import (
	"context"
	"net/http"
)

// projectReadJSON binds the complete response query to one current-authority
// snapshot. Callbacks use proof.tx; network/filesystem work belongs outside it.
func (s *Server) projectReadJSON(w http.ResponseWriter, r *http.Request, actor Actor, project string, read func(context.Context, *authorizedCommand) (any, error)) {
	var body any
	err := s.Service.withCurrentRead(r.Context(), actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		ctx, cancel := context.WithDeadline(r.Context(), auth.deadline)
		defer cancel()
		var err error
		body, err = read(ctx, auth)
		return err
	})
	respond(w, http.StatusOK, body, err)
}

func readListEnvelope[T any](items []T) map[string]any {
	if items == nil {
		items = []T{}
	}
	return map[string]any{"items": items, "total": len(items)}
}
