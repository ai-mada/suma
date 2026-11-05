package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func TestManualWorkerUploadRecoveryRetainsEvidenceAndUnblocksRotation(t *testing.T) {
	f, token, ref, body, request := artifactProtocolFixture(t)
	ctx := context.Background()
	actor := domainCActor(t, f.s, token)
	worker := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)
	id := worker["id"].(string)
	path := "/release-workers/" + id + "/artifact-uploads"
	start, err := f.s.StartArtifactUpload(ctx, actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	hash, putKey := digest(body), newID()
	artifactRawPut(f, token, start.UploadID, hash, putKey, []byte(body), 200)

	abandon := path + "/" + start.UploadID + "/abandon"
	command := f.body(map[string]any{"expected_worker_version": 1, "expected_manifest_sha256": start.ManifestSHA256, "acknowledge_incomplete": true})
	item := f.call("GET", path+"?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)
	if item["attempt_state"] != "running" || item["disposition"] != "awaiting_attempt" || item["upload_id"] != start.UploadID {
		t.Fatalf("running upload was not accurately represented: %+v", item)
	}
	if refusal := f.call("POST", abandon, command, 409)["error"].(map[string]any); refusal["code"] != "artifact_upload_attempt_unresolved" {
		t.Fatal("running attempt did not return the scoped unresolved result")
	}
	f.call("POST", manualRotationPath(id), f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true}), 409)

	result := dag.Result{
		RunID: ref.RunID, NodeID: ref.NodeID, Attempt: ref.Attempt, Fence: ref.Fence, WorkerID: ref.WorkerID,
		ExitCode: 1, Error: "Fixture build failed after uploading", OperationID: newID(),
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal(err)
	}
	item = f.call("GET", path+"?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)
	if item["attempt_state"] != "failed" || item["disposition"] != "eligible" || item["manifest_sha256"] != start.ManifestSHA256 {
		t.Fatalf("resolved attempt did not retain the manifest: %+v", item)
	}
	f.call("POST", abandon, f.body(map[string]any{"expected_worker_version": 2, "expected_manifest_sha256": start.ManifestSHA256, "acknowledge_incomplete": true}), 409)
	f.call("POST", abandon, f.body(map[string]any{"expected_worker_version": 1, "expected_manifest_sha256": strings.Repeat("a", 64), "acknowledge_incomplete": true}), 409)

	if _, err = f.s.DB.Exec(`CREATE TRIGGER fail_upload_abandon_activity BEFORE INSERT ON suma_activity
		WHEN NEW.kind='release_worker.artifact_upload_abandoned' BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	f.call("POST", abandon, command, 500)
	if _, err = f.s.DB.Exec(`DROP TRIGGER fail_upload_abandon_activity`); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = f.s.DB.QueryRow(`SELECT state FROM suma_artifact_uploads WHERE id=?`, start.UploadID).Scan(&state); err != nil || state != "receiving" {
		t.Fatalf("failed activity did not roll back the upload: %s %v", state, err)
	}
	receipt := f.call("POST", abandon, command, 200)
	if receipt["state"] != "abandoned" || receipt["operation_id"] != command["idempotency_key"] || receipt["upload_id"] != start.UploadID {
		t.Fatalf("invalid abandonment receipt: %+v", receipt)
	}

	var manifest, artifact, blobPath string
	var size int64
	if err = f.s.DB.QueryRow(`SELECT state,manifest_sha256,artifact_id,size_bytes FROM suma_artifact_uploads WHERE id=?`, start.UploadID).Scan(&state, &manifest, &artifact, &size); err != nil ||
		state != "abandoned" || manifest != start.ManifestSHA256 || artifact != "" || size != int64(len(body)) {
		t.Fatalf("end-upload altered retained evidence: %s %s %q %d %v", state, manifest, artifact, size, err)
	}
	if err = f.s.DB.QueryRow(`SELECT storage_path FROM suma_artifact_blobs WHERE upload_id=? AND digest=?`, start.UploadID, hash).Scan(&blobPath); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(filepath.Join(f.s.DataDir, "release-artifacts", filepath.FromSlash(blobPath)))
	if err != nil || !bytes.Equal(blob, []byte(body)) {
		t.Fatal("retained bytes changed", err)
	}
	if domainACount(t, f.s, "suma_release_artifacts") != 0 {
		t.Fatal("abandonment published a release artifact")
	}
	if err = f.s.DB.QueryRow(`SELECT state FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, ref.RunID, ref.NodeID, ref.Attempt).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("end-upload changed original result: %s %v", state, err)
	}

	artifactRawPut(f, token, start.UploadID, hash, putKey, []byte(body), 200)
	artifactRawPut(f, token, start.UploadID, hash, newID(), []byte(body), 409)
	_, err = f.s.SealArtifactUpload(ctx, actor, f.project, start.UploadID, rc.ArtifactSealRequest{ProjectID: f.project, Assignment: ref, OperationID: newID()})
	releaseWantStatus(t, err, 409)

	expireManualArtifactWorker(t, f, token)
	if items := f.call("GET", path+"?project_id="+f.project, nil, 200)["items"].([]any); len(items) != 0 {
		t.Fatal("ended upload remained in the active recovery list")
	}
	f.call("POST", manualRotationPath(id), f.body(map[string]any{"expected_version": 1, "acknowledge_stopped": true}), 200)
	if replay := f.call("POST", abandon, command, 200); !reflect.DeepEqual(receipt, replay) {
		t.Fatal("exact replay changed after credential rotation")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call("POST", abandon, command, 403)
	f.call("GET", path+"?project_id="+f.project, nil, 403)
}

