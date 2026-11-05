package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

func TestManualWorkerCandidateRecoveryRetainsBytesAndUnblocksRotation(t *testing.T) {
	f, genericToken, request := candidateTransferFixture(t)
	registration := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": request.Assignment.WorkerID, "profiles": []any{}}), 201)
	token, id := registration["token"].(string), registration["id"].(string)
	path := "/release-workers/" + id + "/candidate-transfers"
	start := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, request), 201))
	bundle := []byte("candidate bytes")
	candidateBinary(t, f, "runner", token, start.TransferID, newID(), request.Assignment, http.MethodPut, bundle, 200)
	list := path + "?project_id=" + f.project
	abandon := path + "/" + start.TransferID + "/abandon"
	cmd := f.body(map[string]any{"expected_worker_version": 1, "expected_bundle_sha256": start.BundleSHA256, "acknowledge_incomplete": true})
	rotation := f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true})

	item := f.call("GET", list, nil, 200)["items"].([]any)[0].(map[string]any)
	if item["transfer_id"] != start.TransferID || item["attempt_state"] != "running" || item["disposition"] != "awaiting_attempt" {
		t.Fatalf("incorrect in-flight transfer: %+v", item)
	}
	if raw := stringify(item); strings.Contains(raw, digest(token)) || strings.Contains(raw, "blobs/") {
		t.Fatal("recovery projection exposed transfer authority or storage paths")
	}
	f.bearer("GET", list, genericToken, nil, 403)
	if code := f.call("POST", abandon, cmd, 409)["error"].(map[string]any)["code"]; code != "candidate_transfer_attempt_unresolved" {
		t.Fatalf("unexpected code for running attempt: %s", code)
	}
	f.call("POST", manualRotationPath(id), rotation, 409)

	ref := request.Assignment
	result := dag.Result{
		RunID: ref.RunID, NodeID: ref.NodeID, Attempt: ref.Attempt, Fence: ref.Fence, WorkerID: ref.WorkerID,
		ExitCode: 1, Error: "fixture command failed after uploading", OperationID: newID(),
	}
	ctx := context.Background()
	if err := f.e.Complete(ctx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal(err)
	}
	item = f.call("GET", list, nil, 200)["items"].([]any)[0].(map[string]any)
	if item["attempt_state"] != "failed" || item["disposition"] != "eligible" || item["bundle_sha256"] != start.BundleSHA256 {
		t.Fatalf("incorrect resolved transfer: %+v", item)
	}
	f.call("POST", abandon, f.body(map[string]any{"expected_worker_version": 2, "expected_bundle_sha256": start.BundleSHA256, "acknowledge_incomplete": true}), 409)
	f.call("POST", abandon, f.body(map[string]any{"expected_worker_version": 1, "expected_bundle_sha256": strings.Repeat("a", 64), "acknowledge_incomplete": true}), 409)
	f.call("POST", abandon, f.body(map[string]any{"expected_worker_version": 1, "expected_bundle_sha256": start.BundleSHA256, "acknowledge_incomplete": false}), 400)

	resultQuery := `UPDATE dag_attempts SET state=?,result=? WHERE run_id=? AND node_id=? AND attempt=?`
	mutateResult := func(change func(*dag.Result)) string {
		changed := result
		change(&changed)
		return stringify(changed)
	}
	for _, scenario := range []struct {
		name, state, raw string
	}{
		{"unknown outcome", "failed", mutateResult(func(r *dag.Result) { r.OutcomeUnknown = true })},
		{"other attempt", "failed", mutateResult(func(r *dag.Result) { r.Fence++ })},
		{"reported bundle", "failed", mutateResult(func(r *dag.Result) { r.WorkspaceEvidence = map[string]any{"candidate_bundle": start} })},
		{"contradictory success result", "failed", mutateResult(func(r *dag.Result) { r.ExitCode, r.Error = 0, "" })},
		{"success", "succeeded", stringify(result)},
		{"reconciling", "reconciling", stringify(result)},
		{"duplicate outcome", "failed", strings.Replace(stringify(result), `"outcome_unknown":false`, `"outcome_unknown":true,"outcome_unknown":false`, 1)},
		{"aliased outcome", "failed", strings.Replace(stringify(result), `"outcome_unknown":false`, `"OUTCOME_UNKNOWN":true,"outcome_unknown":false`, 1)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := f.s.DB.Exec(resultQuery, scenario.state, scenario.raw, ref.RunID, ref.NodeID, ref.Attempt); err != nil {
				t.Fatal(err)
			}
			if code := f.call("POST", abandon, cmd, 409)["error"].(map[string]any)["code"]; code != "candidate_transfer_attempt_unresolved" {
				t.Fatalf("unexpected refusal: %s", code)
			}
		})
	}
	if _, err := f.s.DB.Exec(resultQuery, "failed", stringify(result), ref.RunID, ref.NodeID, ref.Attempt); err != nil {
		t.Fatal(err)
	}
	unknownField := f.body(map[string]any{"expected_worker_version": 1, "expected_bundle_sha256": start.BundleSHA256, "acknowledge_incomplete": true, "action": "publish"})
	f.call("POST", abandon, unknownField, 400)
	f.call("POST", abandon, json.RawMessage(strings.Replace(stringify(cmd), `"expected_worker_version":1`, `"expected_worker_version":1.0`, 1)), 400)
	expireManualArtifactWorker(t, f, token)
	receipt := f.call("POST", abandon, cmd, 200)
	if receipt["state"] != "abandoned" || receipt["operation_id"] != cmd["idempotency_key"] || receipt["transfer_id"] != start.TransferID {
		t.Fatalf("incorrect recovery receipt: %+v", receipt)
	}
	var raw, pathOnDisk string
	if err := f.s.DB.QueryRow(`SELECT binding,storage_path FROM suma_candidate_transfers WHERE id=?`, start.TransferID).Scan(&raw, &pathOnDisk); err != nil {
		t.Fatal(err)
	}
	var retained cc.Binding
	start.State = "abandoned"
	if err := json.Unmarshal([]byte(raw), &retained); err != nil || retained != start {
		t.Fatalf("abandonment changed manifest: %+v %v", retained, err)
	}
	if data, err := os.ReadFile(filepath.Join(f.s.DataDir, "release-artifacts", filepath.FromSlash(pathOnDisk))); err != nil || !bytes.Equal(data, bundle) {
		t.Fatalf("abandonment changed bytes: %v", err)
	}
	if domainACount(t, f.s, "suma_release_artifacts") != 0 {
		t.Fatal("abandonment published a release artifact")
	}
	var state string
	if err := f.s.DB.QueryRow(`SELECT state FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, ref.RunID, ref.NodeID, ref.Attempt).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("abandonment changed original result: %s %v", state, err)
	}
	if items := f.call("GET", list, nil, 200)["items"].([]any); len(items) != 0 {
		t.Fatalf("abandoned transfer remained in recovery list: %+v", items)
	}
	tampered := retained
	tampered.Source.WorkerID = "different-worker"
	for _, changed := range []string{stringify(tampered), `{"state":"abandoned","State":"sealed"}`} {
		if _, err := f.s.DB.Exec(`UPDATE suma_candidate_transfers SET binding=? WHERE id=?`, changed, start.TransferID); err != nil {
			t.Fatal(err)
		}
		f.call("POST", manualRotationPath(id), rotation, 409)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_candidate_transfers SET binding=? WHERE id=?`, raw, start.TransferID); err != nil {
		t.Fatal(err)
	}
	f.call("POST", manualRotationPath(id), rotation, 200)
	if replay := f.call("POST", abandon, cmd, 200); !reflect.DeepEqual(receipt, replay) {
		t.Fatal("exact replay changed after credential rotation")
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call("POST", abandon, cmd, 403)
	f.call("GET", list, nil, 403)
}

