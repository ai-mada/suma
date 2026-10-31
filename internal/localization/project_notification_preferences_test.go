package localization

import "testing"

func TestProjectNotificationPreferencesExactSourcesRemainBilingual(t *testing.T) {
	for _, source := range []string{"Invalid project notification preferences request", "The project notification preferences changed; reload before deciding"} {
		key, ok := LookupSource(source)
		if !ok {
			t.Fatal("reviewed preference source is not registered", source)
		}
		zh, err := Format(ZhCN, key, nil)
		if err != nil || !zh.Localized || zh.Text == source {
			t.Fatal("preference error has no Chinese presentation", key, err)
		}
		en, err := Format(EnUS, key, nil)
		if err != nil || !en.Localized || en.Text == "" || en.Key != zh.Key {
			t.Fatal("preference error lost stable identity", key, err)
		}
		if _, ok = LookupSource("untrusted: " + source); ok {
			t.Fatal("preference localization expanded source authority")
		}
	}
}
