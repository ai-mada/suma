package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func localPlanningCapability(spec CommandSpec) (*tc.PlanningCapability, error) {
	if !spec.Planning {
		return nil, nil
	}
	if spec.KnowledgeExtraction {
		return nil, fmt.Errorf("planning and knowledge extraction profiles are mutually exclusive")
	}
	model, origin, err := localReadOnlyModel(spec, "planning")
	if err != nil {
		return nil, err
	}
	p := &tc.PlanningCapability{Protocol: tc.PlanningProtocol, Model: model, EndpointOrigin: origin}
	if err = tc.ValidatePlanningCapability(*p); err != nil {
		return nil, err
	}
	return p, nil
}

func probePlanning(ctx context.Context, exe string, spec CommandSpec, secrets []string) error {
	expected, err := localPlanningCapability(spec)
	if err != nil || expected == nil {
		return err
	}
	raw, err := probeCommandStreams(ctx, exe, append(append([]string{}, spec.Args...), "--planning-probe"), spec, secrets, true)
	if err != nil {
		return err
	}
	if _, err = decodeToolObject([]byte(raw)); err != nil {
		return fmt.Errorf("planning protocol probe is invalid")
	}
	var actual tc.PlanningCapability
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&actual) != nil || actual != *expected || tc.ValidatePlanningCapability(actual) != nil {
		return fmt.Errorf("planning protocol probe does not match fixed local model configuration")
	}
	return nil
}

func planningLimit(value any, fallback, maximum int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v < 1 || v > float64(maximum) {
			return 0, fmt.Errorf("planning budget is invalid")
		}
		n = int64(v)
	case json.Number:
		var err error
		n, err = v.Int64()
		if err != nil {
			return 0, fmt.Errorf("planning budget is invalid")
		}
	default:
		return 0, fmt.Errorf("planning budget is invalid")
	}
	if n < 1 || n > int64(maximum) {
		return 0, fmt.Errorf("planning budget is invalid")
	}
	return int(n), nil
}

// Only the fixed planning purpose opens its smaller budget contract. Ordinary
// agent requests retain their original input and limits.
func planningRequest(a Assignment, spec CommandSpec, p tc.Profile) (map[string]any, error) {
	value, present := a.Config["purpose"]
	if !present {
		if spec.Planning {
			return nil, fmt.Errorf("planning profile requires the planning purpose")
		}
		return nil, nil
	}
	purpose, ok := value.(string)
	if !ok || purpose != "planning" {
		return nil, fmt.Errorf("unsupported agent purpose")
	}
	if a.Type != "agent" || !spec.Planning || p.Planning == nil || spec.NativeWorkspace != nil || spec.Input != "agent-jsonl" {
		return nil, fmt.Errorf("planning purpose requires a probed planning profile")
	}
	for _, name := range []string{"max_steps", "max_output_tokens"} {
		if value, present := a.Config[name]; present && value == nil {
			return nil, fmt.Errorf("planning budget is invalid")
		}
	}
	steps, err := planningLimit(a.Config["max_steps"], 3, 4)
	if err != nil {
		return nil, err
	}
	tokens, err := planningLimit(a.Config["max_output_tokens"], 4096, 8192)
	if err != nil {
		return nil, err
	}
	return map[string]any{"purpose": "planning", "max_steps": steps, "max_tokens": tokens}, nil
}
