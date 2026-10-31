package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func codexSkillsDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func codexSkillsBundle(t *testing.T) *cc.ExecutionSkillBundle {
	t.Helper()
	doc := cc.ExecutionSkillDocument{ID: "skill-173", Revision: 3, SHA256: strings.Repeat("d", 64), Title: "Reviewed skill", Description: "Review the actual changes", Content: "Use the independent tests and inspect the patch."}
	doc.Name = "suma-" + codexSkillsDigest([]byte(doc.ID))[:24]
	doc.ContentSHA256 = codexSkillsDigest([]byte(doc.Content))
	raw, err := cc.RenderExecutionSkill(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc.DocumentSHA256 = codexSkillsDigest(raw)
	bundle := &cc.ExecutionSkillBundle{Protocol: "suma-codex-skills-v1", Documents: []cc.ExecutionSkillDocument{doc}}
	raw, _ = json.Marshal(bundle)
	bundle.BundleSHA256 = codexSkillsDigest(raw)
	return bundle
}

func codexSkillsDriver(t *testing.T, withSkills bool) (*codexAppServer, *cc.ExecutionSkillBundle, *cc.ExecutionSkillEvidence) {
	t.Helper()
	profile := tc.Profile{CommandID: "codex173", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "protocol-fixture"}
	spec := CommandSpec{CodexAppServer: &CodexAppServerSpec{ProviderID: "owned173"}}
	d := newCodexAppServer(profile, spec, t.TempDir(), t.TempDir(), "Implement the fixed objective.", &cc.Binding{Model: "owned-model-173", ReasoningEffort: "high"}, newToolStream(profile, nil, nil))
	if !withSkills {
		return d, nil, nil
	}
	bundle := codexSkillsBundle(t)
	evidence := &cc.ExecutionSkillEvidence{BundleSHA256: bundle.BundleSHA256, Materialized: true, Documents: []cc.ExecutionSkillReceipt{{ID: bundle.Documents[0].ID, Revision: 3, SHA256: bundle.Documents[0].SHA256, Name: bundle.Documents[0].Name, DocumentSHA256: bundle.Documents[0].DocumentSHA256}}}
	if err := d.configureSkills(bundle, evidence); err != nil {
		t.Fatal(err)
	}
	return d, bundle, evidence
}

func codexSkillsDrain(t *testing.T, d *codexAppServer) []map[string]any {
	t.Helper()
	d.input.mu.Lock()
	raw := append([]byte(nil), d.input.pending...)
	d.input.pending = nil
	d.input.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		value, err := decodeToolObject([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, value)
	}
	return out
}

func codexSkillsThreadACK(t *testing.T, d *codexAppServer) {
	t.Helper()
	codexSkillsDrain(t, d)
	d.waiting, d.waitingID = "thread/start", "3"
	d.requestID = 3
	result := map[string]any{"thread": map[string]any{"id": "thread-173"}, "model": "owned-model-173", "modelProvider": "owned173", "reasoningEffort": "high", "cwd": d.root, "approvalPolicy": "never", "sandbox": map[string]any{"type": "workspaceWrite", "writableRoots": []string{d.root}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}}
	codexSkillsReply(t, d, "3", result)
}

func codexSkillsReply(t *testing.T, d *codexAppServer, id string, result any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"id": json.Number(id), "result": result})
	_, _ = d.Write(append(raw, '\n'))
}

func codexSkillsCatalogue(d *codexAppServer, bundle *cc.ExecutionSkillBundle) map[string]any {
	doc := bundle.Documents[0]
	return map[string]any{"data": []any{map[string]any{"cwd": d.root, "skills": []any{map[string]any{"name": doc.Name, "path": filepath.Join(d.home, "skills", doc.Name, "SKILL.md"), "enabled": true}}, "errors": []any{}}}}
}

