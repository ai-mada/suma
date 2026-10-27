package runner

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func TestWorkspaceTransportInventoryRequiresSuccessfulNativeProbe(t *testing.T) {
	cfg, entry := nativeHostFixture(t)
	check := func(expectedRead, expectedWrite string, available bool) tc.Observation {
		t.Helper()
		items, err := DescribeTools(context.Background(), cfg)
		if err != nil || len(items) != 1 {
			t.Fatalf("inventory failed: %+v %v", items, err)
		}
		observation := items[0]
		raw, _ := json.Marshal(observation)
		var wire map[string]json.RawMessage
		_ = json.Unmarshal(raw, &wire)
		var read, write string
		_ = json.Unmarshal(wire["workspace_observation_protocol"], &read)
		_ = json.Unmarshal(wire["workspace_write_protocol"], &write)
		if observation.Available != available || read != expectedRead || write != expectedWrite || tc.ValidateObservation(observation) != nil {
			t.Fatalf("probe/current broker capability mismatch: %+v", observation)
		}
		return observation
	}
	check(woc.Protocol, wwc.Protocol, true)
	cfg.AllowExec = false
	check("", "", false)
	cfg.AllowExec = true
	if err := os.WriteFile(entry, []byte("// drifted bundle; must not advertise current transport"), 0600); err != nil {
		t.Fatal(err)
	}
	check("", "", false)
}
