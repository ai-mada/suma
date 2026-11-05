package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func organizationCalendarTestPath(user string) string {
	return "/organization/work-calendars/" + user
}

func organizationCalendarTestRequest(version int, schedule map[string]any) map[string]any {
	return map[string]any{"expected_version": version, "idempotency_key": newID(), "schedule": schedule}
}

func TestOrganizationWorkCalendarHTTPUnknownAndOriginalReceipt(t *testing.T) {
	f := newFixture(t)
	path := organizationCalendarTestPath(f.user.ID)
	before := domainACount(t, f.s, "suma_operations")
	unknown := f.call("GET", path, nil, 200)
	schedule := unknown["schedule"].(map[string]any)
	if unknown["version"] != float64(0) || unknown["organization_id"] != f.user.TenantID ||
		unknown["user_id"] != f.user.ID || unknown["can_edit"] != true || unknown["updated_at"] != "" ||
		len(schedule["weekdays"].([]any)) != 7 || len(schedule["exceptions"].([]any)) != 0 {
		t.Fatalf("unexpected unset organization calendar: %v", unknown)
	}
	for _, hours := range schedule["weekdays"].([]any) {
		if hours != nil {
			t.Fatal("unset calendar synthesized availability")
		}
	}
	if domainACount(t, f.s, organizationWorkCalendarTable) != 0 || domainACount(t, f.s, "suma_operations") != before {
		t.Fatal("calendar GET wrote a calendar or command receipt")
	}
	request := organizationCalendarTestRequest(0, map[string]any{
		"weekdays": []any{0, nil, 6.25, 8, 8, 0, 0},
		"exceptions": []any{map[string]any{"date": "2026-11-09", "available_hours": nil, "note": "pending"},
			map[string]any{"date": "2026-11-10", "available_hours": 0}},
	})
	first := f.call("PUT", path, request, 200)
	weekdays := first["schedule"].(map[string]any)["weekdays"].([]any)
	exceptions := first["schedule"].(map[string]any)["exceptions"].([]any)
	if first["version"] != float64(1) || first["updated_at"] == "" ||
		weekdays[0] != float64(0) || weekdays[1] != nil || weekdays[2] != float64(6.25) ||
		exceptions[0].(map[string]any)["available_hours"] != nil || exceptions[1].(map[string]any)["available_hours"] != float64(0) {
		t.Fatalf("stored calendar lost explicit nulls, zero or quarter hours: %v", first)
	}
	f.call("PUT", path, organizationCalendarTestRequest(0, calendarTestSchedule(8)), 409)
	second := f.call("PUT", path, organizationCalendarTestRequest(1, calendarTestSchedule(6)), 200)
	if replay := f.call("PUT", path, request, 200); stringify(replay) != stringify(first) {
		t.Fatal("original successful receipt was replaced by the current value")
	}
	if read := f.call("GET", path, nil, 200); stringify(read) != stringify(second) {
		t.Fatal("recovering original receipt changed the calendar head")
	}
	request["schedule"] = calendarTestSchedule(4)
	f.call("PUT", path, request, 409)
	if domainACount(t, f.s, "suma_resource_calendars") != 0 {
		t.Fatal("organization update wrote a project resource calendar")
	}
}

