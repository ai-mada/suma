package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAPIInternationalizationIsRequestScoped(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		locale := "en-US"
		if i%2 == 0 {
			locale = "zh-CN"
		}
		wg.Add(1)
		go func(locale string) {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
			r.Header.Set("Accept-Language", locale)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Error(err)
				return
			}
			detail, ok := body["error"].(map[string]any)
			if !ok {
				t.Error("authentication error envelope missing")
				return
			}
			if w.Code != 401 || w.Header().Get("Content-Language") != locale || detail["locale"] != locale || detail["code"] != "unauthenticated" || detail["localized"] != true || detail["message_key"] == "" {
				t.Errorf("request language/identity mixed: locale=%s status=%d body=%s", locale, w.Code, w.Body.String())
			}
			if locale == "en-US" && strings.Contains(detail["message"].(string), "登录") {
				t.Error("known login message not translated")
			}
		}(locale)
	}
	wg.Wait()
}

func TestAPIInternationalizationPreservesUnknownDiagnostics(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Language", "en-US")
	const diagnostic = "用户自定义原始诊断保留"
	writeError(w, &APIError{400, "invalid_input", diagnostic, map[string]any{"record_id": "unchanged"}})
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	detail := body["error"].(map[string]any)
	if detail["message"] != diagnostic || detail["localized"] != false || detail["fallback"] == nil || detail["fallback"] == "" || detail["code"] != "invalid_input" || detail["details"].(map[string]any)["record_id"] != "unchanged" {
		t.Fatal("unknown exact diagnostic was hidden by generic translation", w.Body.String())
	}
}

func TestLocalizationCatalogIsPublicStaticAndVersioned(t *testing.T) {
	f := newFixture(t)
	f.cookie = nil
	first := f.call("GET", "/localization/catalog", nil, 200)
	second := f.call("GET", "/localization/catalog", nil, 200)
	if first["version"] != second["version"] || len(first["version"].(string)) != 64 || first["template_format"] != "named-dot-v1" {
		t.Fatal("catalog has unstable content identity")
	}
	if len(first["messages"].([]any)) < 12 {
		t.Fatal("compiled message catalog is missing")
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), f.user.ID) || strings.Contains(string(raw), f.project) {
		t.Fatal("public catalog includes runtime identity")
	}
	f.call("POST", "/localization/catalog", map[string]any{}, 405)
}
