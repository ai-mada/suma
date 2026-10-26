package platform

import (
	"github.com/ai-mada/suma/internal/localization"
	"testing"
)

func TestApplicationToolDescriptionsHaveStablePairedCatalogues(t *testing.T) {
	for _, definition := range DeclaredApplicationTools() {
		key, found := localization.LookupSource(definition.Description)
		if !found || key != definition.DescriptionKey {
			t.Fatalf("description identity missing: %s", definition.Name)
		}
		for _, locale := range []localization.Locale{localization.ZhCN, localization.EnUS} {
			out, err := localization.Format(locale, key, nil)
			if err != nil || !out.Localized || out.Text == "" {
				t.Fatalf("description %s / %s: %v", definition.Name, locale, err)
			}
		}
	}
}
