package main

import "testing"

func TestOctopContractKeepsPublicStopAndConfigurationFacts(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	capability := toolContractMap(schemas, "OctopExecutionCapability")
	fields := toolContractMap(capability, "properties")
	if capability["additionalProperties"] != false || toolContractMap(fields, "acp_sdk_version")["const"] != "0.12.1" || len(fields) != 2 {
		t.Fatal("generated runtime capability reintroduced a second model authority")
	}
	proof := toolContractMap(schemas, "OctopExecutionEvidence", "properties")
	if proof["process_stopped"] == nil || proof["cancel_requested"] == nil || proof["binding_sha256"] == nil || proof["abort_acknowledged"] != nil || proof["api_key"] != nil || proof["home"] != nil {
		t.Fatal("generated public facts conflate ACP intent with host stop or expose local configuration")
	}
}
