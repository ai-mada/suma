package localization

import "testing"

func TestDisplayCatalogExplicitSourceAndNoReverseAlias(t *testing.T) {
	count := 0
	for _, d := range Catalog() {
		if len(d.Key) > 13 && d.Key[:13] == "display.tool." {
			count++
			if d.SourceTemplate == "" {
				t.Error("missing canonical source", d.Key)
			}
			if k, ok := LookupSource(d.Texts[ZhCN]); ok && k == d.Key {
				t.Error("display text became global source alias", d.Key)
			}
			for _, locale := range []Locale{ZhCN, EnUS} {
				params := Params(nil)
				if d.Key == "display.tool.reported_unavailable" {
					params = Params{"reason": "external {{.reason}}"}
				}
				r, e := Format(locale, d.Key, params)
				if e != nil || !r.Localized {
					t.Errorf("missing display catalog %s/%s: %v", d.Key, locale, e)
				}
			}
		}
	}
	if count != 16 {
		t.Fatalf("got %d display definitions, want 16", count)
	}
	if k, ok := LookupSource("资源池已暂停，停止新派发"); !ok || k != "pool.paused" {
		t.Fatal("existing pool source changed", k, ok)
	}
}
