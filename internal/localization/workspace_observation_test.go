package localization

import "testing"

func TestWorkspaceObservationExactBilingualDiagnostics(t *testing.T) {
	for _, definition := range workspaceObservationCatalog {
		key, ok := LookupSource(definition.SourceTemplate)
		if !ok || key != definition.Key {
			t.Fatal("missing exact alias", definition.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			rendered, err := WithSource(locale, key, nil, definition.SourceTemplate)
			if err != nil || !rendered.Localized || rendered.Text != definition.Texts[locale] {
				t.Fatal(locale, key, rendered, err)
			}
		}
		if _, ok = LookupSource(definition.SourceTemplate + " /private/user-path"); ok {
			t.Fatal("matched user content")
		}
	}
}
