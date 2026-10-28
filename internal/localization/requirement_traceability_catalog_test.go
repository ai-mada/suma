package localization

import "testing"

func TestRequirementTraceabilityCatalogExactSource(t *testing.T) {
	for _, example := range requirementTraceabilityCatalog {
		key, found := LookupSource(example.SourceTemplate)
		if !found || key != example.Key {
			t.Fatal("traceability diagnostic lost its exact semantic key")
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			out, err := WithSource(locale, key, nil, example.SourceTemplate)
			if err != nil || !out.Localized || out.Text != example.Texts[locale] {
				t.Fatalf("traceability diagnostic is not localized in %s", locale)
			}
		}
		if _, matched := LookupSource(example.SourceTemplate + " user content"); matched {
			t.Fatal("a diagnostic prefix matched original user prose")
		}
	}
}
