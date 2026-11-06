package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// Accounting is a human-facing control-plane domain. Execution credentials
// report their ordinary fenced result; they cannot write prices or balances.
func (s *Server) usageAccountingHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "usage" {
		return false
	}
	attempts := len(parts) == 2 && parts[1] == "attempts"
	facts := len(parts) == 4 && parts[1] == "attempts" && parts[3] == "facts"
	if facts {
		w.Header().Set("Cache-Control", "no-store")
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	limit := 20
	if len(r.URL.RawQuery) > 8192 {
		err = invalid("Invalid usage query")
	}
	for key, items := range values {
		allowed := key == "project_id" || r.Method == http.MethodGet &&
			(!facts && key == "month" || attempts && key == "state" || (attempts || facts) && (key == "limit" || key == "before"))
		if !allowed || len(items) != 1 {
			err = invalid("Invalid usage query")
		}
	}
	if v, ok := values["project_id"]; ok && (v[0] == "" || v[0] != project) {
		err = invalid("Invalid usage query")
	}
	if r.Method == http.MethodGet && values.Get("project_id") == "" {
		err = invalid("Invalid usage query")
	}
	if raw, ok := values["limit"]; ok {
		if raw[0] == "" || raw[0][0] == '0' {
			err = invalid("Invalid usage query")
		} else {
			for _, c := range raw[0] {
				if c < '0' || c > '9' {
					err = invalid("Invalid usage query")
				}
			}
			var parseErr error
			limit, parseErr = strconv.Atoi(raw[0])
			if parseErr != nil || limit < 1 || limit > 100 {
				err = invalid("Invalid usage query")
			}
		}
	}
	if raw, ok := values["before"]; ok && (len(raw[0]) > 64 || facts && raw[0] == "") {
		err = invalid("Invalid usage query")
	}
	if err != nil {
		writeError(w, invalid("Invalid usage query"))
		return true
	}
	decode := func(out any, required ...string) error {
		for _, key := range required {
			if value, ok := m[key]; !ok || value == nil {
				return invalid("Incomplete usage request")
			}
		}
		return strictAutomation(m, out)
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		out, err := s.Service.GetUsageDashboard(r.Context(), actor, project, values.Get("month"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && parts[1] == "policies" && r.Method == http.MethodPost:
		var req UsagePolicyWriteRequest
		if _, ok := m["token_limit"]; !ok {
			writeError(w, invalid("Incomplete usage request"))
			break
		}
		if _, ok := m["cost_limit_micros"]; !ok {
			writeError(w, invalid("Incomplete usage request"))
			break
		}
		if err := decode(&req, "project_id", "scope", "expected_version", "mode", "currency", "reason", "acknowledge_enforcement", "idempotency_key"); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.WriteUsagePolicy(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && parts[1] == "rates" && r.Method == http.MethodPost:
		var req UsageRateWriteRequest
		if err := decode(&req, "project_id", "tool_profile_id", "expected_tool_version", "expected_version", "currency", "input_rate_micros_per_million", "output_rate_micros_per_million", "input_token_reservation", "output_token_reservation", "reason", "idempotency_key"); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.WriteUsageRate(r.Context(), actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case attempts && r.Method == http.MethodGet:
		out, err := s.Service.ListUsageAttempts(r.Context(), actor, project, values.Get("month"), values.Get("state"), limit, values.Get("before"))
		respond(w, http.StatusOK, out, err)
	case facts && r.Method == http.MethodGet:
		out, err := s.Service.ListUsageAttemptFacts(r.Context(), actor, project, parts[2], limit, values.Get("before"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 4 && parts[1] == "attempts" && parts[3] == "confirm" && r.Method == http.MethodPost:
		var req UsageConfirmRequest
		if err := decode(&req, "project_id", "expected_version", "usage", "source_ref", "reason", "acknowledge_usage", "idempotency_key"); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.ConfirmUsageAttempt(r.Context(), actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
