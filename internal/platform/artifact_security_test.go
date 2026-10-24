package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func artifactProtocolFixture(t *testing.T) (*fixture, string, rc.AssignmentRef, string, rc.ArtifactUploadRequest) {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	hash := strings.Repeat("a", 64)
	worker, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, map[string]any{"worker_id": "bounded-builder", "profiles": []ReleaseProfileRegistration{{ID: "site-build", Kind: "build", SHA256: hash}}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	token := worker["token"].(string)
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Upload protocol only", Nodes: []dag.Node{{ID: "build", Type: "command", Command: []string{"site-build"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "bounded-builder", toolFixtureCapabilities(t, f))
	if err != nil || a == nil {
		t.Fatal(err)
	}
	ref := rc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	body := "actual immutable bytes\n"
	m := rc.ArtifactManifest{SchemaVersion: 1, Kind: rc.StaticSiteKind, Files: []rc.ArtifactFile{{Path: "index.html", SizeBytes: int64(len(body)), SHA256: digest(body)}}}
	return f, token, ref, body, rc.ArtifactUploadRequest{ProjectID: f.project, Assignment: ref, OperationID: newID(), ProfileID: "site-build", ProfileSHA256: hash, Manifest: m}
}
func artifactRawPut(f *fixture, token, upload, hash, op string, b []byte, want int) {
	f.t.Helper()
	req := httptest.NewRequest("PUT", "http://suma.test/api/v1/runner/artifact-uploads/"+upload+"/blobs/"+hash+"?project_id="+f.project, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Suma-Operation-ID", op)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != want {
		f.t.Fatalf("actual PUT %d want %d: %s", w.Code, want, w.Body.String())
	}
}
func TestArtifactUploadAuthenticatesBytesAndSealNeverMeansBuildPassed(t *testing.T) {
	f, token, ref, body, request := artifactProtocolFixture(t)
	ctx := context.Background()
	shared := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	var payload map[string]any
	raw, _ := json.Marshal(request)
	json.Unmarshal(raw, &payload)
	f.bearer("POST", "/runner/artifact-uploads", shared, payload, 403)
	start := f.bearer("POST", "/runner/artifact-uploads", token, payload, 201)
	upload := start["upload_id"].(string)
	hash := digest(body)
	seal := rc.ArtifactSealRequest{ProjectID: f.project, Assignment: ref, OperationID: newID()}
	raw, _ = json.Marshal(seal)
	var sealMap map[string]any
	json.Unmarshal(raw, &sealMap)
	f.bearer("POST", "/runner/artifact-uploads/"+upload+"/seal", token, sealMap, 409)
	artifactRawPut(f, token, upload, hash, newID(), []byte("wrong bytes"), 409)
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_artifact_blobs`).Scan(&count); err != nil || count != 0 {
		t.Fatal("bad bytes became stored", err)
	}
	operation := newID()
	artifactRawPut(f, token, upload, hash, operation, []byte(body), 200)
	artifactRawPut(f, token, upload, hash, operation, []byte(body), 200)
	ack := f.bearer("POST", "/runner/artifact-uploads/"+upload+"/seal", token, sealMap, 200)
	artifact := ack["artifact_id"].(string)
	again := f.bearer("POST", "/runner/artifact-uploads/"+upload+"/seal", token, sealMap, 200)
	if again["artifact_id"] != artifact {
		t.Fatal("seal replay created another artifact")
	}
	stored, err := f.s.ReleaseArtifact(ctx, f.user, f.project, artifact)
	if err != nil || stored.State != "sealed" || len(stored.Proofs) != 0 {
		t.Fatalf("seal fabricated proof: %+v %v", stored, err)
	}
	_, err = f.s.CreateReleaseCandidate(ctx, f.user, f.project, map[string]any{"name": "不能以封存冒充验收", "artifact_id": artifact, "build_attestation_id": "missing", "package_test_attestation_id": "missing", "tasks": []any{map[string]any{"id": "missing", "expected_version": 1}}, "idempotency_key": newID()})
	if err == nil {
		t.Fatal("candidate created without actual process proof")
	}
	wrong := request
	wrong.OperationID = request.OperationID
	wrong.Manifest.Files = append([]rc.ArtifactFile(nil), request.Manifest.Files...)
	wrong.Manifest.Files[0].Path = "other.html"
	_, err = f.s.StartArtifactUpload(ctx, f.user, f.project, token, wrong)
	releaseWantStatus(t, err, 409)
	other := f.call("POST", "/projects", map[string]any{"title": "其他范围", "idempotency_key": newID()}, 201)
	f.call("GET", "/artifacts/"+artifact+"?project_id="+other["id"].(string), nil, 404)
	// A scoped credential cannot impersonate a different worker or be impersonated.
	f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": "other-worker", "capabilities": []string{"command"}}, 403)
	f.bearer("POST", "/runner/claim", shared, map[string]any{"project_id": f.project, "worker_id": ref.WorkerID, "capabilities": []string{"command"}}, 403)
}

func TestArtifactRawUploadOverJSONLimitAndInvalidatedLease(t *testing.T) {
	f, token, ref, _, r := artifactProtocolFixture(t)
	ctx := context.Background()
	b := bytes.Repeat([]byte("z"), (2<<20)+17)
	r.Manifest.Files[0].SizeBytes = int64(len(b))
	r.Manifest.Files[0].SHA256 = digest(string(b))
	start, err := f.s.StartArtifactUpload(ctx, f.user, f.project, token, r)
	if err != nil {
		t.Fatal(err)
	}
	artifactRawPut(f, token, start.UploadID, r.Manifest.Files[0].SHA256, newID(), b, 200)
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, ref.RunID, ref.NodeID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.SealArtifactUpload(ctx, f.user, f.project, token, start.UploadID, rc.ArtifactSealRequest{ProjectID: f.project, Assignment: ref, OperationID: newID()})
	releaseWantStatus(t, err, 409)
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_release_artifacts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired worker sealed new content", err)
	}
	entries, err := os.ReadDir(filepath.Join(f.s.DataDir, "release-artifacts", "blobs"))
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected spooled file debris", err)
	}
}

func TestReleaseWorkerCredentialOnceOnlyAndRevocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	body := map[string]any{"worker_id": "revocable-worker", "profiles": []any{}, "deployment": true, "idempotency_key": newID()}
	created, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, body)
	if err != nil {
		t.Fatal(err)
	}
	secret := created["token"].(string)
	replay, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, body)
	if err != nil || replay["token"] != nil {
		t.Fatal("secret repeated in receipt", err)
	}
	if _, err = f.s.Authenticate(ctx, secret); err != nil {
		t.Fatal(err)
	}
	revoke := map[string]any{"status": "revoked", "expected_version": 1, "idempotency_key": newID()}
	_, err = f.s.RevokeReleaseWorker(ctx, f.user, f.project, created["id"].(string), f.cookie.Value, revoke)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Authenticate(ctx, secret); err == nil {
		t.Fatal("revoked worker still authenticated")
	}
	_, err = f.s.RevokeReleaseWorker(ctx, f.user, f.project, created["id"].(string), f.cookie.Value, revoke)
	if err != nil {
		t.Fatal("lost revoke ACK not recoverable", err)
	}
	body["idempotency_key"] = newID()
	_, err = f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, body)
	releaseWantStatus(t, err, 409)
}

func TestReleaseProviderConfigurationReferencesSecretsAndFixedAudience(t *testing.T) {
	secret := strings.Repeat("s", 40)
	reconcile := strings.Repeat("r", 40)
	vars := map[string]string{"PROVIDER_SECRET": secret, "READ_SECRET": reconcile}
	base := `[{"id":"provider","tenant_id":"tenant","project_id":"project","site_ids":["preview"],"token_env":"PROVIDER_SECRET","origin":"http://127.0.0.1:4444","reconcile_token_env":"READ_SECRET"}]`
	providers, err := LoadReleaseProviders(base, func(k string) string { return vars[k] })
	if err != nil || len(providers) != 1 || providers[0].TokenHash != digest(secret) {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(providers)
	if strings.Contains(string(wire), secret) || strings.Contains(string(wire), reconcile) || strings.Contains(string(wire), digest(secret)) {
		t.Fatal("provider config exposed secrets")
	}
	for _, raw := range []string{strings.Replace(base, "http://127.0.0.1:4444", "http://remote.invalid", 1), strings.Replace(base, "http://127.0.0.1:4444", "https://remote.invalid", 1), strings.Replace(base, "127.0.0.1", "localhost", 1), strings.Replace(base, "http://127.0.0.1", "https://127.0.0.1", 1), strings.Replace(base, "READ_SECRET", "MISSING", 1), strings.Replace(base, `"id":"provider"`, `"id":"provider","id":"other"`, 1), strings.Replace(base, `"site_ids":["preview"]`, `"site_ids":["preview","preview"]`, 1)} {
		if _, err = LoadReleaseProviders(raw, func(k string) string { return vars[k] }); err == nil {
			t.Fatal("invalid provider configuration accepted")
		}
	}
}
