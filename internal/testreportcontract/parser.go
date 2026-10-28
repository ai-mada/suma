package testreportcontract

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var errInvalidReport = errors.New("Invalid or unsupported JUnit test report")

// JUnit has many vendor dialects. This deliberately supports a documented
// subset instead of interpreting unknown tags or status attributes as success.
// XML directives, entities, namespaces and processing instructions cannot add
// content or authority. The standard decoder performs no external resource IO.
type reportElement struct {
	name       string
	attributes map[string]string
	children   []*reportElement
	text       string
}

type reportDecoder struct {
	decoder *xml.Decoder
	tokens  int
}

func (d *reportDecoder) token() (xml.Token, error) {
	d.tokens++
	if d.tokens > 50000 {
		return nil, errInvalidReport
	}
	return d.decoder.Token()
}

func (d *reportDecoder) element(start xml.StartElement, depth int) (*reportElement, error) {
	if depth > 16 || start.Name.Space != "" {
		return nil, errInvalidReport
	}
	node := &reportElement{name: start.Name.Local, attributes: map[string]string{}, children: []*reportElement{}}
	for _, attribute := range start.Attr {
		if attribute.Name.Space != "" || attribute.Name.Local == "xmlns" {
			return nil, errInvalidReport
		}
		if _, duplicate := node.attributes[attribute.Name.Local]; duplicate {
			return nil, errInvalidReport
		}
		node.attributes[attribute.Name.Local] = attribute.Value
	}
	var text strings.Builder
	for {
		token, err := d.token()
		if err != nil {
			return nil, errInvalidReport
		}
		switch value := token.(type) {
		case xml.StartElement:
			child, err := d.element(value, depth+1)
			if err != nil {
				return nil, err
			}
			node.children = append(node.children, child)
		case xml.EndElement:
			if value.Name.Space != "" || value.Name.Local != start.Name.Local {
				return nil, errInvalidReport
			}
			node.text = text.String()
			return node, nil
		case xml.CharData:
			text.Write(value)
		case xml.Comment:
			// Comments are inert original bytes, not selector or outcome data.
		default:
			return nil, errInvalidReport
		}
	}
}

// Parse never truncates XML or silently drops an unsupported test outcome.
// Diagnostic Message alone is a bounded UI projection; complete bytes remain
// in TestReport.XML and its original SHA256, outside ordinary run list payloads.
func Parse(raw string) (ParsedTestReport, error) {
	if len(raw) == 0 || len(raw) > MaxXMLBytes || !utf8.ValidString(raw) {
		return ParsedTestReport{}, errInvalidReport
	}
	input := strings.TrimPrefix(raw, "\ufeff")
	d := reportDecoder{decoder: xml.NewDecoder(strings.NewReader(input))}
	var root *reportElement
	declaration := false
	for {
		token, err := d.token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ParsedTestReport{}, errInvalidReport
		}
		switch value := token.(type) {
		case xml.StartElement:
			if root != nil || value.Name.Local != "testsuite" && value.Name.Local != "testsuites" {
				return ParsedTestReport{}, errInvalidReport
			}
			root, err = d.element(value, 1)
			if err != nil {
				return ParsedTestReport{}, err
			}
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return ParsedTestReport{}, errInvalidReport
			}
		case xml.ProcInst:
			if root != nil || declaration || value.Target != "xml" {
				return ParsedTestReport{}, errInvalidReport
			}
			declaration = true
		case xml.Comment:
		default:
			return ParsedTestReport{}, errInvalidReport
		}
	}
	if root == nil {
		return ParsedTestReport{}, errInvalidReport
	}
	projection := ParsedTestReport{Cases: []TestReportCase{}}
	counts, err := projectSuite(root, &projection, map[TestCaseSelector]bool{})
	if err != nil || counts.Total == 0 {
		return ParsedTestReport{}, errInvalidReport
	}
	projection.Total, projection.Passed, projection.Failed, projection.Errors, projection.Skipped = counts.Total, counts.Passed, counts.Failed, counts.Errors, counts.Skipped
	return projection, nil
}