func TestManualWorkerUploadRecoveryRejectsUnverifiedAttemptsAndRequests(t *testing.T) {
	f, token, ref, _, request := artifactProtocolFixture(t)
	start, err := f.s.StartArtifactUpload(context.Background(), domainCActor(t, f.s, token), f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	id := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)["id"].(string)
	path := "/release-workers/" + id + "/artifact-uploads"
	abandon := path + "/" + start.UploadID + "/abandon"
	command := f.body(map[string]any{"expected_worker_version": 1, "expected_manifest_sha256": start.ManifestSHA256, "acknowledge_incomplete": true})
	f.call("GET", path+"?project_id="+f.project+"&project_id="+f.project, nil, 400)
	f.call("GET", path+"?project_id="+f.project+"&unknown=x", nil, 400)
	f.call("GET", path+"?project_id="+f.project+"&cursor=", nil, 400)
	f.bearer("POST", abandon, token, command, 403)
	other := f.call("POST", "/projects", map[string]any{"title": "Unrelated project", "idempotency_key": newID()}, 201)["id"].(string)
	f.call("GET", path+"?project_id="+other, nil, 403)

	req := httptest.NewRequest("POST", "http://suma.test/api/v1"+abandon,
		strings.NewReader(strings.Replace(stringify(command), "\"expected_worker_version\":1", "\"expected_worker_version\":1.0000000000000000001", 1)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Expected-User", f.user.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("non-integer CAS proof reached the command: %d %s", w.Code, w.Body.String())
	}

	cases := []struct {
		state, raw, want string
	}{
		{"reconciling", "", "awaiting_attempt"},
		{"failed", "{}", "unverified"},
		{"failed", stringify(dag.Result{RunID: ref.RunID, NodeID: ref.NodeID, Attempt: ref.Attempt, Fence: ref.Fence, WorkerID: ref.WorkerID, OutcomeUnknown: true, OperationID: newID()}), "unverified"},
		{"failed", stringify(dag.Result{RunID: ref.RunID, NodeID: ref.NodeID, Attempt: ref.Attempt, Fence: ref.Fence + 1, WorkerID: ref.WorkerID, OperationID: newID()}), "unverified"},
	}
	for _, tc := range cases {
		if _, err = f.s.DB.Exec(`UPDATE dag_attempts SET state=?,result=?,completed_at=? WHERE run_id=? AND node_id=? AND attempt=?`, tc.state, tc.raw, now(), ref.RunID, ref.NodeID, ref.Attempt); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='failed',lease_until=0 WHERE run_id=? AND id=?`, ref.RunID, ref.NodeID); err != nil {
			t.Fatal(err)
		}
		item := f.call("GET", path+"?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)
		if item["disposition"] != tc.want {
			t.Fatalf("%s returned %v, want %s", tc.state, item["disposition"], tc.want)
		}
		f.call("POST", abandon, command, 409)
	}
	known := dag.Result{RunID: ref.RunID, NodeID: ref.NodeID, Attempt: ref.Attempt, Fence: ref.Fence, WorkerID: ref.WorkerID, ExitCode: 1, Error: "Fixture terminal failure", OperationID: newID()}
	if _, err = f.s.DB.Exec("UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?", stringify(known), ref.RunID, ref.NodeID, ref.Attempt); err != nil {
		t.Fatal(err)
	}
	expireManualArtifactWorker(t, f, token)
	if expired := f.call("GET", path+"?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any); expired["disposition"] != "eligible" {
		t.Fatalf("credential expiry hid proven terminal evidence: %+v", expired)
	}
	f.call("POST", abandon, command, 200)
}

func expireManualArtifactWorker(t *testing.T, f *fixture, token string) {
	t.Helper()
	var err error
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	for _, query := range []string{
		`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`,
		`UPDATE suma_credential_slots SET expires_at=? WHERE current_digest=?`,
		`UPDATE suma_credential_grants SET expires_at=? WHERE id IN (SELECT grant_id FROM suma_credential_authority WHERE token_hash=?)`,
		`UPDATE suma_release_workers SET expires_at=? WHERE token_hash=?`,
	} {
		if _, err = f.s.DB.Exec(query, past, digest(token)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManualWorkerUploadRecoveryPagesOnlyTheExactCurrentDigest(t *testing.T) {
	f, token, _, _, request := artifactProtocolFixture(t)
	start, err := f.s.StartArtifactUpload(context.Background(), domainCActor(t, f.s, token), f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	const seed = `INSERT INTO suma_artifact_uploads(id,tenant_id,project_id,worker_id,token_hash,run_id,node_id,attempt,fence,profile_id,profile_sha256,manifest_sha256,manifest,size_bytes,state,artifact_id,created_at)
		SELECT ?,tenant_id,project_id,worker_id,?,run_id,node_id,attempt+?,fence,profile_id,profile_sha256,manifest_sha256,manifest,size_bytes,state,artifact_id,created_at
		FROM suma_artifact_uploads WHERE id=?`
	for i := 1; i <= 21; i++ {
		if _, err = f.s.DB.Exec(seed, fmt.Sprintf("%032x", i), digest(token), i, start.UploadID); err != nil {
			t.Fatal(err)
		}
	}
	excluded := strings.Repeat("0", 32)
	if _, err = f.s.DB.Exec(seed, excluded, strings.Repeat("1", 64), 22, start.UploadID); err != nil {
		t.Fatal(err)
	}
	id := f.call("GET", "/release-workers?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)["id"].(string)
	path := "/release-workers/" + id + "/artifact-uploads?project_id=" + f.project
	first := f.call("GET", path, nil, 200)
	if len(first["items"].([]any)) != 20 || first["next_cursor"] == "" {
		t.Fatalf("expected a bounded first page, got %d", len(first["items"].([]any)))
	}
	second := f.call("GET", path+"&cursor="+first["next_cursor"].(string), nil, 200)
	if len(second["items"].([]any)) != 2 || second["next_cursor"] != "" {
		t.Fatalf("expected two remaining upload rows, got %d", len(second["items"].([]any)))
	}
	seen := map[string]bool{excluded: true}
	for _, page := range []map[string]any{first, second} {
		for _, raw := range page["items"].([]any) {
			item := raw.(map[string]any)
			id := item["upload_id"].(string)
			if seen[id] || item["disposition"] == "eligible" {
				t.Fatalf("duplicate, foreign or unverified upload was misrepresented: %+v", item)
			}
			seen[id] = true
		}
	}
}
