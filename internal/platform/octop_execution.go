package platform

import tc "github.com/ai-mada/suma/internal/toolcontract"

// Octop uses the same deployment selection, immutable ModelBinding and Go
// accounting path as the native runtime; it has no separate model authority.
func supportsOctopExecution(p ToolProfile) bool { return tc.SupportsOctopExecutionProfile(p.Profile) }
