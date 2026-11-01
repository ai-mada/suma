package localization

import "testing"

func TestDevelopmentContinuationDiagnosticsUseExactBilingualSources(t *testing.T) {
	for _, definition := range developmentContinuationCatalog {
		key, ok := LookupSource(definition.SourceTemplate)
		if !ok || key != definition.Key {
			t.Fatal("missing exact continuation diagnostic binding", definition.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			got, err := WithSource(locale, key, nil, definition.SourceTemplate)
			if err != nil || !got.Localized || got.Text != definition.Texts[locale] {
				t.Fatal("continuation diagnostic fallback", locale, key)
			}
		}
		if _, ok := LookupSource(definition.SourceTemplate + " /original-user-file"); ok {
			t.Fatal("machine/user content must not match a diagnostic prefix")
		}
	}
}