func TestCodexSkillsDirectoryBeforeSingleExplicitInvocation(t *testing.T) {
	d, bundle, evidence := codexSkillsDriver(t, true)
	codexSkillsThreadACK(t, d)
	requests := codexSkillsDrain(t, d)
	if len(requests) != 1 || textAt(requests[0], "method") != "skills/list" || evidence.CatalogAcknowledged || evidence.InvocationRequested {
		t.Fatalf("task escaped directory gate: %#v / %+v", requests, evidence)
	}
	p := objectAt(requests[0], "params")
	wantParams := map[string]any{"cwds": []any{d.root}, "forceReload": true, "perCwdExtraUserRoots": []any{map[string]any{"cwd": d.root, "extraUserRoots": []any{filepath.Join(d.home, "skills")}}}}
	if !reflect.DeepEqual(p, wantParams) {
		t.Fatalf("directory did not use the owned cwd/home: %#v", p)
	}
	catalogue := codexSkillsCatalogue(d, bundle)
	row := catalogue["data"].([]any)[0].(map[string]any)
	row["skills"] = append(row["skills"].([]any), map[string]any{"name": "unapproved-local-skill", "path": filepath.Join(d.root, "local", "SKILL.md"), "enabled": false})
	codexSkillsReply(t, d, "4", catalogue)
	requests = codexSkillsDrain(t, d)
	if len(requests) != 1 || textAt(requests[0], "method") != "turn/start" || !evidence.CatalogAcknowledged || !evidence.InvocationRequested || evidence.TurnAcknowledged || evidence.FilesVerified {
		t.Fatalf("incorrect independent facts: %#v / %+v", requests, evidence)
	}
	input, ok := objectAt(requests[0], "params")["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected one text and one explicit skill input: %#v", input)
	}
	text, skill := input[0].(map[string]any), input[1].(map[string]any)
	if textAt(text, "type") != "text" || !strings.Contains(textAt(text, "text"), "$"+bundle.Documents[0].Name) || strings.Contains(textAt(text, "text"), bundle.Documents[0].Content) || textAt(skill, "type") != "skill" || textAt(skill, "name") != bundle.Documents[0].Name || textAt(skill, "path") != filepath.Join(d.home, "skills", bundle.Documents[0].Name, "SKILL.md") {
		t.Fatalf("skill identity or nonduplicated text differs: %#v", input)
	}
	codexSkillsReply(t, d, "5", map[string]any{"turn": map[string]any{"id": "turn-173", "status": "inProgress"}})
	if !evidence.TurnAcknowledged || evidence.FilesVerified || len(codexSkillsDrain(t, d)) != 0 {
		t.Fatal("turn acknowledgement was lost or invented file verification")
	}
	// Replayed directory ACK cannot start a second turn.
	codexSkillsReply(t, d, "4", codexSkillsCatalogue(d, bundle))
	if d.bad == nil || len(codexSkillsDrain(t, d)) != 0 {
		t.Fatal("repeated catalogue response created new work")
	}
}

