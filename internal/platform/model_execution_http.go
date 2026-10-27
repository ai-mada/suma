package platform

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Model options are observations under the same current project/tool authority
// as dispatch. Reading them never probes a provider or changes configuration.
func (s *Server) modelExecutionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) != 2 || parts[0] != "model-execution" || parts[1] != "options" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		writeError(w, invalid(modelExecutionInvalid))
		return true
	}
	for key, values := range q {
		if (key != "project_id" && key != "tool_profile_id" && key != "expected_tool_version") || len(values) != 1 || values[0] == "" {
			writeError(w, invalid(modelExecutionInvalid))
			return true
		}
	}
	version, err := strconv.Atoi(q.Get("expected_tool_version"))
	if err != nil || version < 1 || version > 2147483646 || strconv.Itoa(version) != q.Get("expected_tool_version") || q.Get("project_id") != project {
		writeError(w, invalid(modelExecutionInvalid))
		return true
	}
	value, err := s.Service.GetModelExecutionOptions(r.Context(), actor, project, ModelExecutionOptionsRequest{ProjectID: project, ToolProfileID: q.Get("tool_profile_id"), ExpectedToolVersion: version})
	respond(w, http.StatusOK, value, err)
	return true
}

// validateModelSelectionFields preserves explicit caller intent before a typed
// decoder can turn JSON null into a missing optional pointer. All dispatch
// entry points use the same closed model contracts; authorization and current
// tool/catalog checks still belong to their domain transaction.
func validateModelSelectionFields(body map[string]any) error {
	for key := range body {
		for _, canonical := range []string{"model_execution_selection", "codex_execution_selection"} {
			if key != canonical && strings.EqualFold(key, canonical) {
				return invalid(modelExecutionInvalid)
			}
		}
	}
	native, nativePresent := body["model_execution_selection"]
	codex, codexPresent := body["codex_execution_selection"]
	if nativePresent && codexPresent {
		return invalid(codexExecutionInvalid)
	}
	if nativePresent {
		if _, err := mc.DecodeSelection(native); err != nil || !canonicalModelSelectionKeys(native, reflect.TypeOf(mc.Selection{})) {
			return invalid(modelExecutionInvalid)
		}
	}
	if codexPresent {
		if _, err := cc.DecodeSelection(codex); err != nil || !canonicalModelSelectionKeys(codex, reflect.TypeOf(cc.Selection{})) {
			return invalid(codexExecutionInvalid)
		}
	}
	return nil
}

// encoding/json folds field names even with DisallowUnknownFields. Public model
// choices have canonical keys; inspect the original map against the same DTO
// tags before decoding can hide a case alias. Value/null/range semantics remain
// owned by the existing model contract decoders.
func canonicalModelSelectionKeys(value any, kind reflect.Type) bool {
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	if value == nil || kind.Kind() != reflect.Struct {
		return true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	fields := map[string]reflect.Type{}
	for i := 0; i < kind.NumField(); i++ {
		field := kind.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if field.PkgPath == "" && name != "" && name != "-" {
			fields[name] = field.Type
		}
	}
	for key, nested := range object {
		field, found := fields[key]
		if !found || !canonicalModelSelectionKeys(nested, field) {
			return false
		}
	}
	return true
}
