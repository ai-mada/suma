package platform

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// The shared canonical worker namespace must never resolve to an independently
// registered legacy project credential when tools inspect an execution identity.
func TestProjectWorkerCannotRegisterReservedPoolIdentity(t *testing.T) {
	f := newFixture(t)
	pool := f.call("POST", "/execution-pools", poolCreateBody(), 201)
	machine := f.call("POST", "/execution-pools/"+pool["id"].(string)+"/runners", map[string]any{"name": "唯一资源池身份", "max_slots": 1, "ttl_seconds": 3600, "idempotency_key": newID()}, 201)
	worker := machine["runner"].(map[string]any)["worker_id"]
	for _, identity := range []string{worker.(string), strings.ToUpper(worker.(string))} {
		raw, _ := json.Marshal(f.body(map[string]any{"worker_id": identity, "profiles": []any{}, "deployment": false}))
		request := httptest.NewRequest("POST", "/api/v1/release-workers", bytes.NewReader(raw))
		request.AddCookie(f.cookie)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Suma-Expected-User", f.view.ID)
		request.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		// A regressed registration can return a one-time secret. Never include
		// that unexpected response body in test logs.
		if response.Code != 400 {
			t.Fatalf("reserved worker registration returned status %d", response.Code)
		}
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_release_workers WHERE worker_id=?`, worker).Scan(&count); err != nil || count != 0 {
		t.Fatal("pool identity acquired a second project credential", err)
	}
}
