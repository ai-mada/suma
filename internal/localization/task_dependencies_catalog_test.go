package localization

import "testing"

func TestTaskDependenciesCatalogExactPairedSources(t *testing.T) {
	for _, example := range append(append([]Definition{}, taskDependenciesCatalog...), taskDeliveryCatalog...) {
		key, found := LookupSource(example.SourceTemplate)
		if !found || key != example.Key {
			t.Fatalf("task dependency source lost its stable key: %q => %q", example.SourceTemplate, key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			out, err := WithSource(locale, key, nil, example.SourceTemplate)
			if err != nil || !out.Localized || out.Key != key || out.Text != example.Texts[locale] {
				t.Fatalf("task dependency source is not paired in %s: %+v, %v", locale, out, err)
			}
		}
		if _, matched := LookupSource(example.SourceTemplate + " user content"); matched {
			t.Fatal("task dependency diagnostic matched arbitrary trailing user content")
		}
	}
}
