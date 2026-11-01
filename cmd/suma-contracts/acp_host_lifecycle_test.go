package main

import "testing"

func TestACPHostContractSeparatesActualExitAndCancelNotification(t *testing.T) {
	api, _ := generate()
	s := mapAt(api, "components", "schemas")
	for _, name := range []string{"ACPExecutionEvidence", "ACPModelDiscoveryEvidence"} {
		p := mapAt(s, name, "properties")
		if mapAt(p, "host_process_exit_code")["minimum"] != -1 || mapAt(p, "host_process_exit_code")["maximum"] != 4294967295 || p["host_stopped_after_rpc"] == nil || len(mapAt(s, name)["allOf"].([]any)) != 2 {
			t.Fatal("RPC and native exit evidence were conflated", name)
		}
	}
	p := mapAt(s, "ACPModelDiscoveryEvidence", "properties")
	if p["session_cancel_requested"] == nil || p["session_cancel_written"] == nil || p["session_close_requested"] != nil || p["session_close_acknowledged"] != nil {
		t.Fatal("notification became a fictional native ACK")
	}
}
