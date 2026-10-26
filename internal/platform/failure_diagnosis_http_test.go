package platform

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestFailureDiagnosisHTTPIngressAndPersistentRepairLink(t *testing.T) {
	h := newDiagnosisFixture(t)
	f := h.f
	location := fmt.Sprintf("project_id=%s&run_id=%s&node_id=%s&attempt=1", f.project, h.request.RunID, h.request.NodeID)
	for _, suffix := range []string{"&attempt=2", "&unknown=1", "&limit=1"} {
		f.call("GET", "/failure-diagnosis/context?"+location+suffix, nil, 400)
	}
	for _, attempt := range []string{"01", "1e0", "1.0", "0", "2147483647"} {
		f.call("GET", "/failure-diagnosis/context?"+strings.Replace(location, "attempt=1", "attempt="+attempt, 1), nil, 400)
	}
	f.call("GET", "/issues/"+h.request.IssueID+"/intake?project_id="+f.project, nil, 404)
	raw := stringify(h.request)
	for _, pair := range []struct{ name, value string }{{"max_steps", "null"}, {"max_steps", "1.0"}, {"max_output_tokens", "8193"}, {"attempt", "1e0"}, {"expected_issue_version", "1.0"}} {
		body := strings.TrimSuffix(raw, "}") + `,"` + pair.name + `":` + pair.value + `}`
		f.call("POST", "/failure-diagnosis/analyses", json.RawMessage(body), 400)
	}
	created := f.call("POST", "/failure-diagnosis/analyses", h.request, 201)
	id := created["id"].(string)
	f.call("GET", "/failure-diagnosis/analyses?"+location+"&limit=1&limit=2", nil, 400)
	page := f.call("GET", "/failure-diagnosis/analyses?"+location, nil, 200)
	if page["total"] != float64(1) {
		t.Fatal("invalid ingress created a paid execution", page)
	}
	a := h.claim(t)
	h.complete(t, h.result(a, h.output))
	ready := h.get(t, id)
	command := h.command(ready, "adopt")
	badNull := strings.TrimSuffix(stringify(command), "}") + `,"repair":null}`
	f.call("POST", "/failure-diagnosis/analyses/"+id+"/commands", json.RawMessage(badNull), 400)
	adopted := f.call("POST", "/failure-diagnosis/analyses/"+id+"/commands", command, 200)
	key := adopted["adopted_requirement_id"].(string)
	requirement := f.call("GET", "/requirements/"+key+"?project_id="+f.project, nil, 200)
	if requirement["status"] != "draft" || requirement["failure_diagnosis_adoption"] == nil {
		t.Fatal("requirement read lost authoritative adoption", requirement)
	}
	opened := f.call("GET", "/issues/"+h.request.IssueID+"/intake?project_id="+f.project, nil, 200)
	if opened["id"] != key || opened["failure_diagnosis_adoption"] == nil {
		t.Fatal("refresh cannot open exact existing repair", opened)
	}
	f.call("GET", "/issues/"+h.request.IssueID+"/intake?project_id="+f.project+"&project_id="+f.project, nil, 400)
	forged := f.body(map[string]any{"title": "Forged proof", "failure_diagnosis_adoption": map[string]any{"analysis_id": id}})
	f.call("POST", "/requirements", forged, 400)
	forged = f.body(map[string]any{"title": "Forged data proof", "data": map[string]any{"failure_diagnosis_adoption": map[string]any{"analysis_id": id}}})
	f.call("POST", "/requirements", forged, 400)
	f.call("PATCH", "/requirements/"+key, f.body(map[string]any{"expected_version": requirement["version"], "data": map[string]any{"failure_diagnosis_adoption": nil}}), 400)
}

func TestFailureDiagnosisPresentationPreservesLiteralSourcesAndHumanReasons(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Language", "zh-CN")
	source := FailureDiagnosisSource{Evidence: []FailureDiagnosisEvidence{{Kind: "error", Text: diagnosisSource}}}
	proposal := &FailureDiagnosisOutput{Summary: diagnosisFailed}
	raw := FailureDiagnosisAnalysis{State: "ready", BasisCurrent: true, Reason: diagnosisSource, Source: source, Proposal: proposal, OriginalOutput: diagnosisUnknown}
	copy := localizePresentation(w, raw).(FailureDiagnosisAnalysis)
	if !reflect.DeepEqual(raw, copy) {
		t.Fatal("human decision or frozen original was translated")
	}
	raw.State, raw.Reason = "failed", diagnosisFailed
	copy = localizePresentation(w, raw).(FailureDiagnosisAnalysis)
	if copy.Reason != raw.Reason || copy.ReasonDiagnostic == nil || copy.ReasonDiagnostic.Text == raw.Reason || !reflect.DeepEqual(raw.Source, copy.Source) || copy.Proposal.Summary != diagnosisFailed || copy.OriginalOutput != diagnosisUnknown {
		t.Fatal("controlled warning or immutable source projection incorrect", copy)
	}
	context := FailureDiagnosisContext{Blockers: []string{diagnosisUncertain}, Source: &source}
	translated := localizePresentation(w, context).(FailureDiagnosisContext)
	if translated.Blockers[0] != context.Blockers[0] || translated.BlockerDiagnostics[0] == nil || translated.BlockerDiagnostics[0].Text == diagnosisUncertain || context.Blockers[0] != diagnosisUncertain || translated.Source.Evidence[0].Text != diagnosisSource {
		t.Fatal("context projection mutated its source")
	}
}

func TestFailureDiagnosisUsesFixedModelPricingAndRejectsMixedPurpose(t *testing.T) {
	p := toolTestProfile()
	p.ReadOnly = true
	p.FailureDiagnosis = &toolcontract.FailureDiagnosisCapability{Protocol: toolcontract.FailureDiagnosisProtocol, Model: "fixed-diagnosis", EndpointOrigin: "https://model.example"}
	profile := ToolProfile{State: "enabled", Profile: p}
	model, origin, ok := usagePricedModel(profile)
	if !ok || model != p.FailureDiagnosis.Model || origin != p.FailureDiagnosis.EndpointOrigin {
		t.Fatal("diagnosis bypassed fixed model price identity", model, origin, ok)
	}
	profile.Profile.Planning = &toolcontract.PlanningCapability{Protocol: toolcontract.PlanningProtocol, Model: "other", EndpointOrigin: "https://other.example"}
	if _, _, ok := usagePricedModel(profile); ok {
		t.Fatal("mixed purpose gained accounting identity")
	}
}
