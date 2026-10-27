package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const repairCLIModelSelection = `{"deployment_id":"repair-model","deployment_revision":1,"deployment_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","parameters":{"max_steps":2,"max_output_tokens":100},"budget":{"token_limit":65000}}`

func TestCandidateRepairModelCLIExactSelectionAndUnknownOutcomeRecovery(t *testing.T) {
	for _, action := range []string{"repair", "revise-repair"} {
		t.Run(action, func(t *testing.T) {
			base, suffix := conflictRepairCLIRequest, "repairs"
			if action == "revise-repair" {
				base, suffix = repairRevisionCLIRequest, "repair-revisions"
			}
			body := strings.Replace(base, `"idempotency_key":`, `"model_execution_selection":`+repairCLIModelSelection+`,"idempotency_key":`, 1)
			calls, original := 0, ""
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/api/v1/candidate-compositions/original/"+suffix {
					t.Error("wrong operation route", r.Method, r.URL)
				}
				raw, _ := io.ReadAll(r.Body)
				if calls == 1 {
					original = string(raw)
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":{"code":"unavailable","message":"Original operation outcome unavailable"}}`))
					return
				}
				var originalBody map[string]json.RawMessage
				var originalChoice, intendedChoice map[string]any
				_ = json.Unmarshal([]byte(original), &originalBody)
				_ = json.Unmarshal(originalBody["model_execution_selection"], &originalChoice)
				_ = json.Unmarshal([]byte(repairCLIModelSelection), &intendedChoice)
				encodedChoice, _ := json.Marshal(originalChoice)
				encodedIntended, _ := json.Marshal(intendedChoice)
				if string(raw) != original || string(encodedChoice) != string(encodedIntended) {
					t.Error("recovery changed the original model selection or operation", string(raw), original)
				}
				w.WriteHeader(201)
				_, _ = w.Write([]byte(`{"id":"fixed-plan","task_id":"repair-task","run_id":""}`))
			}))
			defer s.Close()
			code, _, _ := contextInvoke(t, s.URL, "en-US", body, "candidate-compositions", action, "original", "--file", "-", "--confirm")
			if code == 0 || calls != 1 {
				t.Fatal("first model repair did not retain its unknown operation", code, calls)
			}
			code, output, errText := contextInvoke(t, s.URL, "en-US", body, "candidate-compositions", action, "original", "--file", "-", "--confirm")
			if code != 0 || errText != "" || calls != 2 || !strings.Contains(output, "fixed-plan") {
				t.Fatal(code, output, errText, calls)
			}
			for _, selection := range []string{
				`null`,
				strings.Replace(repairCLIModelSelection, `"deployment_revision":1`, `"deployment_revision":1e0`, 1),
				strings.Replace(repairCLIModelSelection, `"max_steps":2`, `"max_steps":17`, 1),
				strings.Replace(repairCLIModelSelection, `"max_output_tokens":100`, `"max_output_tokens":32769`, 1),
				strings.Replace(repairCLIModelSelection, `"max_steps":2`, `"max_steps":2,"max_steps":2`, 1),
				strings.Replace(repairCLIModelSelection, `"max_steps":2`, `"max_steps":2,"auto_retry":true`, 1),
				strings.Replace(repairCLIModelSelection, `"token_limit":65000`, `"token_limit":null`, 1),
			} {
				bad := strings.Replace(body, repairCLIModelSelection, selection, 1)
				code, _, _ = contextInvoke(t, s.URL, "en-US", bad, "candidate-compositions", action, "original", "--file", "-", "--confirm")
				if code != 2 || calls != 2 {
					t.Fatal("invalid model choice reached server", code, calls, selection)
				}
			}
		})
	}
}
