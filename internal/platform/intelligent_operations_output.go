package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	operationInvalid   = "Invalid intelligent operations request"
	operationOutput    = "The model output does not satisfy the closed local-draft plan contract"
	operationVersion   = "The intelligent operation plan has changed; reload before deciding"
	operationSource    = "The intelligent operation source has changed; create a new analysis"
	operationProfile   = "Select a current enabled read-only Suma intelligent operations profile"
	operationConsent   = "Confirm that the selected goal and source text will be sent to the configured model"
	operationDrafts    = "Review the current preview and confirm the local draft changes"
	operationQuestions = "Resolve the plan questions before applying local drafts"
	operationPrompt    = "Intelligent operation context exceeds 32000 UTF-8 bytes; select a smaller scope"
	operationSensitive = "Remove private keys, obvious tokens and URL credentials before model analysis"
	operationFailed    = "The intelligent operation model execution did not complete successfully"
	operationUnknown   = "The model execution outcome is unknown; inspect the original run"
	operationAuthority = "The intelligent operation creator no longer has current execution authority"
	operationPolicy    = "suma-local-drafts-v1"
	operationLimit     = 32000
)

// This is a small preflight screen, not a complete secret classification system.
var operationSecretPattern = regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----|\bsk-(?:proj-)?[a-z0-9_-]{16,}|\bgh[pousr]_[a-z0-9_]{20,}|\bbearer\s+[a-z0-9._~+/=\-]{16,}|https?://[^\s/]+@[^\s/]+|(?:api[_ -]?key|access[_ -]?token|authorization|password)\s*[:=]\s*["']?[a-z0-9_./+\-=]{12,}`)

func operationSafeText(text string) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0) && !operationSecretPattern.MatchString(text)
}

// encoding/json otherwise accepts duplicate object members. Walk the complete
// document first so no caller can hide a second action, target or budget field.
func operationJSONValue(d *json.Decoder, depth int) error {
	if depth > 40 {
		return errors.New("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("null is not a local-draft contract value")
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate JSON member")
			}
			seen[name] = true
			if err = operationJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = operationJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected delimiter")
	}
	end, err := d.Token()
	if err != nil {
		return err
	}
	if delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
		return errors.New("invalid JSON closing token")
	}
	return nil
}

func operationStrictJSON(raw string, out any) error {
	if len(raw) > 64<<10 || !utf8.ValidString(raw) {
		return invalid(operationOutput)
	}
	scan := json.NewDecoder(strings.NewReader(raw))
	scan.UseNumber()
	if err := operationJSONValue(scan, 0); err != nil {
		return invalid(operationOutput)
	}
	if _, err := scan.Token(); err != io.EOF {
		return invalid(operationOutput)
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return invalid(operationOutput)
	}
	return nil
}

func operationPriority(priority string) bool {
	return priority == "" || priority == "low" || priority == "medium" || priority == "high" || priority == "urgent"
}

func parseIntelligentOperationOutput(raw string, source IntelligentOperationSource) (out IntelligentOperationOutput, err error) {
	bad := func() (IntelligentOperationOutput, error) {
		return IntelligentOperationOutput{}, invalid(operationOutput)
	}
	if len(raw) > operationLimit || !operationSafeText(raw) || operationStrictJSON(raw, &out) != nil || out.SchemaVersion != 1 || !planningText(out.Summary, 2000) || !planningText(out.Rationale, 8000) || out.Questions == nil || len(out.Questions) > 8 || out.Steps == nil || len(out.Steps) > 8 || len(out.Steps) == 0 && len(out.Questions) == 0 {
		return bad()
	}
	for _, question := range out.Questions {
		if !planningText(question, 1000) {
			return bad()
		}
	}
	steps := map[string]IntelligentOperationStep{}
	designTargets := map[string]bool{}
	for _, step := range out.Steps {
		if !designKey(step.Key) || steps[step.Key].Key != "" || step.DependsOn == nil || len(step.DependsOn) > 8 {
			return bad()
		}
		steps[step.Key] = step
		seen := map[string]bool{}
		for _, dependency := range step.DependsOn {
			if !designKey(dependency) || dependency == step.Key || seen[dependency] {
				return bad()
			}
			seen[dependency] = true
		}
		switch step.Action {
		case "requirement.create_draft":
			p := step.Requirement
			if p == nil || step.Design != nil || step.Knowledge != nil || !planningText(p.Title, 200) || !knowledgeText(p.Description, 16000) || !operationPriority(p.Priority) || p.AcceptanceCriteria == nil || len(p.AcceptanceCriteria) < 1 || len(p.AcceptanceCriteria) > 16 {
				return bad()
			}
			for _, criterion := range p.AcceptanceCriteria {
				if !planningText(criterion, 1000) {
					return bad()
				}
			}
		case "requirement.design.save_draft":
			p := step.Design
			if p == nil || step.Requirement != nil || step.Knowledge != nil || (p.RequirementID == "") == (p.RequirementStepKey == "") || validateDesignDraft(p.Content, p.Tasks) != nil {
				return bad()
			}
			target := "step:" + p.RequirementStepKey
			if p.RequirementID != "" {
				if !handoffID(p.RequirementID, false) {
					return bad()
				}
				found := false
				for _, ref := range source.Sources {
					if ref.Kind == "requirements" && ref.ID == p.RequirementID {
						found = true
					}
				}
				if !found {
					return bad()
				}
				target = "id:" + p.RequirementID
			} else if !designKey(p.RequirementStepKey) || !seen[p.RequirementStepKey] {
				return bad()
			}
			if designTargets[target] {
				return bad()
			}
			designTargets[target] = true
		case "knowledge.create_draft":
			p := step.Knowledge
			if p == nil || step.Requirement != nil || step.Design != nil || p.SourceIndices == nil || len(p.SourceIndices) > len(source.Sources) {
				return bad()
			}
			if validateKnowledgeDraft(ProjectKnowledgeWriteRequest{Kind: p.Kind, Title: p.Title, Summary: p.Summary, Content: p.Content}) != nil {
				return bad()
			}
			indices := map[int]bool{}
			for _, index := range p.SourceIndices {
				if index < 0 || index >= len(source.Sources) || indices[index] {
					return bad()
				}
				indices[index] = true
			}
		default:
			return bad()
		}
	}
	for _, step := range out.Steps {
		for _, dependency := range step.DependsOn {
			if steps[dependency].Key == "" {
				return bad()
			}
		}
		if step.Design != nil && step.Design.RequirementStepKey != "" && steps[step.Design.RequirementStepKey].Action != "requirement.create_draft" {
			return bad()
		}
	}
	if _, err := operationOrderedSteps(out); err != nil {
		return bad()
	}
	return out, nil
}

func operationOrderedSteps(proposal IntelligentOperationOutput) ([]IntelligentOperationStep, error) {
	steps := map[string]IntelligentOperationStep{}
	for _, step := range proposal.Steps {
		steps[step.Key] = step
	}
	state := map[string]int{}
	ordered := []IntelligentOperationStep{}
	var visit func(string) error
	visit = func(key string) error {
		if state[key] == 1 {
			return invalid(operationOutput)
		}
		if state[key] == 2 {
			return nil
		}
		step, found := steps[key]
		if !found {
			return invalid(operationOutput)
		}
		state[key] = 1
		for _, dependency := range step.DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[key] = 2
		ordered = append(ordered, step)
		return nil
	}
	for _, step := range proposal.Steps {
		if err := visit(step.Key); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
