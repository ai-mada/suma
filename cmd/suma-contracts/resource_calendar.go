package main

func resourceCalendarContracts(add operationAdder, api map[string]any) {
	for _, method := range []string{"get", "put"} {
		op := add("/resource-calendars/{member_id}", method, "ResourceCalendar", false, method == "put")
		successStatus(op, "200")
		if method == "put" {
			request(op, "ResourceCalendarSaveRequest")
			op["description"] = "Explicit full calendar replacement with current project/member authority, expected calendar version and stable operation receipt. Does not edit task plans or machine execution quotas."
		} else {
			op["x-read-only"] = true
			op["description"] = "Current authorized calendar. Version0 is unconfigured: seven unknown weekdays and no exceptions, not an assumed workweek."
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ResourceCalendar", "ResourceCalendarSaveRequest", "ResourceCalendarSchedule", "ResourceCalendarException", "ResourceCalendarDay", "OrganizationWorkCalendar", "OrganizationWorkCalendarSaveRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	weekdays := toolContractMap(schemas, "ResourceCalendarSchedule", "properties", "weekdays")
	weekdays["minItems"], weekdays["maxItems"] = 7, 7
	weekdays["description"] = "Monday through Sunday; null means unknown and0 is explicitly unavailable."
	toolContractMap(schemas, "ResourceCalendarSchedule", "properties", "exceptions")["maxItems"] = 366
	toolContractMap(schemas, "ResourceCalendarException", "properties", "date")["format"] = "date"
	toolContractMap(schemas, "ResourceCalendarException", "properties", "note")["maxLength"] = 200
	toolContractMap(schemas, "ResourceCalendarException", "properties", "note")["description"] = "At most200 Unicode codepoints of user-authored plain text."
	for _, number := range []map[string]any{toolContractMap(weekdays, "items"), toolContractMap(schemas, "ResourceCalendarException", "properties", "available_hours")} {
		branch := number["anyOf"].([]any)[0].(map[string]any)
		branch["minimum"], branch["maximum"], branch["multipleOf"] = 0, 24, 0.25
	}
	toolContractMap(schemas, "ResourceCalendarDay", "properties", "source")["enum"] = []string{"base", "exception", "unknown"}
	toolContractMap(schemas, "ResourceCalendarSaveRequest", "properties", "expected_version")["minimum"] = 0
	organizationWorkCalendarContracts(add, schemas)
	toolContractMap(schemas, "CapacityEntry", "properties", "days")["minItems"] = 7
	toolContractMap(schemas, "CapacityEntry", "properties", "days")["maxItems"] = 7
	for _, name := range []string{"ResourceCalendar", "ResourceCalendarDay", "CapacityEntry", "OrganizationWorkCalendar"} {
		for _, property := range toolContractMap(schemas, name, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
}
