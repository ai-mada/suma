package main

func organizationWorkCalendarContracts(add operationAdder, schemas map[string]any) {
	const path = "/organization/work-calendars/{user_id}"
	read := add(path, "get", "OrganizationWorkCalendar", false, false)
	write := add(path, "put", "OrganizationWorkCalendar", false, true)
	for _, op := range []map[string]any{read, write} {
		browserOnly(op)
		successStatus(op, "200")
		op["responses"].(map[string]any)["404"] = map[string]any{"description": "Target is no longer a current member of the active organization."}
	}
	read["x-read-only"] = true
	read["description"] = "Current organization availability calendar. An organization administrator may view an active member; members may view their own calendar. Version 0 has seven unknown weekdays."
	request(write, "OrganizationWorkCalendarSaveRequest")
	write["description"] = "Replace a current member's organization calendar as a current organization owner or administrator. Independent version and original-operation receipt; no project resource calendar changes."
	fields := toolContractMap(schemas, "OrganizationWorkCalendarSaveRequest", "properties")
	version := fields["expected_version"].(map[string]any)
	version["minimum"], version["maximum"] = 0, 2147483646
	key := fields["idempotency_key"].(map[string]any)
	key["minLength"], key["maxLength"] = 8, 128
}