func reportFields(node *reportElement, allowed ...string) bool {
	for name := range node.attributes {
		found := false
		for _, candidate := range allowed {
			if name == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func addReportCounts(total *ParsedTestReport, child ParsedTestReport) {
	total.Total += child.Total
	total.Passed += child.Passed
	total.Failed += child.Failed
	total.Errors += child.Errors
	total.Skipped += child.Skipped
}

func projectSuite(node *reportElement, out *ParsedTestReport, seen map[TestCaseSelector]bool) (ParsedTestReport, error) {
	counts := ParsedTestReport{}
	if node.name != "testsuite" && node.name != "testsuites" || strings.TrimSpace(node.text) != "" || !reportFields(node, "name", "tests", "failures", "errors", "skipped", "disabled", "time", "timestamp", "hostname", "id", "package", "file", "assertions") {
		return counts, errInvalidReport
	}
	if _, err := reportDuration(node.attributes["time"]); err != nil {
		return counts, err
	}
	for _, child := range node.children {
		switch child.name {
		case "testsuite":
			value, err := projectSuite(child, out, seen)
			if err != nil {
				return counts, err
			}
			addReportCounts(&counts, value)
		case "testcase":
			// Node's built-in JUnit reporter can place cases directly under
			// testsuites. They share the same global identity and count limits.
			if len(out.Cases) >= MaxCases {
				return counts, errInvalidReport
			}
			value, err := projectCase(child)
			if err != nil || seen[value.Selector] {
				return counts, errInvalidReport
			}
			seen[value.Selector] = true
			out.Cases = append(out.Cases, value)
			counts.Total++
			switch value.Status {
			case "passed":
				counts.Passed++
			case "failed":
				counts.Failed++
			case "error":
				counts.Errors++
			case "skipped":
				counts.Skipped++
			}
		case "properties", "system-out", "system-err":
			if !reportMetadata(child) {
				return counts, errInvalidReport
			}
		default:
			return counts, errInvalidReport
		}
	}
	for name, actual := range map[string]int{"tests": counts.Total, "failures": counts.Failed, "errors": counts.Errors, "skipped": counts.Skipped, "disabled": counts.Skipped} {
		if raw, declared := node.attributes[name]; declared {
			if raw == "" || strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return counts, errInvalidReport
			}
			value, err := strconv.Atoi(raw)
			if err != nil || value != actual {
				return counts, errInvalidReport
			}
		}
	}
	return counts, nil
}

func validSelectorText(text string, empty bool) bool {
	return (empty || text != "") && len(text) <= 500 && strings.TrimSpace(text) == text && strings.IndexFunc(text, unicode.IsControl) < 0
}

func projectCase(node *reportElement) (TestReportCase, error) {
	result := TestReportCase{Selector: TestCaseSelector{ClassName: node.attributes["classname"], Name: node.attributes["name"]}, Status: "passed"}
	if !reportFields(node, "classname", "name", "time", "file", "line", "assertions", "failure") || strings.TrimSpace(node.text) != "" || !validSelectorText(result.Selector.ClassName, true) || !validSelectorText(result.Selector.Name, false) {
		return result, errInvalidReport
	}
	var err error
	result.DurationMS, err = reportDuration(node.attributes["time"])
	if err != nil {
		return result, err
	}
	outcome := false
	failureAttribute, hasFailureAttribute := node.attributes["failure"]
	failureAttributeMatched := false
	for _, child := range node.children {
		switch child.name {
		case "failure", "error", "skipped":
			if outcome || len(child.children) > 0 || !reportFields(child, "message", "type") {
				return result, errInvalidReport
			}
			outcome = true
			result.Status = child.name
			if child.name == "failure" {
				result.Status = "failed"
				if message, present := child.attributes["message"]; hasFailureAttribute && present && message == failureAttribute {
					failureAttributeMatched = true
				}
			}
			message := strings.TrimSpace(child.attributes["message"])
			body := strings.TrimSpace(child.text)
			if body != "" {
				if message != "" {
					message += "\n"
				}
				message += body
			}
			if len(message) > MaxMessageBytes {
				message = message[:MaxMessageBytes]
				for !utf8.ValidString(message) {
					message = message[:len(message)-1]
				}
			}
			result.Message = message
		case "properties", "system-out", "system-err":
			if !reportMetadata(child) {
				return result, errInvalidReport
			}
		default:
			return result, errInvalidReport
		}
	}
	// Node's failure attribute is redundant metadata, never an outcome by
	// itself. Require the actual failure child and its exact decoded message;
	// attribute-only or contradictory reports cannot silently become passed.
	if hasFailureAttribute && !failureAttributeMatched {
		return result, errInvalidReport
	}
	return result, nil
}

func reportMetadata(node *reportElement) bool {
	if node.name == "system-out" || node.name == "system-err" {
		return len(node.children) == 0 && len(node.attributes) == 0
	}
	if node.name != "properties" || len(node.attributes) > 0 || strings.TrimSpace(node.text) != "" {
		return false
	}
	for _, property := range node.children {
		if property.name != "property" || len(property.children) > 0 || !reportFields(property, "name", "value") {
			return false
		}
	}
	return true
}

func reportDuration(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	periods := 0
	for index, r := range raw {
		if r == '.' {
			periods++
			if periods > 1 || index == 0 || index == len(raw)-1 {
				return 0, errInvalidReport
			}
		} else if r < '0' || r > '9' {
			return 0, errInvalidReport
		}
	}
	value, err := time.ParseDuration(raw + "s")
	if err != nil || value < 0 || value > 24*time.Hour {
		return 0, errInvalidReport
	}
	return value.Milliseconds(), nil
}
