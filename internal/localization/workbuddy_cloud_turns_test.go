package localization

import "testing"

func TestCloudTurnErrorSourcesRetainExactBilingualIdentity(t *testing.T) {
	for _, d := range workBuddyCloudTurnsCatalog {
		key, ok := LookupSource(d.SourceTemplate)
		if !ok || key != d.Key {
			t.Fatal("unregistered turn error", d.Key)
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			out, err := Format(locale, key, nil)
			if err != nil || !out.Localized || out.Text == "" || out.Key != key {
				t.Fatal("missing turn translation", key, locale, err)
			}
		}
		if _, ok := LookupSource("provider: " + d.SourceTemplate); ok {
			t.Fatal("unreviewed provider error became trusted")
		}
	}
}
