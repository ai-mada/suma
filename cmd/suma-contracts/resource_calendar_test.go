package main

import "testing"

func TestResourceCalendarContractUsesOneCurrentCalendarAndExplicitUnknown(t *testing.T) {
	api, _ := generate()
	if mapAt(api, "paths", "/planning/capacity")["post"] != nil {
		t.Fatal("obsolete weekly writes may not bypass the calendar")
	}
	if mapAt(api, "paths", "/resource-calendars/{member_id}", "put", "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/ResourceCalendarSaveRequest" {
		t.Fatal("calendar mutation must use the current versioned full request")
	}
	weekdays := mapAt(api, "components", "schemas", "ResourceCalendarSchedule", "properties", "weekdays")
	if weekdays["minItems"] != 7 || weekdays["maxItems"] != 7 {
		t.Fatal("calendar must preserve each Monday-Sunday value")
	}
	if mapAt(api, "components", "schemas", "ResourceCalendarDay", "properties")["actual_hours"] != nil {
		t.Fatal("planned capacity must not claim worklog evidence")
	}
}
