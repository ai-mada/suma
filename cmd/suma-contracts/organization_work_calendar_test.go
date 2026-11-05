package main

import "testing"

func TestOrganizationWorkCalendarContract(t *testing.T) {
	api, _ := generate()
	path := mapAt(api, "paths", "/organization/work-calendars/{user_id}")
	for _, method := range []string{"get", "put"} {
		op := mapAt(path, method)
		security := op["security"].([]any)
		if len(security) != 1 || mapAt(security[0])["cookieAuth"] == nil ||
			responseSchema(op, "200")["$ref"] != "#/components/schemas/OrganizationWorkCalendar" {
			t.Fatalf("%s must return the current calendar to a browser session", method)
		}
		for _, raw := range op["parameters"].([]any) {
			if parameter := mapAt(raw); parameter["in"] == "query" {
				t.Fatalf("%s exposes unexpected query scope: %v", method, parameter)
			}
		}
	}
	save := mapAt(path, "put")
	if mapAt(save, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/OrganizationWorkCalendarSaveRequest" ||
		mapAt(save, "x-browser-original-view")["required"] != true {
		t.Fatal("organization calendar writes must be versioned and tied to the original browser view")
	}
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"OrganizationWorkCalendar", "OrganizationWorkCalendarSaveRequest"} {
		model := mapAt(schemas, name)
		if model["additionalProperties"] != false ||
			mapAt(model, "properties", "schedule")["$ref"] != "#/components/schemas/ResourceCalendarSchedule" {
			t.Fatalf("%s must share the closed resource schedule schema", name)
		}
	}
}
