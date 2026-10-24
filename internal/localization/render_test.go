package localization

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestCatalogKnownMessagesAreBilingualAndKeepMachineFields(t *testing.T) {
	en, err := Format(EnUS, "common.forbidden", nil)
	if err != nil || en.Text != "Your current identity does not have permission for this operation." || !en.Localized || en.Key != "common.forbidden" {
		t.Fatal(en, err)
	}
	zh, err := Format(ZhCN, "common.forbidden", nil)
	if err != nil || zh.Text != "当前身份没有此操作权限" {
		t.Fatal(zh, err)
	}
	key, ok := LookupSource("当前身份没有此操作权限")
	if !ok || key != en.Key {
		t.Fatal("exact source lookup absent")
	}
	if _, ok = LookupSource("prefix: 当前身份没有此操作权限"); ok {
		t.Fatal("source lookup guessed a user/source substring")
	}
	if key, ok := LookupCode("invalid_input"); !ok || key != "common.invalid_input" {
		t.Fatal("generic empty-source code bridge absent")
	}
}
func TestTypedParametersKeepExplicitZeroAndRejectUnknownOrUnsafeValues(t *testing.T) {
	params := Params{"field": "project_id", "limit": int64(0)}
	got, err := Format(EnUS, "validation.max_length", params)
	if err != nil || !strings.Contains(got.Text, "project_id") || !strings.Contains(got.Text, "0") || got.Params["limit"] != int64(0) {
		t.Fatal(got, err)
	}
	for _, params := range []Params{{"field": "project_id"}, {"field": "project_id", "limit": nil}, {"field": "project_id", "limit": 1.5}, {"field": "project_id", "limit": -1}, {"field": "project_id", "limit": json.Number("9007199254740992")}, {"field": "project_id", "limit": json.Number("1.0")}, {"field": "project_id", "limit": 1, "token": "DO_NOT_ECHO_VALUE"}, {"field": "project_id\n", "limit": 1}} {
		if _, err := Format(EnUS, "validation.max_length", params); !errors.Is(err, ErrInvalidParameters) {
			t.Errorf("unsafe/missing params accepted: err=%v", err)
		} else if strings.Contains(err.Error(), "DO_NOT_ECHO_VALUE") {
			t.Fatal("error leaked parameter value")
		}
	}
}
func TestUnknownKeysAndSourceFallbackDoNotTranslateDiagnostics(t *testing.T) {
	got, err := Format(EnUS, "future.new_message", Params{"token": "NEVER_LEAK"})
	if err != nil || got.Key != "future.new_message" || got.Localized || got.Fallback != "unknown_key" || strings.Contains(got.Text, "NEVER_LEAK") {
		t.Fatal(got, err)
	}
	const source = "具体校验：名称不能包含路径；tool_status=waiting_input"
	got, err = WithSource(EnUS, "future.new_message", nil, source)
	if err != nil || got.Text != source || got.Localized || got.Fallback != "source_untranslated" {
		t.Fatal(got, err)
	}
	got, err = WithSource(EnUS, "validation.max_length", Params{"field": "project_id"}, source)
	if !errors.Is(err, ErrInvalidParameters) || got.Text != source || got.Localized {
		t.Fatal(got, err)
	}
	if _, err = Format(EnUS, "bad\nkey", nil); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("invalid key accepted")
	}
}
func TestCatalogCopiesAndConcurrentLocalesAreIndependent(t *testing.T) {
	defs := Catalog()
	if len(defs) == 0 {
		t.Fatal("empty catalog")
	}
	for _, d := range defs {
		if d.Texts[ZhCN] == "" || d.Texts[EnUS] == "" {
			t.Fatal("incomplete translation", d.Key)
		}
		for _, loc := range []Locale{ZhCN, EnUS} {
			params := Params{}
			for _, p := range d.Parameters {
				switch p.Kind {
				case Integer:
					params[p.Name] = int64(1)
				default:
					params[p.Name] = "fixture"
				}
			}
			if _, err := Format(loc, d.Key, params); err != nil {
				t.Fatal(d.Key, err)
			}
		}
	}
	defs[0].Texts[ZhCN] = "CORRUPTED"
	if Catalog()[0].Texts[ZhCN] == "CORRUPTED" {
		t.Fatal("caller mutated shared catalog")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			loc := ZhCN
			if i%2 == 0 {
				loc = EnUS
			}
			m, e := Format(loc, "common.forbidden", nil)
			if e != nil || m.Locale != loc {
				t.Error("shared current locale leaked between calls")
			}
		}(i)
	}
	wg.Wait()
}
