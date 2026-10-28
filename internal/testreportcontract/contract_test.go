package testreportcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

func reportFor(raw string) TestReport {
	h := sha256.Sum256([]byte(raw))
	return TestReport{Protocol: Protocol, XML: raw, SHA256: hex.EncodeToString(h[:])}
}

func TestJUnitReportPreservesCasesAndOriginalDigest(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?><testsuites tests="4" failures="1" errors="1" skipped="1"><testsuite name="suite" tests="4" failures="1" errors="1" skipped="1"><properties><property name="case_id" value="untrusted-not-authority"/></properties><testcase classname="orders.Export" name="allows current project" time="0.125"/><testsuite name="nested" tests="3" failures="1" errors="1" skipped="1"><testcase classname="orders.Export" name="denies other project" time="1.250"><failure message="Expected rejection"><![CDATA[Literal <script> and quoted 原文]]></failure></testcase><testcase classname="orders.Export" name="storage fault"><error message="Fixture disk fault"/></testcase><testcase classname="orders.Export" name="optional browser"><skipped message="No browser fixture"/></testcase></testsuite><system-out>Retained only in the original, not case identities</system-out></testsuite></testsuites>`
	report := reportFor(raw)
	parsed, err := Validate(report)
	if err != nil || parsed.Total != 4 || parsed.Passed != 1 || parsed.Failed != 1 || parsed.Errors != 1 || parsed.Skipped != 1 || len(parsed.Cases) != 4 {
		t.Fatal(parsed, err)
	}
	if parsed.Cases[0].Selector != (TestCaseSelector{ClassName: "orders.Export", Name: "allows current project"}) || parsed.Cases[0].DurationMS != 125 || parsed.Cases[1].DurationMS != 1250 || parsed.Cases[1].Status != "failed" || !strings.Contains(parsed.Cases[1].Message, "Literal <script> and quoted 原文") || report.XML != raw {
		t.Fatal("case projection or original bytes changed", parsed)
	}
	large := `<testsuite><testcase name="failed"><failure>` + strings.Repeat("中", 1500) + `</failure></testcase></testsuite>`
	bounded, err := Parse(large)
	if err != nil || len(bounded.Cases) != 1 || len(bounded.Cases[0].Message) > MaxMessageBytes || bounded.Failed != 1 {
		t.Fatal("large diagnostic did not keep bounded projection with original report", bounded, err)
	}
	t.Run("node-flat-cases-and-redundant-failure", func(t *testing.T) {
		// Node 22 emits testcase directly beneath testsuites and repeats the
		// failure child's message in an attribute. Only the child is an outcome.
		nodeRaw := `<?xml version="1.0" encoding="utf-8"?><testsuites><testcase name="export_contract" time="0.002253" classname="test" failure="Expected &lt;actual&gt; to equal expected"><failure type="testCodeFailure" message="Expected &lt;actual&gt; to equal expected">Original assertion &lt;stack&gt;</failure></testcase><testcase name="access_contract" time="0.000251" classname="test"/><!-- tests 2 --></testsuites>`
		nodeReport := reportFor(nodeRaw)
		node, err := Validate(nodeReport)
		if err != nil || node.Total != 2 || node.Passed != 1 || node.Failed != 1 || node.Errors != 0 || node.Skipped != 0 || len(node.Cases) != 2 {
			t.Fatal("Node's flat report lost known failure or passing case", node, err)
		}
		if node.Cases[0].Selector != (TestCaseSelector{ClassName: "test", Name: "export_contract"}) || node.Cases[0].Status != "failed" || node.Cases[0].DurationMS != 2 || node.Cases[0].Message != "Expected <actual> to equal expected\nOriginal assertion <stack>" || node.Cases[1].Status != "passed" || nodeReport.XML != nodeRaw {
			t.Fatal("Node case evidence or original XML changed", node)
		}
	})
}

func TestJUnitReportRejectsAmbiguousOrUnsafeXML(t *testing.T) {
	many := strings.Builder{}
	many.WriteString(`<testsuite>`)
	for index := 0; index <= MaxCases; index++ {
		many.WriteString(`<testcase name="case-` + strconv.Itoa(index) + `"/>`)
	}
	many.WriteString(`</testsuite>`)
	for name, raw := range map[string]string{
		"empty": "", "no-cases": `<testsuite tests="0"/>`,
		"dtd":                             `<!DOCTYPE testsuite [<!ENTITY report "injected">]><testsuite><testcase name="x">&report;</testcase></testsuite>`,
		"namespace":                       `<testsuite xmlns="https://untrusted.test"><testcase name="x"/></testsuite>`,
		"prefixed":                        `<s:testsuite xmlns:s="urn:test"><s:testcase name="x"/></s:testsuite>`,
		"duplicate-case":                  `<testsuite><testcase classname="A" name="x"/><testcase classname="A" name="x"/></testsuite>`,
		"duplicate-attribute":             `<testsuite><testcase name="x" name="y"/></testsuite>`,
		"contradictory-total":             `<testsuite tests="2"><testcase name="x"/></testsuite>`,
		"contradictory-status":            `<testsuite failures="0"><testcase name="x"><failure/></testcase></testsuite>`,
		"ambiguous-status":                `<testsuite><testcase name="x"><failure/><skipped/></testcase></testsuite>`,
		"missing-selector":                `<testsuite><testcase/></testsuite>`,
		"negative-time":                   `<testsuite><testcase name="x" time="-1"/></testsuite>`,
		"nan-time":                        `<testsuite><testcase name="x" time="NaN"/></testsuite>`,
		"trailing-root":                   `<testsuite><testcase name="x"/></testsuite><testsuite><testcase name="y"/></testsuite>`,
		"unknown-result-tag":              `<testsuite><testcase name="x"><command>approve</command></testcase></testsuite>`,
		"node-failure-attribute-only":     `<testsuites><testcase name="x" failure="broken"/></testsuites>`,
		"node-failure-attribute-mismatch": `<testsuites><testcase name="x" failure="broken"><failure message="different"/></testcase></testsuites>`,
		"node-failure-attribute-missing-child-message": `<testsuites><testcase name="x" failure=""><failure/></testcase></testsuites>`,
		"node-failure-attribute-error":                 `<testsuites><testcase name="x" failure="broken"><error message="broken"/></testcase></testsuites>`,
		"node-failure-attribute-skipped":               `<testsuites><testcase name="x" failure="broken"><skipped message="broken"/></testcase></testsuites>`,
		"node-unknown-status-attribute":                `<testsuites><testcase name="x" status="passed"/></testsuites>`,
		"node-contradictory-counter":                   `<testsuites tests="1" failures="0"><testcase name="x" failure="broken"><failure message="broken"/></testcase></testsuites>`,
		"node-duplicate-case":                          `<testsuites><testcase classname="test" name="x"/><testcase classname="test" name="x"/></testsuites>`,
		"oversized":                                    strings.Repeat(" ", MaxXMLBytes+1),
		"over-depth":                                   strings.Repeat(`<testsuite>`, 18) + `<testcase name="x"/>` + strings.Repeat(`</testsuite>`, 18),
		"over-cases":                                   many.String(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(raw); err == nil {
				t.Fatal("unsafe or ambiguous report accepted")
			}
		})
	}
}

func TestJUnitReportRequiresProtocolAndExactOriginalHash(t *testing.T) {
	raw := `<testsuite tests="1"><testcase classname="A" name="x"/></testsuite>`
	for _, mutate := range []func(*TestReport){func(r *TestReport) { r.Protocol = "foreign" }, func(r *TestReport) { r.SHA256 = strings.Repeat("0", 64) }, func(r *TestReport) { r.XML += "\n" }} {
		r := reportFor(raw)
		mutate(&r)
		if _, err := Validate(r); err == nil {
			t.Fatal("protocol or original XML digest not checked")
		}
	}
}
