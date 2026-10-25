package platform

import (
	"net/http"
	"strings"
)

// BrowserCommandRequiresView distinguishes commands from the explicitly implemented POST
// queries. Unknown writes require a view; a new route cannot gain a read-only
// exception just by adopting a name such as "preview".
func BrowserCommandRequiresView(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	if method == http.MethodPost {
		switch path {
		case "auth/setup", "auth/login", "auth/invitations/accept", "auth/logout",
			"search", "workflows/validate", "agent/tools/invoke", "automations/preview", "release-plans/preview":
			return false
		}
		parts := strings.Split(path, "/")
		if len(parts) == 3 && parts[0] == "deployments" && parts[1] != "" && parts[2] == "rollback-preview" {
			return false
		}
	}
	return true
}

// validateOriginalView compares display context, not credential authority. The
// Actor still comes from the request secret and is rechecked in the command's
// transaction. In particular, these headers never select a tenant or user.
func validateOriginalView(r *http.Request, actual Principal, accountOnly bool) error {
	users := r.Header.Values("X-Suma-Expected-User")
	organizations := r.Header.Values("X-Suma-Expected-Organization")
	if len(users) != 1 || !localePreferenceScopeID.MatchString(users[0]) ||
		(!accountOnly && (len(organizations) != 1 || !localePreferenceScopeID.MatchString(organizations[0]))) ||
		(accountOnly && len(organizations) != 0) {
		return &APIError{http.StatusPreconditionRequired, "view_context_required", "需要原始账户与组织上下文，请刷新页面后重试", nil}
	}
	if users[0] != actual.ID || !accountOnly && organizations[0] != actual.TenantID {
		return &APIError{http.StatusForbidden, "view_context_changed", "账户或组织已变化，请核对原请求后重试", nil}
	}
	return nil
}
