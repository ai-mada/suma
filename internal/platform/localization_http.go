package platform

import (
	"net/http"

	"github.com/ai-mada/suma/internal/localization"
)

// Request language is a presentation preference, never a permission, tenant
// switch or input to persisted business/receipt hashes. Account and organization
// defaults are intentionally not synthesized before those settings exist.
func setResponseLanguage(w http.ResponseWriter, r *http.Request) {
	selection := localization.Resolve(localization.Preferences{Explicit: r.Header.Get("X-Suma-Language"), AcceptLanguage: r.Header.Get("Accept-Language")})
	w.Header().Set("Content-Language", string(selection.Locale))
	w.Header().Add("Vary", "Accept-Language, X-Suma-Language")
}

// Exact source lookup prevents a general invalid_input translation from hiding
// the useful specific validation diagnostic. Legacy gaps remain visible and
// machine-readable; adding a catalog entry needs no change to stored evidence.
func localizedAPIError(w http.ResponseWriter, e *APIError) map[string]any {
	locale, ok := localization.Normalize(w.Header().Get("Content-Language"))
	if !ok {
		locale = localization.DefaultLocale
	}
	key, found := localization.LookupSource(e.Message)
	if !found && e.Message == "" {
		key, found = localization.LookupCode(e.Code)
	}
	if !found {
		key = "legacy." + e.Code
	}
	text, _ := localization.WithSource(locale, key, nil, e.Message)
	message := text.Text
	if message == "" {
		message = e.Message
	}
	out := map[string]any{"code": e.Code, "message": message, "details": e.Details, "message_key": text.Key, "locale": string(locale), "localized": text.Localized}
	if len(text.Params) > 0 {
		out["message_params"] = text.Params
	}
	if text.Fallback != "" {
		out["fallback"] = text.Fallback
	}
	return out
}
