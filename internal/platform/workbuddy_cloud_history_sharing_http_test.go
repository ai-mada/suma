package platform

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkBuddyCloudHistorySharingHTTPClosesBodyQueryAndActor(t *testing.T) {
	h, load, req := historySharingFixture(t)
	server := &Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w}
	call := func(actor Actor, scope, path, body string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		out := httptest.NewRecorder()
		if !server.workBuddyCloudHistorySharingHTTP(out, r, actor, scope, h.f.project, []string{"workbuddy-cloud-history-loads", load.ID, "share-preview"}, h.w) {
			t.Fatal("share route unhandled")
		}
		if out.Code != want || out.Header().Get("Cache-Control") != "no-store" || out.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("share HTTP contract", out.Code, out.Body.String())
		}
	}
	raw := stringify(req)
	call(h.f.actor, "", "/unused", raw, 200)
	call(h.f.actor, "", "/unused?project_id="+h.f.project, raw, 400)
	call(h.f.actor, "", "/unused", strings.Replace(raw, `"project_id":`, `"body":"forged","project_id":`, 1), 400)
	call(h.actor, "runner", "/unused", raw, 403)
	for _, locale := range []string{"zh-CN", "en-US"} {
		for _, test := range []struct {
			body, code, key string
			status          int
		}{
			{strings.Replace(raw, `"project_id":`, `"body":"forged","project_id":`, 1), "workbuddy_cloud_history_share_invalid", "workbuddy_history_sharing.invalid", 400},
			{strings.Replace(raw, `"expected_load_version":`+stringify(req.ExpectedLoadVersion), `"expected_load_version":`+stringify(req.ExpectedLoadVersion+1), 1), "workbuddy_cloud_history_share_changed", "workbuddy_history_sharing.changed", 409},
		} {
			r := httptest.NewRequest("POST", "/unused", strings.NewReader(test.body))
			r.Header.Set("Accept-Language", locale)
			out := httptest.NewRecorder()
			out.Header().Set("Content-Language", locale)
			if !server.workBuddyCloudHistorySharingHTTP(out, r, h.f.actor, "", h.f.project, []string{"workbuddy-cloud-history-loads", load.ID, "share-preview"}, h.w) {
				t.Fatal("localized share route unhandled")
			}
			var body map[string]any
			if json.Unmarshal(out.Body.Bytes(), &body) != nil {
				t.Fatal("invalid error envelope")
			}
			detail, ok := body["error"].(map[string]any)
			if !ok || out.Code != test.status || detail["code"] != test.code || detail["message_key"] != test.key || detail["localized"] != true || detail["locale"] != locale || out.Header().Get("Content-Language") != locale || detail["message"] == test.code {
				t.Fatal("share error was generic or untranslated", locale, out.Body.String())
			}
		}
	}
}
