package main

import "testing"

func TestProjectPortfolioContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/project-portfolio", "get")
	if op["x-read-only"] != true || op["requestBody"] != nil || responseSchema(op, "200")["$ref"] != "#/components/schemas/ProjectPortfolioPage" {
		t.Fatalf("portfolio is not a typed read: %#v", op)
	}
	security := op["security"].([]any)
	if len(security) != 1 || mapAt(security[0])["cookieAuth"] == nil {
		t.Fatal("portfolio must be scoped to a browser session")
	}
	query := map[string]bool{}
	for _, p := range op["parameters"].([]any) {
		parameter := mapAt(p)
		if parameter["in"] == "query" {
			query[parameter["name"].(string)] = true
		}
	}
	if len(query) != 3 || !query["q"] || !query["limit"] || !query["after"] {
		t.Fatalf("portfolio has the wrong query contract: %#v", query)
	}
	schemas := mapAt(api, "components", "schemas")
	if mapAt(schemas, "ProjectPortfolioPage", "properties")["total"] != nil ||
		mapAt(schemas, "ProjectPortfolioPage", "properties", "as_of")["format"] != "date-time" ||
		mapAt(schemas, "ProjectPortfolioMilestone", "properties", "target_date")["format"] != "date" {
		t.Fatal("portfolio must describe a bounded page and distinguish snapshot time from planned date")
	}
}
