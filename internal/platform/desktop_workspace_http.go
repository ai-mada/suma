package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func (s *Server) workspaceDesktopBindingHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, body map[string]any) bool {
	if path != "workspace/desktop-binding" {
		return false
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "不支持此请求方法", nil})
		return true
	}
	// The shared ingress already decoded the body and authenticates the view.
	// Reject unknown fields here, including nested grant fields. This does not
	// claim to recover duplicate JSON keys already consumed by that ingress.
	if len(body) != 4 {
		writeError(w, desktopWorkspaceInvalid())
		return true
	}
	for _, key := range []string{"project_id", "expected_binding_sha256", "idempotency_key"} {
		if _, ok := body[key].(string); !ok {
			writeError(w, desktopWorkspaceInvalid())
			return true
		}
	}
	grant, ok := body["grant"].(map[string]any)
	if !ok || len(grant) != 9 {
		writeError(w, desktopWorkspaceInvalid())
		return true
	}
	for _, key := range []string{"protocol", "operation_id", "user_id", "tenant_id", "project_id", "path", "expires_at", "nonce", "signature"} {
		if _, ok = grant[key].(string); !ok {
			writeError(w, desktopWorkspaceInvalid())
			return true
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		writeError(w, desktopWorkspaceInvalid())
		return true
	}
	var request WorkspaceDesktopBindingRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		writeError(w, desktopWorkspaceInvalid())
		return true
	}
	out, err := s.Service.BindDesktopWorkspace(r.Context(), actor, project, request)
	respond(w, http.StatusOK, out, err)
	return true
}
