package localization

import "testing"

func TestTestPlansControlledDiagnosticsArePairedWithoutTranslatingOriginals(t *testing.T) {
	for _, definition := range testPlanCatalog {
		for _, source := range []string{definition.SourceTemplate, definition.Texts[ZhCN]} {
			key, found := LookupSource(source)
			if !found || key != definition.Key {
				t.Fatal("exact controlled producer alias missing", source)
			}
			for _, locale := range []Locale{ZhCN, EnUS} {
				value, err := WithSource(locale, key, nil, source)
				if err != nil || !value.Localized || value.Text != definition.Texts[locale] {
					t.Fatal("controlled diagnostic is not paired", locale, value, err)
				}
			}
		}
		if _, matched := LookupSource(definition.SourceTemplate + " — literal model or human context"); matched {
			t.Fatal("controlled diagnostic prefix consumed an original message")
		}
	}
	original := `<failure message="Invalid test plan request">Human diagnostic {{.private}}</failure>`
	if _, matched := LookupSource(original); matched {
		t.Fatal("raw report XML was guessed to be a host diagnostic")
	}
	value, err := WithSource(ZhCN, "test_plan.unregistered_original", nil, original)
	if err != nil || value.Localized || value.Text != original {
		t.Fatal("unregistered report/human original changed", value, err)
	}
}
