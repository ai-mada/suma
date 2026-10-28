package localization

import "testing"

func TestIntelligentOperationsExactBilingualDiagnosticsPreserveOriginalText(t *testing.T) {
	for _, diagnostic := range intelligentOperationsCatalog {
		for _, source := range []string{diagnostic.SourceTemplate, diagnostic.Texts[ZhCN]} {
			key, found := LookupSource(source)
			if !found || key != diagnostic.Key {
				t.Fatalf("controlled diagnostic has no exact alias: %q", source)
			}
			for _, locale := range []Locale{ZhCN, EnUS} {
				out, err := WithSource(locale, key, nil, source)
				if err != nil || !out.Localized || out.Text != diagnostic.Texts[locale] {
					t.Fatalf("diagnostic is not paired in %s: %+v %v", locale, out, err)
				}
			}
		}
		if _, matched := LookupSource(diagnostic.SourceTemplate + " model or human original {{.secret}}"); matched {
			t.Fatal("diagnostic prefix consumed original prose")
		}
	}
	original := "Human review: The intelligent operation source has changed; create a new analysis {{.private_text}}"
	if _, matched := LookupSource(original); matched {
		t.Fatal("human/model text was guessed to be a controlled diagnostic")
	}
	out, err := WithSource(ZhCN, "intelligent_operations.unregistered_original", nil, original)
	if err != nil || out.Localized || out.Text != original {
		t.Fatal("unregistered original text was modified", out, err)
	}
}
