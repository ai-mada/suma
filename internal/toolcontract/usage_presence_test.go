package toolcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func usageEventJSON(usage string, state string) []byte {
	return []byte(`{"schema_version":1,"command_id":"native","profile_sha256":"` + strings.Repeat("a", 64) + `","protocol":"suma-jsonl-v1","kind":"usage","usage_state":"` + state + `"` + usage + `}`)
}

func TestUsageWirePreservesKnownZeroAndOmittedSplits(t *testing.T) {
	e, err := DecodeEvent(usageEventJSON(`,"usage":{"input_tokens":0,"output_tokens":0}`, "complete"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	for _, key := range []string{`"input_tokens":0`, `"output_tokens":0`, `"usage_state":"complete"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("explicit fact lost: %s", raw)
		}
	}
	if strings.Contains(string(raw), "cached_input_tokens") || strings.Contains(string(raw), "reasoning_output_tokens") {
		t.Fatalf("missing split fabricated: %s", raw)
	}
}

func TestUsageContractRejectsNonSafeOrAmbiguousFacts(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"input_tokens":null}`, `{"input_tokens":-1}`, `{"input_tokens":1.5}`, `{"input_tokens":"1"}`, `{"input_tokens":true}`, `{"input_tokens":9007199254740992}`, `{"input_tokens":0,"input_tokens":1}`, `{"unexpected":0}`} {
		t.Run(raw, func(t *testing.T) {
			if _, err := DecodeEvent(usageEventJSON(`,"usage":`+raw, "partial")); err == nil {
				t.Fatal("invalid usage accepted")
			}
		})
	}
	for _, raw := range []string{`{"input_tokens":0}`, `{"output_tokens":0}`} {
		if _, err := DecodeEvent(usageEventJSON(`,"usage":`+raw, "complete")); err == nil {
			t.Fatal("incomplete base counters accepted as complete")
		}
	}
	if _, err := DecodeEvent(usageEventJSON(`,"usage":{"input_tokens":0,"output_tokens":0}`, "unreported")); err == nil {
		t.Fatal("unreported cannot contain measured counters")
	}
}

func TestUsageStateRequiresActualCoverage(t *testing.T) {
	for _, state := range []string{"complete", "partial", "invented", ""} {
		if _, err := DecodeEvent(usageEventJSON("", state)); err == nil {
			t.Fatalf("state without source usage accepted: %q", state)
		}
	}
	if _, err := DecodeEvent(usageEventJSON("", "unreported")); err != nil {
		t.Fatalf("unreported is an explicit unknown, not a zero: %v", err)
	}
	e, err := DecodeEvent(usageEventJSON(`,"usage":{"input_tokens":9007199254740991}`, "partial"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	if !strings.Contains(string(raw), `9007199254740991`) {
		t.Fatal("safe upper bound was not preserved")
	}
}
