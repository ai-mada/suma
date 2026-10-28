package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This is a transport/contract fixture, not an attestation that a package was
// built. The OS/HTTP release-set journey separately exercises real artifact
// sealing. In particular, a successful process without that proof stays gated.
func TestTestPlanMixedPackageEvidenceKeepsJUnitAndArtifactGates(t *testing.T) {
	for _, scenario := range []string{"mixed", "missing-junit", "unknown-test", "package-only"} {
		t.Run(scenario, func(t *testing.T) {
			h := newQualityPlanFixture(t, true)
			f, ctx := h.f, context.Background()
			buildDefinition := rc.CommandProfile{ID: "mixed-build", Kind: "build", Executable: "fixture-node", Args: []string{"build-fixture"}, ArtifactOutput: "dist"}
			packageDefinition := rc.CommandProfile{ID: "mixed-package", Kind: "package_test", Executable: "fixture-node", Args: []string{"package-fixture"}, TestEvidence: true, ArtifactInput: true}
			buildSHA, err := rc.ProfileDigest(buildDefinition)
			if err != nil {
				t.Fatal(err)
			}
			packageSHA, err := rc.ProfileDigest(packageDefinition)
			if err != nil {
				t.Fatal(err)
			}
			registered := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "mixed-worker", "profiles": []ReleaseProfileRegistration{{ID: buildDefinition.ID, Kind: "build", SHA256: buildSHA}, {ID: packageDefinition.ID, Kind: "package_test", SHA256: packageSHA}}, "deployment": false}), 201)
			token := registered["token"].(string)
			profiles := map[string]ToolProfile{}
			inventory := []tc.Observation{}
			for _, command := range []string{"mixed-build", "mixed-package", "mixed-unknown"} {
				definition := toolTestProfile()
				definition.CommandID, definition.Adapter, definition.Protocol = command, "command", "text-v1"
				definition.ProfileSHA256 = digest("fixed protocol fixture " + command)
				profile, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "mixed-worker", Name: command, Profile: definition, IdempotencyKey: newID()})
				if e != nil {
					t.Fatal(e)
				}
				profiles[command] = profile
				inventory = append(inventory, tc.Observation{Profile: definition, Available: true})
			}
			if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: "mixed-worker", OperationID: newID(), Items: inventory}); err != nil {
				t.Fatal(err)
			}
			for command, profile := range profiles {
				profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
				profiles[command] = profile
			}
			bind := func(profile ToolProfile) map[string]any {
				return map[string]any{"tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}
			}
			command := "mixed-package"
			if scenario == "unknown-test" {
				command = "mixed-unknown"
			}
			packageConfig := bind(profiles[command])
			packageConfig["artifact_from"] = "build"
			nodes := []any{map[string]any{"id": "build", "type": "command", "command": []string{"mixed-build"}, "config": bind(profiles["mixed-build"])}, map[string]any{"id": "package", "type": "test", "command": []string{command}, "config": packageConfig}}
			edges := []any{map[string]any{"from": "build", "to": "package"}}
			if scenario != "package-only" {
				nodes = append(nodes, map[string]any{"id": "junit", "type": "test", "command": []string{h.profile.Profile.CommandID}, "config": bind(h.profile)})
				edges = append(edges, map[string]any{"from": "build", "to": "junit"})
			}
			workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Independent evidence kinds", "nodes": nodes, "edges": edges}), 201)
			if scenario == "unknown-test" || scenario == "package-only" {
				f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": h.task.ID}), 400)
				return
			}
			run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": h.task.ID}), 201)
			runID := run["id"].(string)
			tx, err := f.s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			fixed, found, err := f.s.testPlanRunTx(ctx, tx, f.user.TenantID, f.project, runID)
			tx.Rollback()
			if err != nil || !found || len(fixed.PackageTests) != 1 || fixed.PackageTests[0].ToolProfileVersion != profiles[command].Version || fixed.PackageTests[0].ReleaseProfileSHA256 != packageSHA {
				t.Fatal("package classification was not frozen from current dual registration", fixed, err)
			}
			claim := func(worker, token, command string) dag.Assignment {
				body := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": worker, "capabilities": []string{"command:" + command}}), 200)
				var a dag.Assignment
				if err := json.Unmarshal([]byte(stringify(body["assignment"])), &a); err != nil || a.RunID != runID {
					t.Fatal("missing current assignment", body, err)
				}
				return a
			}
			for _, command := range []string{"mixed-build", "mixed-package"} {
				a := claim("mixed-worker", token, command)
				result := h.result(a, "")
				result.ToolEvidence.Profile = profiles[command].Profile
				result.WorkspaceEvidence["test_command"] = command == "mixed-package"
				f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
			}
			a := claim(h.profile.WorkerID, h.token, h.profile.Profile.CommandID)
			xml := `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`
			if scenario == "missing-junit" {
				xml = ""
			}
			f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(a, xml)}), 200)
			execution, err := f.s.GetTestPlanExecution(ctx, f.actor, f.project, h.plan.ID, runID)
			if err != nil || len(execution.Reports) != 1 || execution.Reports[0].NodeID != "junit" || execution.State == "passed" || execution.RunState != "succeeded" {
				t.Fatal("artifact test became a JUnit report or absent artifact proof became passing evidence", execution, err)
			}
			if scenario == "mixed" && (execution.Reports[0].Eligibility != "current" || execution.Passed != 1 || execution.SourceCommits[0] != strings.Repeat("a", 40)) {
				t.Fatal("independent valid JUnit facts were lost", execution)
			}
			if scenario == "missing-junit" && execution.Reports[0].Eligibility != "invalid" {
				t.Fatal("ordinary report node lost its missing-report gate", execution)
			}
		})
	}
}
