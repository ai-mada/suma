package workspaceobservationcontract

import "testing"

func TestWorkspaceObservationClosedWire(t *testing.T) {
	good := `{"project_id":"project-a","scope":{"run_id":"run-a","node_id":"code","attempt":1,"fence":2,"worker_id":"worker-a","tool_profile_id":"profile-a","profile_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"operation":"read","arguments":{"path":"src/main.go"},"idempotency_key":"read-original-037"}`
	var request CreateRequest
	if err := Decode([]byte(good), &request); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"project_id":"a","project_id":"b"}`, `{"scope":{"Attempt":1}}`, `{"scope":{"attempt":null}}`, `{"arguments":{"path":"\ud800"}}`, `{"arguments":{"max_lines":1.5}}`, `{"unexpected":1}`} {
		if Decode([]byte(raw), &request) == nil {
			t.Fatal("ambiguous wire accepted", raw)
		}
	}
}
func TestWorkspaceObservationReadBoundary(t *testing.T) {
	for _, p := range []string{"../file", "a/../file", "C:/file", "\\\\host\\file", "/file", "a//file", "a/./file"} {
		if ValidPath(p, false) {
			t.Fatal("nonlocal path accepted", p)
		}
	}
	o := Observation{Operation: "read", Arguments: Arguments{Path: "src/main.go"}, ExpectedSHA256: "a" + string(make([]byte, 63))}
	if ValidateResult(o, &Result{Read: &Read{Path: "other.go", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Bytes: 1, StartLine: 1, Content: "x", Complete: true}}, "") == nil {
		t.Fatal("wrong file receipt accepted")
	}
	if ResultDigest(nil, "workspace_closed") == ResultDigest(nil, "path_not_allowed") {
		t.Fatal("closed errors share an identity")
	}
}
