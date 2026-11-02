package main

import (
	"strings"
	"testing"

	pc "github.com/ai-mada/suma/internal/poolconnectioncontract"
)

func TestPoolConnectionContractSeparatesHumanProjectionAndLocalMachineCheck(t *testing.T) {
	api, types := generate()
	op := mapAt(api, "paths", "/execution-pools/{pool_id}/runners/{runner_id}/readiness", "get")
	if op["x-read-only"] != true || op["requestBody"] != nil {
		t.Fatal("connection projection must not start a Worker or mutate authority")
	}
	security := op["security"].([]any)
	if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
		t.Fatal("Pool machine or project tokens cannot become human administration")
	}
	queries := map[string]bool{}
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["in"] == "query" {
			name := p["name"].(string)
			if queries[name] || p["required"] != true || name != "tenant_id" && name != "project_id" {
				t.Fatal("scope must be two exact required parameters without extra authority")
			}
			queries[name] = true
		}
	}
	if len(queries) != 2 {
		t.Fatal("missing exact configured scope")
	}
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"PoolRunnerReadiness", "PoolRunnerReadinessRunner", "PoolRunnerReadinessScope", "PoolConnectionCheck", "PoolConnectionScope"} {
		model := mapAt(schemas, name)
		if model["additionalProperties"] != false || !strings.Contains(types, "export interface "+name+" {") {
			t.Fatalf("current Go wire and closed generated SDK must agree: %s", name)
		}
		for _, forbidden := range []string{"token", "secret", "path", "commands", "ready", "connected"} {
			if mapAt(model, "properties")[forbidden] != nil {
				t.Fatalf("no secret, private input or combined readiness flag: %s.%s", name, forbidden)
			}
		}
	}
	if mapAt(schemas, "PoolConnectionCheck", "properties", "protocol")["const"] != pc.PoolConnectionCheckProtocol || mapAt(schemas, "PoolConnectionCheck")["x-local-cli-only"] != true {
		t.Fatal("bounded native session check is a separate local protocol")
	}
}
