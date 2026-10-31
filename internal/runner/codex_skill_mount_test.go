package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func codexSkillBundleForMount(t *testing.T) *cc.ExecutionSkillBundle {
	t.Helper()
	bundle, err := cc.BuildExecutionSkillBundle([]cc.ExecutionSkillDocument{
		{ID: "skill-reviewed-one", Revision: 2, SHA256: strings.Repeat("a", 64), Title: "Review current project access", Description: "Use current authorization and exact evidence", Content: "Read the fixed task. Preserve independent tests and formal acceptance.\n中文正文保持原样。"},
		{ID: "skill-reviewed-two", Revision: 1, SHA256: strings.Repeat("b", 64), Title: "Preserve uncertain facts", Content: "A directory or turn acknowledgement does not prove skill use."},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func codexSkillMountHome(t *testing.T, bundle *cc.ExecutionSkillBundle) string {
	t.Helper()
	home := t.TempDir()
	t.Cleanup(func() {
		for _, d := range bundle.Documents {
			_ = os.Chmod(filepath.Join(home, "skills", d.Name, "SKILL.md"), 0600)
		}
	})
	return home
}

func TestCodexSkillMountExactBytesAndReuse(t *testing.T) {
	bundle := codexSkillBundleForMount(t)
	home := codexSkillMountHome(t, bundle)
	evidence, err := mountCodexSkills(home, bundle)
	if err != nil || evidence == nil || !evidence.Materialized || evidence.FilesVerified || evidence.CatalogAcknowledged || evidence.InvocationRequested || evidence.TurnAcknowledged || !cc.ExecutionSkillEvidenceMatchesBundle(*bundle, *evidence) {
		t.Fatal("host bytes invented another evidence phase", evidence, err)
	}
	before := map[string]os.FileInfo{}
	for _, d := range bundle.Documents {
		file := filepath.Join(home, "skills", d.Name, "SKILL.md")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		want, e := cc.RenderExecutionSkill(d)
		if e != nil || !bytes.Equal(raw, want) || digest(raw) != d.DocumentSHA256 {
			t.Fatal("materialized bytes differ from the fixed published revision", e)
		}
		before[file], e = os.Stat(file)
		if e != nil {
			t.Fatal(e)
		}
	}
	if err = verifyCodexSkillFiles(home, bundle); err != nil {
		t.Fatal(err)
	}
	if _, err = mountCodexSkills(home, bundle); err != nil {
		t.Fatal("same attempt exact bytes could not be verified", err)
	}
	for file, old := range before {
		now, e := os.Stat(file)
		if e != nil || !os.SameFile(old, now) || !old.ModTime().Equal(now.ModTime()) {
			t.Fatal("exact existing skill was overwritten", file, e)
		}
	}
	if strings.Contains(stringifySkillEvidence(t, evidence), bundle.Documents[0].Content) || strings.Contains(stringifySkillEvidence(t, evidence), home) {
		t.Fatal("public receipt leaked instructions or a local path")
	}
	missing := filepath.Join(home, "not-created")
	if value, e := mountCodexSkills(missing, nil); e != nil || value != nil {
		t.Fatal("no-skill execution changed semantics", value, e)
	}
	if e := verifyCodexSkillFiles(missing, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(missing); !os.IsNotExist(e) {
		t.Fatal("no-skill execution touched the filesystem")
	}
}

func stringifySkillEvidence(t *testing.T, evidence *cc.ExecutionSkillEvidence) string {
	t.Helper()
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCodexSkillMountRejectsUnsafeOrMismatchedEntries(t *testing.T) {
	for _, scenario := range []string{"home-file", "skills-file", "name-file", "document-directory", "wrong-existing-bytes", "home-link", "skills-link", "name-link", "document-link"} {
		t.Run(scenario, func(t *testing.T) {
			bundle := codexSkillBundleForMount(t)
			base := t.TempDir()
			home := filepath.Join(base, "codex-home")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			skills := filepath.Join(home, "skills")
			name := filepath.Join(skills, bundle.Documents[0].Name)
			file := filepath.Join(name, "SKILL.md")
			write := func(path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("original unique bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			mkdir := func(path string) {
				t.Helper()
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			link := func(target, path string) {
				t.Helper()
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("host cannot create this symbolic link: %v", err)
				}
			}
			switch scenario {
			case "home-file":
				if err := os.Remove(home); err != nil {
					t.Fatal(err)
				}
				write(home)
			case "skills-file":
				write(skills)
			case "name-file":
				mkdir(skills)
				write(name)
			case "document-directory":
				mkdir(file)
			case "wrong-existing-bytes":
				mkdir(name)
				write(file)
			case "home-link":
				if err := os.Remove(home); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(base, "private-target")
				mkdir(target)
				link(target, home)
			case "skills-link":
				target := filepath.Join(base, "private-target")
				mkdir(target)
				link(target, skills)
			case "name-link":
				mkdir(skills)
				target := filepath.Join(base, "private-target")
				mkdir(target)
				link(target, name)
			case "document-link":
				mkdir(name)
				target := filepath.Join(base, "private-target")
				raw, _ := cc.RenderExecutionSkill(bundle.Documents[0])
				if err := os.WriteFile(target, raw, 0600); err != nil {
					t.Fatal(err)
				}
				link(target, file)
			}
			if _, err := mountCodexSkills(home, bundle); err == nil {
				t.Fatal("unsafe or different bytes were accepted", scenario)
			}
			if scenario == "wrong-existing-bytes" {
				raw, err := os.ReadFile(file)
				if err != nil || string(raw) != "original unique bytes" {
					t.Fatal("an existing file was overwritten", err)
				}
			}
			if err := verifyCodexSkillFiles(home, bundle); err == nil {
				t.Fatal("unsafe or missing tree was accepted on recheck", scenario)
			}
		})
	}
}

func TestCodexSkillFilesDetectTamperingAndInvalidBundleBeforeWrite(t *testing.T) {
	for _, scenario := range []string{"changed", "removed", "directory", "bundle-digest", "unsafe-name"} {
		t.Run(scenario, func(t *testing.T) {
			bundle := codexSkillBundleForMount(t)
			home := codexSkillMountHome(t, bundle)
			if scenario == "bundle-digest" || scenario == "unsafe-name" {
				if scenario == "bundle-digest" {
					bundle.BundleSHA256 = strings.Repeat("f", 64)
				} else {
					bundle.Documents[0].Name = "../outside"
					bundle.BundleSHA256 = cc.ExecutionSkillBundleDigest(*bundle)
				}
				if _, err := mountCodexSkills(home, bundle); err == nil {
					t.Fatal("unvalidated bundle reached materialization")
				}
				if _, err := os.Stat(filepath.Join(home, "skills")); !os.IsNotExist(err) {
					t.Fatal("invalid bundle wrote a skill tree")
				}
				return
			}
			if _, err := mountCodexSkills(home, bundle); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(home, "skills", bundle.Documents[0].Name, "SKILL.md")
			if err := os.Chmod(file, 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "changed" {
				if err := os.WriteFile(file, []byte("changed after protocol execution"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if scenario == "directory" {
					if err := os.Mkdir(file, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := verifyCodexSkillFiles(home, bundle); err == nil {
				t.Fatal("post-execution tampering was accepted")
			}
		})
	}
}

func TestCodexSkillAssignmentNeedsExactProtectedBundle(t *testing.T) {
	bundle := codexSkillBundleForMount(t)
	valid := func() Assignment {
		return Assignment{Type: "agent", ExecutionSkills: bundle, Config: map[string]any{"execution_skills": bundle}}
	}
	copy, err := codexExecutionSkillsRequest(valid(), &cc.Binding{})
	if err != nil || copy == bundle || copy.BundleSHA256 != bundle.BundleSHA256 {
		t.Fatal("protected bundle was not validated and frozen", copy, err)
	}
	copy.Documents[0].Content = "caller mutation after validation"
	if copy.Documents[0].Content == bundle.Documents[0].Content {
		t.Fatal("returned bundle aliases the caller")
	}
	for _, scenario := range []string{"workflow-only", "config-null", "assignment-only", "non-codex", "not-agent", "mismatched-config", "unknown-config-field"} {
		t.Run(scenario, func(t *testing.T) {
			a := valid()
			binding := &cc.Binding{}
			switch scenario {
			case "workflow-only":
				a.ExecutionSkills = nil
			case "config-null":
				a.Config["execution_skills"] = nil
			case "assignment-only":
				delete(a.Config, "execution_skills")
			case "non-codex":
				binding = nil
			case "not-agent":
				a.Type = "test"
			case "mismatched-config":
				other := *bundle
				other.Documents = append([]cc.ExecutionSkillDocument{}, bundle.Documents...)
				other.Documents[0].Revision++
				other.BundleSHA256 = cc.ExecutionSkillBundleDigest(other)
				a.Config["execution_skills"] = other
			case "unknown-config-field":
				var raw map[string]any
				bytes, _ := json.Marshal(bundle)
				if err := json.Unmarshal(bytes, &raw); err != nil {
					t.Fatal(err)
				}
				raw["path"] = "foreign"
				a.Config["execution_skills"] = raw
			}
			if _, err := codexExecutionSkillsRequest(a, binding); err == nil {
				t.Fatal("unprotected or mismatched skill input was accepted", scenario)
			}
		})
	}
	if value, err := codexExecutionSkillsRequest(Assignment{Config: map[string]any{}}, nil); err != nil || value != nil {
		t.Fatal("ordinary no-skill assignment changed", value, err)
	}
}

func TestCodexSkillStartupFailureRetainsMaterializedReceipt(t *testing.T) {
	cfg, a := codexRPCFixture(t, "completed")
	spec := cfg.Commands["codex041"]
	const ref = "SUMA_SKILL_STARTUP_KEY_173"
	spec.CodexAppServer.APIKeyEnvRef = ref
	cfg.Commands["codex041"] = spec
	t.Setenv(ref, "controlled-test-key-not-a-real-account")
	cfg.ToolInventory = describeForTest(t, cfg)
	if len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available || cfg.ToolInventory[0].CodexCatalog == nil {
		t.Fatal("controlled local profile was not observed", cfg.ToolInventory)
	}
	catalog := *cfg.ToolInventory[0].CodexCatalog
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-model-041", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	a.Config["tool_profile_sha256"] = cfg.ToolInventory[0].Profile.ProfileSHA256
	a.Config["codex_execution"] = binding
	a.ExecutionSkills = codexSkillBundleForMount(t)
	a.Config["execution_skills"] = a.ExecutionSkills
	// The credential exists during the no-model local inventory, then becomes
	// unavailable after the fixed plan. Host materialization still occurred;
	// the execution must retain that fact without inventing a CLI or turn ACK.
	t.Setenv(ref, "")
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode == 0 || r.Error == "" || r.ToolEvidence == nil || r.ToolEvidence.CodexExecution == nil || r.ToolEvidence.CodexExecution.Skills == nil {
		t.Fatal("startup erased the occurred host receipt", r)
	}
	proof := r.ToolEvidence.CodexExecution.Skills
	if r.ToolEvidence.Terminal != "failed" || !proof.Materialized || proof.CatalogAcknowledged || proof.InvocationRequested || proof.TurnAcknowledged || proof.FilesVerified || r.ToolEvidence.CodexExecution.ThreadID != "" || r.ToolEvidence.CodexExecution.TurnID != "" || !cc.ExecutionSkillEvidenceMatchesBundle(*a.ExecutionSkills, *proof) {
		t.Fatal("startup invented native invocation evidence", r.ToolEvidence)
	}
	if err = tc.ValidateEvidence(*r.ToolEvidence); err != nil {
		t.Fatal("startup receipt is not a valid public result", err)
	}
	if got := strings.TrimSpace(gitTest(t, cfg.Repository, "rev-parse", "HEAD")); got != cfg.BaseCommit {
		t.Fatal("skills changed the administrator Git pin")
	}
}
