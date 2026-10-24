package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLILanguageHelpAndExplicitPreference(t *testing.T) {
	t.Setenv("SUMA_LANGUAGE", "en")
	for _, tc := range []struct {
		args             []string
		contains, absent string
	}{
		{[]string{"--language", "zh-Hans", "--help"}, "全局参数", "Global flags"},
		{[]string{"--help"}, "Global flags", "全局参数"},
	} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), tc.args, strings.NewReader(""), &out, &stderr); code != 0 || !strings.Contains(out.String(), tc.contains) || strings.Contains(out.String(), tc.absent) {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
}

// False is an explicit server fact; absent presentation metadata is different.
// The CLI must preserve both without changing diagnostics or machine fields.
func TestCLIPreservesRemoteLocalizedPresence(t *testing.T) {
	for _, tc := range []struct {
		name           string
		present, value bool
	}{{"explicit_false", true, false}, {"explicit_true", true, true}, {"absent", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			const diagnostic = "服务端具体原文 waiting_input"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err := map[string]any{"code": "forbidden", "message": diagnostic, "message_key": "legacy.forbidden", "locale": "en-US", "details": map[string]any{"state": "waiting_input", "version": 2}, "fallback": "source_untranslated"}
				if tc.present {
					err["localized"] = tc.value
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(403)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": err})
			}))
			defer server.Close()
			var out, stderr bytes.Buffer
			code := Execute(context.Background(), []string{"--language", "en-US", "--control", server.URL, "--project", "p", "tasks"}, strings.NewReader(""), &out, &stderr)
			var body struct {
				Error map[string]any `json:"error"`
			}
			if json.Unmarshal(stderr.Bytes(), &body) != nil || code != 1 || body.Error["message"] != diagnostic || body.Error["code"] != "forbidden" || body.Error["fallback"] != "source_untranslated" {
				t.Fatal(code, stderr.String())
			}
			value, present := body.Error["localized"]
			if present != tc.present || tc.present && value != tc.value {
				t.Fatalf("localized presence/value changed: present=%t value=%v", present, value)
			}
			details, ok := body.Error["details"].(map[string]any)
			if !ok || details["state"] != "waiting_input" || details["version"] != float64(2) {
				t.Fatal("machine details changed", body.Error)
			}
		})
	}
}

func TestCLIRequestLanguageDoesNotTranslateServerDiagnosticsOrMachineFields(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Accept-Language") != "en-US" {
			t.Errorf("wrong request language %q", r.Header.Get("Accept-Language"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"具体诊断，waiting_input 保留","message_key":"legacy.forbidden","locale":"en-US","localized":false,"fallback":"source_untranslated"}}`))
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--language", "en", "--control", server.URL, "--project", "p", "tasks"}, strings.NewReader(""), &out, &stderr)
	var body struct {
		Error failure `json:"error"`
	}
	if json.Unmarshal(stderr.Bytes(), &body) != nil || code != 1 || requests != 1 || body.Error.Code != "forbidden" || body.Error.Message != "具体诊断，waiting_input 保留" {
		t.Fatal(code, stderr.String())
	}
}

func TestCLIPOSIXLanguageAndLocalErrorRemainSeparateFromServerMessages(t *testing.T) {
	t.Setenv("SUMA_LANGUAGE", "")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	var out, stderr bytes.Buffer
	if code := Execute(context.Background(), []string{"--control", "invalid", "tasks"}, strings.NewReader(""), &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "--control must") {
		t.Fatal(code, stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Execute(context.Background(), []string{"--language", "zh", "--control", "invalid", "tasks"}, strings.NewReader(""), &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "控制面") {
		t.Fatal(code, stderr.String())
	}
	for _, language := range []string{"zh-TW", "invalid-language", ""} {
		out.Reset()
		stderr.Reset()
		if code := Execute(context.Background(), []string{"--language", language, "--help"}, strings.NewReader(""), &out, &stderr); code != 2 {
			t.Fatalf("invalid explicit locale accepted: %q code=%d", language, code)
		}
	}
}

func TestCLITUILocalesKeepRawUserContentAndMachineStates(t *testing.T) {
	for _, tc := range []struct{ locale, contains string }{{"zh-CN", "只读"}, {"en-US", "read-only"}} {
		c, _ := newClient("http://127.0.0.1:4310", "p", "", 1)
		c.locale, _ = cliLocale(tc.locale, true)
		m := newModel(context.Background(), c)
		m.loading = false
		m.section = 2
		m.items = []map[string]any{{"id": "t1", "title": "用户原文 unchanged", "state": "waiting_input"}}
		view := m.View()
		if !strings.Contains(view, tc.contains) || !strings.Contains(view, "用户原文 unchanged") || !strings.Contains(view, "waiting_input") {
			t.Fatal(view)
		}
	}
}

func TestCLITUIEmptyProjectUsesSelectedLanguage(t *testing.T) {
	for _, tc := range []struct{ locale, expected, absent string }{{"zh-CN", "（请选择项目）", "(select a project)"}, {"en-US", "(select a project)", "（请选择项目）"}} {
		c, _ := newClient("http://127.0.0.1:4310", "", "", 1)
		c.locale, _ = cliLocale(tc.locale, true)
		view := newModel(context.Background(), c).View()
		if !strings.Contains(view, tc.expected) || strings.Contains(view, tc.absent) {
			t.Fatal(view)
		}
	}
}

func TestCLIUnknownOutcomeIsExplicitInBothLanguages(t *testing.T) {
	for _, tc := range []struct{ locale, fragment string }{{"zh-CN", "结果可能未知"}, {"en-US", "outcome may be unknown"}} {
		locale, _ := cliLocale(tc.locale, true)
		f := localizeFailure(locale, &failure{Code: "transport", Message: "control request failed; outcome may be unknown: retry only with the same operation key and payload"})
		if !strings.Contains(f.Message, tc.fragment) || !strings.HasSuffix(f.MessageKey, ".outcome_unknown") || f.Code != "transport" {
			t.Fatal(f)
		}
	}
}
