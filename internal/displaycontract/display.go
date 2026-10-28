// Package displaycontract binds optional presentation identity to complete raw
// platform diagnostics. It does not authorize dispatch or change business facts.
package displaycontract

import (
	"errors"
	"sort"

	"github.com/ai-mada/suma/internal/localization"
)

type DisplayDiagnostic struct {
	SchemaVersion int            `json:"schema_version"`
	MessageKey    string         `json:"message_key"`
	MessageParams map[string]any `json:"message_params,omitempty"`
}

type SourceDefinition struct {
	Key            string                   `json:"key"`
	Parameters     []localization.Parameter `json:"parameters"`
	SourceTemplate string                   `json:"source_template"`
}

// Explicit approval keeps future display domains and native keys out of tool
// eligibility. Canonical prose lives only in the Go localization catalog.
var toolKeys = map[string]bool{
	"display.tool.profile_revoked": true, "display.tool.profile_paused": true,
	"display.tool.activation_revoked": true, "display.tool.worker_expired": true,
	"display.tool.worker_creator_revoked": true, "display.tool.worker_unregistered": true,
	"display.tool.pool_paused": true, "display.tool.pool_unavailable": true,
	"display.tool.pool_expired": true, "display.tool.inventory_missing": true,
	"display.tool.inventory_stale": true, "display.tool.command_missing": true,
	"display.tool.profile_mismatch": true, "display.tool.reported_unavailable": true,
	"display.tool.dispatchable": true,
}

// BindToolReason is called at a concrete trusted producer, never by reverse
// lookup of stored prose. Invalid presentation is simply absent.
func BindToolReason(source, key string, params map[string]any) *DisplayDiagnostic {
	return ValidateToolReason(source, &DisplayDiagnostic{SchemaVersion: 1, MessageKey: key, MessageParams: params})
}

func ValidateToolReason(source string, value *DisplayDiagnostic) *DisplayDiagnostic {
	if source == "" || value == nil || value.SchemaVersion != 1 || !toolKeys[value.MessageKey] {
		return nil
	}
	proof := localization.AnnotateError(errors.New(source), value.MessageKey, localization.Params(value.MessageParams))
	rendered, ok := localization.FormatError(localization.EnUS, proof)
	if !ok {
		return nil
	}
	// The template validator only returns scalar values and a fresh map.
	return &DisplayDiagnostic{SchemaVersion: 1, MessageKey: value.MessageKey, MessageParams: rendered.Params}
}

func SourceDefinitions() []SourceDefinition {
	out := make([]SourceDefinition, 0, len(toolKeys))
	for _, d := range localization.Catalog() {
		if toolKeys[d.Key] && d.SourceTemplate != "" {
			out = append(out, SourceDefinition{Key: d.Key, Parameters: append([]localization.Parameter{}, d.Parameters...), SourceTemplate: d.SourceTemplate})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