func TestCodexSkillsInvalidDirectoryNeverSendsPrompt(t *testing.T) {
	for _, mode := range []string{"missing", "duplicate_name", "duplicate_path", "wrong_path", "wrong_cwd", "disabled", "missing_enabled", "read_error", "missing_errors", "extra_cwd", "rejected", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			d, bundle, evidence := codexSkillsDriver(t, true)
			codexSkillsThreadACK(t, d)
			codexSkillsDrain(t, d)
			catalogue := codexSkillsCatalogue(d, bundle)
			data := catalogue["data"].([]any)
			row := data[0].(map[string]any)
			skill := row["skills"].([]any)[0].(map[string]any)
			switch mode {
			case "missing":
				row["skills"] = []any{}
			case "duplicate_name":
				row["skills"] = []any{skill, skill}
			case "duplicate_path":
				row["skills"] = []any{skill, map[string]any{"name": "unapproved", "path": skill["path"], "enabled": true}}
			case "wrong_path":
				skill["path"] = filepath.Join(d.root, "SKILL.md")
			case "wrong_cwd":
				row["cwd"] = d.home
			case "disabled":
				skill["enabled"] = false
			case "missing_enabled":
				delete(skill, "enabled")
			case "read_error":
				row["errors"] = []any{map[string]any{"path": "private-path", "message": "private provider text"}}
			case "missing_errors":
				delete(row, "errors")
			case "extra_cwd":
				catalogue["data"] = append(data, row)
			case "malformed":
				row["skills"] = "not an array"
			}
			if mode == "rejected" {
				_, _ = d.Write([]byte("{\"id\":4,\"error\":{\"code\":-32601,\"message\":\"unsupported\"}}\n"))
			} else {
				codexSkillsReply(t, d, "4", catalogue)
			}
			if d.bad == nil || evidence.CatalogAcknowledged || evidence.InvocationRequested || evidence.TurnAcknowledged || evidence.FailureCode == "" || len(codexSkillsDrain(t, d)) != 0 {
				t.Fatalf("unsafe catalogue reached task or lost safe failure: %+v / %v", evidence, d.bad)
			}
			if strings.Contains(d.bad.Error(), "private") {
				t.Fatal("directory diagnostics leaked provider fields")
			}
		})
	}
}

func TestCodexSkillsLifecycleKeepsOriginalOperation(t *testing.T) {
	t.Run("cancel_before_directory_reply", func(t *testing.T) {
		d, bundle, evidence := codexSkillsDriver(t, true)
		codexSkillsThreadACK(t, d)
		codexSkillsDrain(t, d)
		if err := d.Cancel(); err != nil {
			t.Fatal(err)
		}
		codexSkillsReply(t, d, "4", codexSkillsCatalogue(d, bundle))
		if evidence.InvocationRequested || evidence.TurnAcknowledged || len(codexSkillsDrain(t, d)) != 0 {
			t.Fatal("late directory ACK launched cancelled work")
		}
	})
	t.Run("configuration_is_immutable", func(t *testing.T) {
		d, bundle, evidence := codexSkillsDriver(t, true)
		if d.configureSkills(bundle, evidence) == nil {
			t.Fatal("reconfiguration replaced a fixed attempt")
		}
		bundle.Documents[0].Name = "caller-mutated"
		codexSkillsThreadACK(t, d)
		codexSkillsDrain(t, d)
		catalogue := map[string]any{"data": []any{map[string]any{"cwd": d.root, "skills": []any{map[string]any{"name": evidence.Documents[0].Name, "path": filepath.Join(d.home, "skills", evidence.Documents[0].Name, "SKILL.md"), "enabled": true}}, "errors": []any{}}}}
		codexSkillsReply(t, d, "4", catalogue)
		if d.bad != nil || !evidence.InvocationRequested {
			t.Fatalf("caller mutated owned bundle: %v", d.bad)
		}
	})
	t.Run("unmaterialized_or_foreign_receipt", func(t *testing.T) {
		for _, foreign := range []bool{false, true} {
			d, _, _ := codexSkillsDriver(t, false)
			bundle := codexSkillsBundle(t)
			evidence := &cc.ExecutionSkillEvidence{BundleSHA256: bundle.BundleSHA256, Materialized: false}
			if foreign {
				evidence.Materialized = true
				evidence.BundleSHA256 = strings.Repeat("e", 64)
			}
			if d.configureSkills(bundle, evidence) == nil {
				t.Fatal("unowned materialization accepted")
			}
		}
	})
	t.Run("no_skills_keeps_original_rpc", func(t *testing.T) {
		d, _, _ := codexSkillsDriver(t, false)
		if err := d.configureSkills(nil, nil); err != nil {
			t.Fatal(err)
		}
		codexSkillsThreadACK(t, d)
		requests := codexSkillsDrain(t, d)
		if len(requests) != 1 || textAt(requests[0], "method") != "turn/start" || d.evidence.Skills != nil {
			t.Fatal("ordinary task acquired skill protocol")
		}
	})
}
