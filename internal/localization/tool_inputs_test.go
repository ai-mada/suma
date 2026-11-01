package localization

import "testing"

func TestToolInputDiagnosticsUseExactBilingualSources(t *testing.T) {
	for _, d := range toolInputsCatalog {
		key, ok := LookupSource(d.SourceTemplate)
		if !ok || key != d.Key {
			t.Fatal("missing diagnostic", d.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			v, err := WithSource(locale, key, nil, d.SourceTemplate)
			if err != nil || !v.Localized || v.Text != d.Texts[locale] {
				t.Fatal("untranslated diagnostic", key, locale)
			}
		}
		if _, ok := LookupSource(d.SourceTemplate + " /original-user-content"); ok {
			t.Fatal("source alias matched private content")
		}
	}
}
