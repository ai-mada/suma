package dag

import (
	"strings"
	"testing"

	tr "github.com/ai-mada/suma/internal/testreportcontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestJUnitReportFixedToolBindingAndReadOnlyIsolation(t *testing.T) {
	n := toolNode()
	n.Type = "test"
	if _, _, bound, err := ToolBinding(n); err != nil || !bound {
		t.Fatal("test command cannot pin its observed profile", bound, err)
	}
	xml := `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`
	profile := toolcontract.Profile{CommandID: n.Command[0], Adapter: "command", Protocol: "text-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "local-command", TestReportProtocol: tr.Protocol}
	r := Result{TestReport: &tr.TestReport{Protocol: tr.Protocol, XML: xml, SHA256: hashText(xml)}, ToolEvidence: &toolcontract.Evidence{Profile: profile, Terminal: "completed", UsageState: toolcontract.UsageUnreported}}
	if err := validateToolResult(n, r); err != nil {
		t.Fatal(err)
	}
	if noCompletionEffects(r) {
		t.Fatal("report gained a read-only model completion path")
	}
	for _, mutation := range []func(*Node, *Result){
		func(n *Node, r *Result) { n.Type = "agent" },
		func(n *Node, r *Result) { r.ToolEvidence.Profile.TestReportProtocol = "" },
		func(n *Node, r *Result) { r.TestReport.SHA256 = strings.Repeat("c", 64) },
	} {
		copyNode, copyResult := n, r
		report, evidence := *r.TestReport, *r.ToolEvidence
		copyResult.TestReport, copyResult.ToolEvidence = &report, &evidence
		mutation(&copyNode, &copyResult)
		if validateToolResult(copyNode, copyResult) == nil {
			t.Fatal("unbound or forged report accepted")
		}
	}
}
