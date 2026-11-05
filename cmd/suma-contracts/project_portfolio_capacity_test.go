package main

import "testing"

func TestProjectPortfolioCapacityContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/project-portfolio/capacity", "get")
	if op["x-read-only"] != true || op["requestBody"] != nil || responseSchema(op, "200")["$ref"] != "#/components/schemas/ProjectPortfolioCapacityPage" {
		t.Fatal("capacity must be a typed organization read")
	}
	security := op["security"].([]any)
	if len(security) != 1 || mapAt(security[0])["cookieAuth"] == nil {
		t.Fatal("capacity must use the current browser session")
	}
	queryCount := 0
	for _, value := range op["parameters"].([]any) {
		param := mapAt(value)
		if param["in"] != "query" {
			continue
		}
		queryCount++
		if param["name"] != "week" || param["required"] != true || mapAt(param, "schema")["format"] != "date" {
			t.Fatalf("invalid week contract: %#v", param)
		}
	}
	if queryCount != 1 {
		t.Fatalf("capacity must accept only the week query parameter, got %d", queryCount)
	}
	schemas := mapAt(api, "components", "schemas")
	person := mapAt(schemas, "ProjectPortfolioCapacityPerson", "properties")
	days := mapAt(person, "days")
	if days["minItems"] != 7 || days["maxItems"] != 7 || len(mapAt(person, "state")["enum"].([]string)) != 3 {
		t.Fatal("capacity requires seven dates and an explicit confidence state")
	}
	task := mapAt(schemas, "ProjectPortfolioCapacityTask", "properties")
	for _, hours := range []map[string]any{
		mapAt(person, "available_hours"),
		mapAt(task, "estimated_hours"),
		mapAt(task, "allocated_hours"),
	} {
		options := hours["anyOf"].([]any)
		if len(options) != 2 || mapAt(options[0])["type"] != "number" || mapAt(options[1])["type"] != "null" {
			t.Fatalf("unknown hours must remain nullable: %#v", hours)
		}
	}
}
