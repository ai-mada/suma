package runner

import (
	"fmt"
	"path/filepath"
	"strings"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

func (d *codexAppServer) configureSkills(bundle *cc.ExecutionSkillBundle, evidence *cc.ExecutionSkillEvidence) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.skillsConfigured || d.frames != 0 || d.waiting != "initialize" || d.stopping.Load() || d.bad != nil {
		return fmt.Errorf("Codex skill configuration is already fixed")
	}
	if bundle == nil && evidence == nil {
		d.skillsConfigured = true
		return nil
	}
	if bundle == nil || evidence == nil || d.evidence == nil || !evidence.Materialized || evidence.CatalogAcknowledged || evidence.InvocationRequested || evidence.TurnAcknowledged || evidence.FilesVerified || evidence.FailureCode != "" || !cc.ExecutionSkillEvidenceMatchesBundle(*bundle, *evidence) {
		return fmt.Errorf("Codex skill materialization does not match the fixed bundle")
	}
	// Caller-owned bundle slices cannot change an accepted attempt. The receipt
	// is deliberately shared with the loader, which verifies files after Wait.
	copy := *bundle
	copy.Documents = append([]cc.ExecutionSkillDocument{}, bundle.Documents...)
	d.skills, d.skillsConfigured, d.evidence.Skills = &copy, true, evidence
	return nil
}

func (d *codexAppServer) failSkills(code, reason string) {
	if d.evidence != nil && d.evidence.Skills != nil && d.evidence.Skills.FailureCode == "" {
		d.evidence.Skills.FailureCode = code
	}
	d.fail(reason)
}

// skillsList adopts only the exact platform-approved name/path pairs. Other
// discovered skills grant no authority and are never added to the invocation.
func (d *codexAppServer) skillsList(result map[string]any) {
	if d.skills == nil || d.evidence == nil || d.evidence.Skills == nil || !d.evidence.ConfigurationAcknowledged || d.evidence.Skills.CatalogAcknowledged || d.turnStarted {
		d.failSkills("codex_skills_catalog_invalid", "unexpected skill catalog phase")
		return
	}
	data, ok := result["data"].([]any)
	if !ok || len(data) != 1 {
		d.failSkills("codex_skills_catalog_invalid", "skill catalog workspace is missing or repeated")
		return
	}
	row, ok := data[0].(map[string]any)
	if !ok || !sameCodexPath(textAt(row, "cwd"), d.root) {
		d.failSkills("codex_skills_catalog_invalid", "skill catalog workspace differs")
		return
	}
	errors, ok := row["errors"].([]any)
	if !ok || len(errors) != 0 {
		d.failSkills("codex_skills_catalog_invalid", "skill catalog reports unreadable documents")
		return
	}
	entries, ok := row["skills"].([]any)
	if !ok || len(entries) > 256 {
		d.failSkills("codex_skills_catalog_invalid", "skill catalog is missing or oversized")
		return
	}
	names, paths := map[string]string{}, map[string]string{}
	for _, doc := range d.skills.Documents {
		path := filepath.Join(d.home, "skills", doc.Name, "SKILL.md")
		names[doc.Name], paths[path] = path, doc.Name
	}
	seen := map[string]bool{}
	for _, value := range entries {
		entry, ok := value.(map[string]any)
		if !ok {
			d.failSkills("codex_skills_catalog_invalid", "skill catalog entry is invalid")
			return
		}
		name, path := textAt(entry, "name"), textAt(entry, "path")
		expectedPath, selectedName := names[name]
		_, selectedPath := paths[filepath.Clean(path)]
		if !selectedName && !selectedPath {
			continue
		}
		if !selectedName || !sameCodexPath(path, expectedPath) || entry["enabled"] != true || seen[name] {
			d.failSkills("codex_skills_catalog_invalid", "approved skill identity, path or availability differs")
			return
		}
		seen[name] = true
	}
	if len(seen) != len(d.skills.Documents) {
		d.failSkills("codex_skills_catalog_invalid", "approved skills are absent from the catalog")
		return
	}
	d.evidence.Skills.CatalogAcknowledged = true
	d.startSkillAwareTurn()
}

func (d *codexAppServer) startSkillAwareTurn() {
	goal := d.goal
	input := []any{map[string]any{"type": "text", "text": goal, "text_elements": []any{}}}
	if d.skills != nil {
		if d.evidence.Skills == nil || !d.evidence.Skills.CatalogAcknowledged || d.evidence.Skills.InvocationRequested {
			d.failSkills("codex_skills_catalog_invalid", "skill invocation has no unique catalog acknowledgement")
			return
		}
		names := make([]string, 0, len(d.skills.Documents))
		for _, doc := range d.skills.Documents {
			names = append(names, "$"+doc.Name)
			input = append(input, map[string]any{"type": "skill", "name": doc.Name, "path": filepath.Join(d.home, "skills", doc.Name, "SKILL.md")})
		}
		input[0].(map[string]any)["text"] = goal + "\n" + strings.Join(names, " ")
	}
	params := map[string]any{"threadId": d.thread, "model": d.binding.Model, "cwd": d.root, "approvalPolicy": cc.ApprovalPolicy, "sandboxPolicy": map[string]any{"type": "workspaceWrite", "writableRoots": []string{d.root}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}, "input": input}
	if d.binding.ReasoningEffort != "" {
		params["effort"] = d.binding.ReasoningEffort
	}
	d.request("turn/start", params)
	if d.skills != nil && d.bad == nil && d.waiting == "turn/start" {
		d.evidence.Skills.InvocationRequested = true
	}
}
