package platform

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkBuddyCloudCollaborationHTTPClosesQueriesBodiesAndMachineScope(t *testing.T) {
	h, sub, posts := cloudHistoryFixture(t, nil, nil)
	server := &Server{Service: h.f.s, Engine: h.f.e}
	check := func(method, path, body, scope string, actor Actor, parts []string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		out := httptest.NewRecorder()
		if !server.workBuddyCloudCollaborationHTTP(out, r, actor, scope, "", parts, h.w) {
			t.Fatal("route not recognized")
		}
		if out.Code != want || out.Header().Get("Cache-Control") != "no-store" || out.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("unsafe route response", out.Code, out.Body.String())
		}
	}
	parts := []string{"workbuddy-cloud-submissions", sub.ID, "collaboration"}
	path := "/api/v1/workbuddy-cloud-submissions/" + sub.ID + "/collaboration?project_id=" + h.f.project
	check("GET", path, "", "", h.f.actor, parts, 200)
	check("GET", path+"&unknown=1", "", "", h.f.actor, parts, 400)
	check("GET", path+"&project_id="+h.f.project, "", "", h.f.actor, parts, 400)
	check("GET", path, "", "runner", h.actor, parts, 403)
	req := cloudHistoryRequest(h, sub)
	raw := stringify(req)
	check("POST", "/unused", strings.Replace(raw, `"expected_version":0,`, "", 1), "", h.f.actor, []string{"workbuddy-cloud-submissions", sub.ID, "history-loads"}, 400)
	if posts.Load() != 0 {
		t.Fatal("invalid route attached")
	}
	load, err := h.w.LoadCloudHistory(context.Background(), h.f.actor, h.f.project, sub.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	check("GET", "/unused?project_id="+h.f.project+"&after=0&limit=1", "", "", h.f.actor, []string{"workbuddy-cloud-history-loads", load.ID}, 200)
	check("GET", "/unused?project_id="+h.f.project+"&limit=0", "", "", h.f.actor, []string{"workbuddy-cloud-history-loads", load.ID}, 400)
}
