package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestParameterizedDiagnosticMetadataCannotRelabelRawMessage(t *testing.T) {
	params := map[string]any{"field": "expected_version"}
	valid := invalidDiagnostic("validation.field_required", "缺少expected_version", params)
	params["field"] = "later_caller_change"
	foreign := &APIError{403, "specific_denial", "Keep this exact unrelated denial", map[string]any{"version": 0, "allowed": false}}
	cases := []struct {
		name      string
		failure   error
		status    int
		message   string
		localized bool
	}{
		{"snapshot through wrapper", fmt.Errorf("request context: %w", valid), 400, "expected_version is required.", true},
		{"unrelated joined cause", errors.Join(foreign, valid), 403, foreign.Message, false},
		{"mismatched source", invalidDiagnostic("validation.field_required", "Keep the actual complete diagnostic", map[string]any{"field": "expected_version"}), 400, "Keep the actual complete diagnostic", false},
		{"extra parameter", invalidDiagnostic("validation.field_required", "Already-public original", map[string]any{"field": "expected_version", "undeclared": "PRIVATE_UNDECLARED_PARAMETER"}), 400, "Already-public original", false},
		{"invalid parameter", invalidDiagnostic("validation.field_required", "Already-public original", map[string]any{"field": "PRIVATE_UNDECLARED_PARAMETER\n"}), 400, "Already-public original", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			w.Header().Set("Content-Language", "en-US")
			writeError(w, c.failure)
			var body struct {
				Error map[string]any `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != c.status || body.Error["message"] != c.message || body.Error["localized"] != c.localized || strings.Contains(w.Body.String(), "PRIVATE_UNDECLARED_PARAMETER") {
				t.Fatalf("presentation replaced cause or leaked undeclared parameters: %s", w.Body.String())
			}
			if !c.localized && body.Error["message_params"] != nil {
				t.Fatal("unvalidated parameters escaped fallback")
			}
			if c.status == 403 && stringify(body.Error["details"]) != stringify(foreign.Details) {
				t.Fatal("joined display metadata changed authority details")
			}
		})
	}
}

// These failures originate in real validators, not synthetic translated strings.
// Their original Error() is also part of internal debugging/recovery behavior.
func TestParameterizedValidatorDiagnosticsKeepOriginalIdentity(t *testing.T) {
	cases := []struct {
		name, key, field, raw, english string
		failure                        error
	}{
		{"required", "validation.field_required", "expected_version", "缺少expected_version", "expected_version is required.", exactVersion(map[string]any{}, "expected_version", 1)},
		{"integer", "validation.field_integer", "expected_version", "expected_version必须为整数", "expected_version must be an integer.", exactVersion(map[string]any{"expected_version": "1"}, "expected_version", 1)},
		{"version", "validation.field_version_integer", "expected_version", "expected_version必须为有效版本整数", "expected_version must be a valid version integer.", exactVersion(map[string]any{"expected_version": 0}, "expected_version", 1)},
		{"generation required", "validation.field_required", "expected_environment_generation", "缺少expected_environment_generation", "expected_environment_generation is required.", releaseGeneration(map[string]any{}, "expected_environment_generation")},
		{"generation integer", "validation.field_integer", "expected_environment_generation", "expected_environment_generation必须为整数", "expected_environment_generation must be an integer.", releaseGeneration(map[string]any{"expected_environment_generation": false}, "expected_environment_generation")},
		{"generation range", "validation.field_safe_integer", "expected_environment_generation", "expected_environment_generation必须为安全非负整数", "expected_environment_generation must be a safe non-negative integer.", releaseGeneration(map[string]any{"expected_environment_generation": -1}, "expected_environment_generation")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var original *APIError
			if !errors.As(c.failure, &original) || original.Status != 400 || original.Code != "invalid_input" || c.failure.Error() != c.raw {
				t.Fatalf("validator identity changed: %v", c.failure)
			}
			for locale, want := range map[string]string{"zh-CN": c.raw, "en-US": c.english} {
				w := httptest.NewRecorder()
				w.Header().Set("Content-Language", locale)
				writeError(w, c.failure)
				var body struct {
					Error map[string]any `json:"error"`
				}
				if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
					t.Fatal(e)
				}
				params, ok := body.Error["message_params"].(map[string]any)
				if w.Code != 400 || body.Error["code"] != "invalid_input" || body.Error["message_key"] != c.key || body.Error["message"] != want || body.Error["localized"] != true || !ok || params["field"] != c.field || body.Error["details"] != nil {
					t.Fatalf("%s lost exact parameterized failure: %s", locale, w.Body.String())
				}
			}
			if c.failure.Error() != c.raw || original.Message != c.raw {
				t.Fatal("serialization mutated the raw diagnostic")
			}
		})
	}
}

// Two independently negotiated requests must not change each other's language
// or create a rule/plan when strict input decoding fails before any transaction.
func TestParameterizedDecodeHTTPConcurrentLanguageAndNoWrites(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(f.handler)
	defer server.Close()
	paths := []struct{ path, key, zh, en string }{
		{"/automations/preview", "validation.automation_detail", "自动化字段或类型无效: ", "Invalid automation fields or types: "},
		{"/release-plans/preview", "validation.release_detail", "发布请求存在未知字段或错误类型: ", "The release request contains unknown fields or incorrect types: "},
	}
	var wait sync.WaitGroup
	errorsChannel := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			c := paths[i%len(paths)]
			locale, prefix := "zh-CN", c.zh
			if i%4 >= 2 {
				locale, prefix = "en-US", c.en
			}
			payload, _ := json.Marshal(map[string]any{"project_id": f.project, "unexpected_field": "<script>raw payload</script>"})
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1"+c.path, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept-Language", locale)
			req.AddCookie(f.cookie)
			res, err := server.Client().Do(req)
			if err != nil {
				errorsChannel <- err.Error()
				return
			}
			defer res.Body.Close()
			var body struct {
				Error map[string]any `json:"error"`
			}
			if err = json.NewDecoder(res.Body).Decode(&body); err != nil {
				errorsChannel <- err.Error()
				return
			}
			params, ok := body.Error["message_params"].(map[string]any)
			if res.StatusCode != 400 || res.Header.Get("Content-Language") != locale || body.Error["code"] != "invalid_input" || body.Error["message_key"] != c.key || body.Error["localized"] != true || !ok || params["detail"] != `json: unknown field "unexpected_field"` || body.Error["message"] != prefix+`json: unknown field "unexpected_field"` || strings.Contains(stringify(body), "raw payload") {
				errorsChannel <- stringify(body)
			}
		}(i)
	}
	wait.Wait()
	close(errorsChannel)
	for message := range errorsChannel {
		t.Error(message)
	}
	for _, table := range []string{"suma_automations", "suma_release_plans", "dag_runs"} {
		var n int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("rejected decode wrote %s: %d %v", table, n, err)
		}
	}
}
