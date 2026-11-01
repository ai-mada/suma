package localization

import "testing"

func TestTurnSteeringDiagnosticsUseExactBilingualSources(t *testing.T) {
	for _, d := range turnSteeringCatalog {
		key, ok := LookupSource(d.SourceTemplate)
		if !ok || key != d.Key {
			t.Fatal("missing exact diagnostic", d.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			v, err := WithSource(locale, key, nil, d.SourceTemplate)
			if err != nil || !v.Localized || v.Text != d.Texts[locale] {
				t.Fatal("untranslated diagnostic", key, locale)
			}
		}
		if _, ok := LookupSource(d.SourceTemplate + " /original-user-content"); ok {
			t.Fatal("private source matched diagnostic prefix")
		}
	}
}
