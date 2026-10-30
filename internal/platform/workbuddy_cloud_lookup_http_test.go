package platform

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudLookupHTTPUsesCurrentRealMachineOnly(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "pool"}[pooled], func(t *testing.T) {
			h := newCloudFixtureMode(t, nil, pooled)
			h.f.handler = (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w, AllowedOrigins: []string{"http://suma.test"}}).Handler()
			namespace, token := "pool-runner", h.poolToken
			if !pooled {
				namespace = "runner"
				token = h.claimToken
				if token == "" {
					t.Fatal("protected claim fixture did not retain its original machine credential")
				}
			}
			request := func(method, suffix string, raw []byte, browser bool) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "http://suma.test/api/v1/"+namespace+"/workbuddy-cloud-submissions"+suffix, bytes.NewReader(raw))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", "http://suma.test")
				if browser {
					r.AddCookie(h.f.cookie)
					r.Header.Set("X-Suma-Expected-User", h.f.view.ID)
					r.Header.Set("X-Suma-Expected-Organization", h.f.view.TenantID)
				} else {
					r.Header.Set("Authorization", "Bearer "+token)
				}
				w := httptest.NewRecorder()
				h.f.handler.ServeHTTP(w, r)
				return w
			}
			req := cloudLookupRequest(h)
			raw, _ := json.Marshal(req)
			before := cloudLookupStoredFingerprint(t, h)
			w := request("POST", "/lookup", raw, false)
			var out wc.CloudLookupResult
			if w.Code != 200 || wc.Decode(w.Body.Bytes(), &out) != nil || out.Validate() != nil || out.State != "not_found" {
				t.Fatal("authenticated lookup failed", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || before != cloudLookupStoredFingerprint(t, h) {
				t.Fatal("read leaked cache or changed business state")
			}
			if w = request("POST", "/lookup", raw, true); w.Code < 400 {
				t.Fatal("browser borrowed machine lookup", w.Code)
			}
			for _, invalid := range [][]byte{
				[]byte(strings.TrimSuffix(string(raw), "}") + `,"idempotency_key":"new-operation"}`),
				[]byte(strings.TrimSuffix(string(raw), "}") + `,"prompt":"replacement"}`),
				[]byte(strings.TrimSuffix(string(raw), "}") + `,"project_id":"other"}`),
				[]byte(strings.Replace(string(raw), `"delegation_id":"`+req.DelegationID+`"`, `"delegation_id":null`, 1)),
			} {
				if w = request("POST", "/lookup", invalid, false); w.Code != 400 {
					t.Fatal("open lookup body accepted", w.Code)
				}
			}
			if w = request("POST", "/lookup?project_id="+h.f.project, raw, false); w.Code != 400 {
				t.Fatal("lookup query accepted", w.Code)
			}
			if w = request("GET", "/lookup", raw, false); w.Code < 400 {
				t.Fatal("lookup proof accepted in GET", w.Code)
			}
			create, _ := json.Marshal(h.request)
			if w = request("POST", "", create, false); w.Code != 201 {
				t.Fatal("fixture original create failed", w.Code, w.Body.String())
			}
			before = cloudLookupStoredFingerprint(t, h)
			if w = request("POST", "/lookup", raw, false); w.Code != 200 || wc.Decode(w.Body.Bytes(), &out) != nil || out.Submission == nil || out.State != "task_known" {
				t.Fatal("original submission not read through authenticated HTTP", w.Code)
			}
			if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 1 || h.queries.Load() != 0 {
				t.Fatal("HTTP read performed extra work")
			}
		})
	}
}
