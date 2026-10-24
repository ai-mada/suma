package localization

import (
	"strings"
	"testing"
)

func TestNormalizeCanonicalLocalesWithoutPretendingTraditionalSupport(t *testing.T) {
	for _, v := range []string{"zh", "zh-CN", "zh-Hans", "zh_hans_CN", "ZH-cn", "zh-SG"} {
		if got, ok := Normalize(v); !ok || got != ZhCN {
			t.Errorf("%q => %q %v", v, got, ok)
		}
	}
	for _, v := range []string{"en", "en-US", "en-GB", "en_US"} {
		if got, ok := Normalize(v); !ok || got != EnUS {
			t.Errorf("%q => %q %v", v, got, ok)
		}
	}
	for _, v := range []string{"", "zh-Hant", "zh-TW", "zh-HK", "fr-FR", "en-Cyrl", "*", "../en", "en\nUS", strings.Repeat("x", 256)} {
		if _, ok := Normalize(v); ok {
			t.Errorf("unsupported locale accepted: %q", v)
		}
	}
}
func TestResolveUsesExplicitUserOrganizationRequestAndDefault(t *testing.T) {
	cases := []struct {
		p    Preferences
		want Selection
	}{{Preferences{Explicit: "en", User: "zh", Organization: "zh"}, Selection{EnUS, SourceExplicit}}, {Preferences{Explicit: "unknown", User: "en", Organization: "zh"}, Selection{EnUS, SourceUser}}, {Preferences{Organization: "en", AcceptLanguage: "zh"}, Selection{EnUS, SourceOrganization}}, {Preferences{Organization: "unknown", AcceptLanguage: "en-US,en;q=0.8"}, Selection{EnUS, SourceRequest}}, {Preferences{AcceptLanguage: "fr"}, Selection{ZhCN, SourceDefault}}}
	for _, c := range cases {
		if got := Resolve(c.p); got != c.want {
			t.Errorf("selection=%+v want %+v", got, c.want)
		}
	}
}
func TestAcceptLanguageWeightsExclusionsAndBoundedFailure(t *testing.T) {
	for _, c := range []struct {
		header string
		locale Locale
		ok     bool
	}{{"en;q=0.7,zh-CN;q=0.9", ZhCN, true}, {"fr;q=1,en;q=0.8", EnUS, true}, {"zh-Hant,en;q=0.8", EnUS, true}, {"en;q=0,*;q=1", ZhCN, true}, {"zh;q=0,*;q=1", EnUS, true}, {"zh;q=0,en;q=0,*;q=1", "", false}, {"en-US;q=0.2,en;q=0.9,zh;q=0.8", ZhCN, true}, {"en;q=2", "", false}, {"en;q=NaN", "", false}, {"en;q=-1", "", false}, {"en;q=0.1234", "", false}, {"en;q=1,en;q=0", "", false}, {strings.Repeat("en,", 100), "", false}} {
		if got, ok := NegotiateAcceptLanguage(c.header); got != c.locale || ok != c.ok {
			t.Errorf("%q => %q %v want %q %v", c.header, got, ok, c.locale, c.ok)
		}
	}
}
func TestEnvironmentLocaleReadsPOSIXPrecedenceWithoutChangingEnvironment(t *testing.T) {
	env := map[string]string{"LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "zh_CN.UTF-8", "LANG": "zh_CN"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if got, ok := EnvironmentLocale(lookup); got != EnUS || !ok {
		t.Fatal(got, ok)
	}
	env["LC_ALL"] = "C.UTF-8"
	if _, ok := EnvironmentLocale(lookup); ok {
		t.Fatal("neutral C locale pretended to be a supported manual preference")
	}
	if env["LC_MESSAGES"] != "zh_CN.UTF-8" {
		t.Fatal("locale lookup mutated environment")
	}
}

func TestLanguageHeaderRejectsControlCharactersBeforeWhitespaceNormalization(t *testing.T) {
	for _, header := range []string{"en\n", "\rzh", "en\x00", "zh;\tq=1"} {
		if _, ok := NegotiateAcceptLanguage(header); ok {
			t.Fatalf("control character accepted: %q", header)
		}
	}
}
