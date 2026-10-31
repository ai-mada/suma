package localization

import "testing"

func TestWorkBuddyTaskDispatchErrorsKeepReviewedBilingualIdentity(t *testing.T) {
	for _, d := range workBuddyTaskDispatchCatalog {
		key, ok := LookupSource(d.SourceTemplate)
		if !ok || key != d.Key {
			t.Fatal("missing exact reviewed source", d.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			out, err := Format(locale, key, nil)
			if err != nil || !out.Localized || out.Text == "" || out.Key != key {
				t.Fatal("missing bilingual domain message", d.Key, locale, err)
			}
		}
		if _, ok := LookupSource("provider: " + d.SourceTemplate); ok {
			t.Fatal("unreviewed source was authorized")
		}
	}
}