func TestOrganizationWorkCalendarCurrentAuthorityAndIsolation(t *testing.T) {
	f := newFixture(t)
	owner := f.cookie
	ownerPath := organizationCalendarTestPath(f.user.ID)
	f.call("PUT", ownerPath, organizationCalendarTestRequest(0, calendarTestSchedule(8)), 200)
	invited := governanceInvite(f, "calendar-admin@example.test", "admin", []any{})
	member := governanceAccept(f, invited, "calendar-admin@example.test")
	adminID := member["user"].(map[string]any)["id"].(string)
	admin := f.cookie
	adminPath := organizationCalendarTestPath(adminID)
	request := organizationCalendarTestRequest(1, calendarTestSchedule(7))
	f.call("PUT", ownerPath, request, 200)

	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	targetRequest := organizationCalendarTestRequest(0, calendarTestSchedule(5))
	f.call("PUT", adminPath, targetRequest, 200)
	f.call("PATCH", "/organization/members/"+adminID, map[string]any{"role": "member", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.cookie = admin
	f.call("GET", "/auth/session", nil, 200)
	if current := f.call("GET", adminPath, nil, 200); current["can_edit"] != false || current["version"] != float64(1) {
		t.Fatalf("ordinary member could not read their own calendar: %v", current)
	}
	f.call("GET", ownerPath, nil, 403)
	f.call("PUT", ownerPath, request, 403)
	f.call("PUT", adminPath, organizationCalendarTestRequest(1, calendarTestSchedule(3)), 403)

	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	other := f.call("POST", "/organizations", map[string]any{"name": "Separate calendar organization", "idempotency_key": newID()}, 201)["id"].(string)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": other, "idempotency_key": newID()}, 200)
	if current := f.call("GET", ownerPath, nil, 200); current["version"] != float64(0) || current["organization_id"] != other {
		t.Fatalf("calendar was not isolated to its organization: %v", current)
	}
	f.call("GET", adminPath, nil, 404)
	f.call("PUT", adminPath, targetRequest, 404)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": f.user.TenantID, "idempotency_key": newID()}, 200)
	f.call("PATCH", "/organization/members/"+adminID, map[string]any{"action": "remove", "expected_version": 2, "idempotency_key": newID()}, 200)
	f.call("GET", adminPath, nil, 404)
	f.call("PUT", adminPath, targetRequest, 404)
}

func TestOrganizationWorkCalendarHTTPClosedRequest(t *testing.T) {
	f := newFixture(t)
	path := organizationCalendarTestPath(f.user.ID)
	f.call("GET", path+"?organization_id="+f.user.TenantID, nil, 400)
	f.call("GET", path+"/extra", nil, 404)
	apiToken := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	f.bearer("GET", path, apiToken, nil, 403)
	f.bearer("PUT", path, apiToken, organizationCalendarTestRequest(0, calendarTestSchedule(8)), 403)

	request := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
	request.AddCookie(f.cookie)
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("calendar read is cacheable or unavailable")
	}
	request = httptest.NewRequest(http.MethodHead, "/api/v1"+path, nil)
	request.AddCookie(f.cookie)
	recorder = httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	if recorder.Code != 405 || recorder.Header().Get("Allow") != "GET, PUT" {
		t.Fatal("calendar HTTP method boundary is not closed")
	}

	for _, fields := range []string{
		`"expected_version":null`,
		`"expected_version":0e0`,
		`"expected_version":0.0000000000000000001`,
		`"expected_version":0,"expected_version":0`,
		`"expected_version":0,"organization_id":"forged"`,
		`"expected_version":0,"project_id":"forged"`,
		`"expected_version":0,"user_id":"forged"`,
	} {
		body := `{` + fields + `,"idempotency_key":"organization-calendar-test","schedule":{"weekdays":[0,8,8,8,8,0,null],"exceptions":[]}}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1"+path, strings.NewReader(body))
		req.AddCookie(f.cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Expected-User", f.user.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("invalid wire fields %s returned %d", fields, w.Code)
		}
	}
	schedule := calendarTestSchedule(8)
	schedule["exceptions"] = []any{map[string]any{"date": "2026-11-09"}}
	f.call("PUT", path, organizationCalendarTestRequest(0, schedule), 400)
	f.call("PUT", path, organizationCalendarTestRequest(0, calendarTestSchedule(8.1)), 400)
	if domainACount(t, f.s, organizationWorkCalendarTable) != 0 {
		t.Fatal("invalid request wrote an organization calendar")
	}
}

func TestOrganizationWorkCalendarCASAcrossConnections(t *testing.T) {
	f := newFixture(t)
	var seq int
	var schema, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &schema, &path); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	services := []*Service{f.s, {DB: other, Dialect: "sqlite"}}

	type result struct {
		request OrganizationWorkCalendarSaveRequest
		value   OrganizationWorkCalendar
		err     error
	}
	results := make(chan result, len(services))
	var wait sync.WaitGroup
	for i, service := range services {
		wait.Add(1)
		go func(i int, service *Service) {
			defer wait.Done()
			hours := float64(i + 6)
			req := OrganizationWorkCalendarSaveRequest{
				IdempotencyKey: fmt.Sprintf("calendar-concurrent-%d", i),
				Schedule: ResourceCalendarSchedule{
					Weekdays:   []*float64{&hours, nil, nil, nil, nil, nil, nil},
					Exceptions: []ResourceCalendarException{},
				},
			}
			value, err := service.SaveOrganizationWorkCalendar(context.Background(), f.actor, f.user.ID, req)
			results <- result{req, value, err}
		}(i, service)
	}
	wait.Wait()
	close(results)

	var success int
	for received := range results {
		if received.err != nil {
			requireAuthorityStatus(t, received.err, 409)
			continue
		}
		success++
		replay, err := f.s.SaveOrganizationWorkCalendar(context.Background(), f.actor, f.user.ID, received.request)
		if err != nil || replay.Version != 1 || stringify(replay) != stringify(received.value) {
			t.Fatal("winning command could not recover its original receipt", err)
		}
	}
	if success != 1 || domainACount(t, f.s, organizationWorkCalendarTable) != 1 {
		t.Fatal("concurrent writers did not commit exactly one calendar")
	}
}

func TestOrganizationWorkCalendarRejectsInvalidInitialSchemaBeforeWrites(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ddl        string
		registered bool
	}{
		{"unregistered", organizationWorkCalendarDDL, false},
		{"missing columns", `CREATE TABLE suma_organization_work_calendars(tenant_id TEXT,user_id TEXT,PRIMARY KEY(tenant_id,user_id))`, true},
		{"missing primary key", `CREATE TABLE suma_organization_work_calendars(tenant_id TEXT,user_id TEXT,version INTEGER,schedule TEXT,updated_at TEXT)`, true},
		{"view", `CREATE VIEW suma_organization_work_calendars AS SELECT 1 AS tenant_id,1 AS user_id,1 AS version,1 AS schedule,1 AS updated_at`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "calendar.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if tc.registered {
				if err = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.ExecContext(ctx, tc.ddl); err != nil {
				t.Fatal(err)
			}
			var before, after int
			const count = `SELECT COUNT(*) FROM sqlite_master`
			if err = db.QueryRowContext(ctx, count).Scan(&before); err != nil {
				t.Fatal(err)
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			if err = s.Initialize(ctx); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatalf("invalid calendar schema was accepted: %v", err)
			}
			if err = db.QueryRowContext(ctx, count).Scan(&after); err != nil || before != after {
				t.Fatalf("schema rejection changed initial schema: %d -> %d (%v)", before, after, err)
			}
		})
	}
}
