package localization

import "testing"

func TestWorkspaceFileContextCatalogExactSources(t *testing.T) {
	for _, example := range workspaceFileContextCatalog {
		key, found := LookupSource(example.SourceTemplate)
		if !found || key != example.Key {
			t.Fatalf("workspace context source lost stable key: %q => %q", example.SourceTemplate, key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			out, err := WithSource(locale, key, nil, example.SourceTemplate)
			if err != nil || !out.Localized || out.Key != key || out.Text != example.Texts[locale] {
				t.Fatalf("workspace context source not paired in %s: %+v, %v", locale, out, err)
			}
		}
		if _, matched := LookupSource(example.SourceTemplate + " user content"); matched {
			t.Fatal("workspace context diagnostic matched a partial source")
		}
	}
}