func TestManualWorkerCandidateRecoveryPagesAcrossHistoryForExactDigest(t *testing.T) {
	f, _, request := candidateTransferFixture(t)
	registration := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": request.Assignment.WorkerID, "profiles": []any{}}), 201)
	token, id := registration["token"].(string), registration["id"].(string)
	binding := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, request), 201))
	originalID := binding.TransferID
	binding.TransferID = strings.Repeat("f", 32)
	if _, err := f.s.DB.Exec(`UPDATE suma_candidate_transfers SET id=?,binding=? WHERE id=?`, binding.TransferID, stringify(binding), originalID); err != nil {
		t.Fatal(err)
	}
	const insert = `INSERT INTO suma_candidate_transfers(id,tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,subject_hash,binding,storage_path,created_at)
		SELECT ?,tenant_id,project_id,run_id,node_id,?,fence,worker_id,?,?,storage_path,created_at
		FROM suma_candidate_transfers WHERE id=?`
	for i := 1; i <= 20; i++ {
		historic := binding
		historic.TransferID = fmt.Sprintf("%032x", i)
		historic.Source.Attempt += i
		historic.State = "sealed"
		if _, err := f.s.DB.Exec(insert, historic.TransferID, historic.Source.Attempt, digest(token), stringify(historic), binding.TransferID); err != nil {
			t.Fatal(err)
		}
	}
	foreign := binding
	foreign.TransferID = fmt.Sprintf("%032x", 21)
	foreign.Source.Attempt += 21
	if _, err := f.s.DB.Exec(insert, foreign.TransferID, foreign.Source.Attempt, strings.Repeat("a", 64), stringify(foreign), binding.TransferID); err != nil {
		t.Fatal(err)
	}
	path := "/release-workers/" + id + "/candidate-transfers?project_id=" + f.project
	first := f.call("GET", path, nil, 200)
	if len(first["items"].([]any)) != 0 || first["next_cursor"] != fmt.Sprintf("%032x", 20) {
		t.Fatalf("filtered page lost the bounded scan cursor: %+v", first)
	}
	second := f.call("GET", path+"&cursor="+first["next_cursor"].(string), nil, 200)
	items := second["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["transfer_id"] != binding.TransferID || second["next_cursor"] != "" {
		t.Fatalf("expected only the current digest's unfinished transfer: %+v", second)
	}
	f.call("GET", path+"&cursor=", nil, 400)
	f.call("GET", path+"&cursor="+first["next_cursor"].(string)+"&unknown=value", nil, 400)
}
