package main

import "testing"

func TestOctopContractKeepsPublicStopAndConfigurationFacts(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	capability := toolContractMap(schemas, "OctopExecutionCapability")
	if capability["additionalProperties"] != false || toolContractMap(capability, "properties", "acp_sdk_version")["const"] != "0.12.1" || toolContractMap(capability, "properties", "budget_mode")["const"] != "observe_only" {
		t.Fatal("generated contract broadened fixed SDK configuration")
	}
	proof := toolContractMap(schemas, "OctopExecutionEvidence", "properties")
	if proof["process_stopped"] == nil || proof["cancel_requested"] == nil || proof["abort_acknowledged"] != nil || proof["api_key"] != nil || proof["home"] != nil {
		t.Fatal("generated public facts conflate ACP intent with host stop or expose local configuration")
	}
}
