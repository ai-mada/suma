package platform

import (
	"net/http"

	"github.com/ai-mada/suma/internal/localization"
)

type LocalizationParameter struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type LocalizationMessage struct {
	Key        string                  `json:"key"`
	Parameters []LocalizationParameter `json:"parameters"`
	Texts      map[string]string       `json:"texts"`
}
type LocalizationCatalog struct {
	Version          string                `json:"version"`
	TemplateFormat   string                `json:"template_format"`
	DefaultLocale    string                `json:"default_locale"`
	SupportedLocales []string              `json:"supported_locales"`
	Messages         []LocalizationMessage `json:"messages"`
}

// The public catalog contains only compiled product messages: no user input,
// tenant records, configured paths, credentials or original tool output. Both
// languages are returned so existing errors can switch without rerunning writes.
func platformLocalizationCatalog() LocalizationCatalog {
	out := LocalizationCatalog{TemplateFormat: "named-dot-v1", DefaultLocale: string(localization.DefaultLocale), SupportedLocales: []string{string(localization.ZhCN), string(localization.EnUS)}, Messages: []LocalizationMessage{}}
	for _, definition := range localization.Catalog() {
		message := LocalizationMessage{Key: definition.Key, Parameters: []LocalizationParameter{}, Texts: map[string]string{}}
		for _, parameter := range definition.Parameters {
			message.Parameters = append(message.Parameters, LocalizationParameter{Name: parameter.Name, Kind: string(parameter.Kind)})
		}
		for locale, text := range definition.Texts {
			message.Texts[string(locale)] = text
		}
		out.Messages = append(out.Messages, message)
	}
	// Bind interpretation and fallback rules as well as message bytes. A later
	// template-format change must not reuse a client's cached catalog version.
	out.Version = digest(stringify(map[string]any{"template_format": out.TemplateFormat, "default_locale": out.DefaultLocale, "supported_locales": out.SupportedLocales, "messages": out.Messages}))
	return out
}

func localizationCatalogHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	if path != "localization/catalog" {
		return false
	}
	if r.Method != http.MethodGet {
		writeError(w, &APIError{405, "method_not_allowed", "消息目录仅允许读取", nil})
		return true
	}
	writeJSON(w, 200, platformLocalizationCatalog())
	return true
}
