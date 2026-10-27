package dag

import (
	"encoding/json"
	"fmt"

	"github.com/ai-mada/suma/internal/modelcontract"
)

// ModelBinding never treats a malformed or partial binding as a legacy node.
func ModelBinding(n Node) (*modelcontract.Binding, error) {
	raw, exists := n.Config["model_execution"]
	if !exists {
		return nil, nil
	}
	if _, codex := n.Config["codex_execution"]; codex {
		return nil, fmt.Errorf("%w: invalid model execution binding", ErrInvalid)
	}
	// Model requests are not certified idempotent provider operations. A fixed
	// model binding must not inherit either scheduler generation's auto retries.
	if n.MaxRetries != 0 {
		return nil, fmt.Errorf("%w: invalid model execution binding", ErrInvalid)
	}
	b, err := modelcontract.DecodeBinding(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid model execution binding", ErrInvalid)
	}
	_, _, bound, err := ToolBinding(n)
	if err != nil || !bound || n.Type != "agent" {
		return nil, fmt.Errorf("%w: model execution requires fixed native agent tool", ErrInvalid)
	}
	for key, value := range map[string]int64{"max_steps": int64(b.Parameters.MaxSteps), "max_output_tokens": b.Parameters.MaxOutputTokens} {
		raw, exists := n.Config[key]
		if !exists {
			return nil, fmt.Errorf("%w: model execution limits differ", ErrInvalid)
		}
		var actual int64
		switch v := raw.(type) {
		case int:
			actual = int64(v)
		case int64:
			actual = v
		case json.Number:
			var e error
			actual, e = v.Int64()
			if e != nil {
				return nil, fmt.Errorf("%w: model execution limits differ", ErrInvalid)
			}
		case float64:
			actual = int64(v)
			if v != float64(actual) {
				return nil, fmt.Errorf("%w: model execution limits differ", ErrInvalid)
			}
		default:
			return nil, fmt.Errorf("%w: model execution limits differ", ErrInvalid)
		}
		if actual != value {
			return nil, fmt.Errorf("%w: model execution limits differ", ErrInvalid)
		}
	}
	return &b, nil
}
