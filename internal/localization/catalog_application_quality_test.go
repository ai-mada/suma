package localization

import "testing"

func TestApplicationQualityDescriptionsAndCursorDiagnosticsPreserveOriginals(t *testing.T) {
	for _, definition := range applicationQualityCatalog {
		sources := []string{definition.Texts[ZhCN]}
		if definition.SourceTemplate != "" {
			sources = append(sources, definition.SourceTemplate)
		}
		for _, source := range sources {
			key, matched := LookupSource(source)
			if !matched || key != definition.Key {
				t.Fatal("controlled quality source was not registered", source)
			}
			for _, locale := range []Locale{ZhCN, EnUS} {
				value, err := WithSource(locale, key, nil, source)
				if err != nil || !value.Localized || value.Text != definition.Texts[locale] {
					t.Fatal("missing paired quality description", locale, value, err)
				}
			}
		}
	}
	original := "Original failed case: The case cursor does not belong to the selected run's frozen test plan"
	if _, matched := LookupSource(original); matched {
		t.Fatal("original case content was treated as a host diagnostic")
	}
}
