package platform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// The transport supplies the original scoped identity. Locations are lookup
// hints, never caller-supplied evidence; the domain freezes authoritative bytes.
func (s *Server) failureDiagnosisHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "failure-diagnosis" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(diagnosisInvalid))
		return true
	}
	locatedRead := r.Method == http.MethodGet && len(parts) == 2 && (parts[1] == "context" || parts[1] == "analyses")
	listRead := locatedRead && parts[1] == "analyses"
	for name, items := range query {
		allowed := name == "project_id" || locatedRead && (name == "run_id" || name == "node_id" || name == "attempt") || listRead && (name == "limit" || name == "before")
		if !allowed || len(items) != 1 || items[0] == "" {
			writeError(w, invalid(diagnosisInvalid))
			return true
		}
	}
	if supplied, present := query["project_id"]; present && supplied[0] != project || r.Method == http.MethodGet && query.Get("project_id") == "" {
		writeError(w, invalid(diagnosisInvalid))
		return true
	}
	attempt := 0
	if locatedRead {
		attempt, err = failureDiagnosisQueryInteger(query.Get("attempt"), 1, 2147483646)
		if err != nil || query.Get("run_id") == "" || query.Get("node_id") == "" {
			writeError(w, invalid(diagnosisInvalid))
			return true
		}
	}
	switch {
	case locatedRead && parts[1] == "context":
		out, err := s.Service.FailureDiagnosisContext(r.Context(), s.Engine, actor, project, query.Get("run_id"), query.Get("node_id"), attempt)
		respond(w, http.StatusOK, out, err)
	case listRead:
		limit := 20
		if value := query.Get("limit"); value != "" {
			limit, err = failureDiagnosisQueryInteger(value, 1, 100)
		}
		if err != nil || len(query.Get("before")) > 4096 {
			writeError(w, invalid(diagnosisInvalid))
			return true
		}
		out, err := s.Service.ListFailureDiagnoses(r.Context(), s.Engine, actor, project, query.Get("run_id"), query.Get("node_id"), attempt, limit, query.Get("before"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && parts[1] == "analyses" && r.Method == http.MethodPost:
		var req FailureDiagnosisCreateRequest
		if err := projectKnowledgeRequired(body, "project_id", "issue_id", "expected_issue_version", "run_id", "node_id", "attempt", "expected_source_sha256", "goal", "locale", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key"); err != nil {
			writeError(w, invalid(diagnosisInvalid))
			return true
		}
		if err := failureDiagnosisBodyIntegers(body, map[string]int64{"expected_issue_version": 2147483646, "attempt": 2147483646, "expected_tool_version": 2147483646, "max_steps": 1, "max_output_tokens": 8192}); err != nil {
			writeError(w, err)
			return true
		}
		if err := strictAutomation(body, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateFailureDiagnosis(r.Context(), s.Engine, actor, req)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 3 && parts[1] == "analyses" && r.Method == http.MethodGet:
		out, err := s.Service.GetFailureDiagnosis(r.Context(), s.Engine, actor, project, parts[2])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 4 && parts[1] == "analyses" && parts[3] == "commands" && r.Method == http.MethodPost:
		var req FailureDiagnosisCommandRequest
		if err := projectKnowledgeRequired(body, "project_id", "expected_version", "action", "reason", "idempotency_key"); err != nil {
			writeError(w, invalid(diagnosisInvalid))
			return true
		}
		if err := failureDiagnosisBodyIntegers(body, map[string]int64{"expected_version": 2147483646}); err != nil {
			writeError(w, err)
			return true
		}
		for _, name := range []string{"repair", "acknowledge_draft"} {
			if raw, present := body[name]; present && raw == nil {
				writeError(w, invalid(diagnosisInvalid))
				return true
			}
		}
		if err := strictAutomation(body, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandFailureDiagnosis(r.Context(), s.Engine, actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

func failureDiagnosisQueryInteger(raw string, minimum, maximum int) (int, error) {
	if raw == "" || raw[0] == '0' {
		return 0, invalid(diagnosisInvalid)
	}
	for _, char := range raw {
		if char < '0' || char > '9' {
			return 0, invalid(diagnosisInvalid)
		}
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minimum || n > maximum {
		return 0, invalid(diagnosisInvalid)
	}
	return n, nil
}

func failureDiagnosisBodyIntegers(body map[string]any, fields map[string]int64) error {
	for name, maximum := range fields {
		raw, present := body[name]
		if !present {
			continue
		}
		number, ok := raw.(json.Number)
		if !ok {
			return invalid(diagnosisInvalid)
		}
		value, err := number.Int64()
		if err != nil || value < 1 || value > maximum {
			return invalid(diagnosisInvalid)
		}
	}
	return nil
}

func validateFailureIntakeReadQuery(r *http.Request, project string) error {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 || len(values) != 1 {
		return invalid(diagnosisInvalid)
	}
	if current := values["project_id"]; len(current) != 1 || current[0] == "" || current[0] != project {
		return invalid(diagnosisInvalid)
	}
	return nil
}
