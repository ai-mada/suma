package localization

import "testing"

func TestRunnerPairingDiagnosticsResolveExactOwnedSourcesInBothLanguages(t *testing.T) {
	for _, definition := range runnerPairingCatalog {
		key, ok := LookupSource(definition.SourceTemplate)
		if !ok || key != definition.Key {
			t.Fatalf("missing pairing source binding: %s", definition.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			value, err := WithSource(locale, key, nil, definition.SourceTemplate)
			if err != nil || !value.Localized || value.Text != definition.Texts[locale] {
				t.Fatalf("pairing diagnostic must preserve its precise meaning: %s %s", locale, key)
			}
		}
		if _, ok := LookupSource(definition.SourceTemplate + " /machine-original-content"); ok {
			t.Fatal("pairing source binding must not absorb machine/user content")
		}
	}
}
