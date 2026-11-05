package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestManualRunnerRotationContractKeepsCredentialOnceOnly(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/release-workers/{id}/rotate-credential", "post")
	if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) {
		t.Fatalf("rotation must use current browser authority, got %v", op["security"])
	}
	if got := mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"]; got != "#/components/schemas/ReleaseWorkerRotationRequest" {
		t.Fatal("rotation requires a closed request with stop acknowledgement")
	}
	if got := responseSchema(op, "200")["$ref"]; got != "#/components/schemas/ReleaseWorkerRotationResult" {
		t.Fatal("rotation must return the typed original receipt")
	}
	schemas := mapAt(api, "components", "schemas")
	request := mapAt(schemas, "ReleaseWorkerRotationRequest")
	if request["additionalProperties"] != false || mapAt(request, "properties", "acknowledge_stopped")["const"] != true {
		t.Fatal("request must acknowledge the stop explicitly")
	}
	result := mapAt(schemas, "ReleaseWorkerRotationResult")
	token := mapAt(result, "properties", "token")
	if token["readOnly"] != true || slices.Contains(result["required"].([]string), "token") || mapAt(schemas, "ReleaseWorker", "properties")["token"] != nil {
		t.Fatal("one-time credential must be optional on the result and absent from the registry")
	}
}
