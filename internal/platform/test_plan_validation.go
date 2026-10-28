package platform

import (
	"encoding/json"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	"strings"
	"unicode/utf8"
)

func testPlanSelectorKey(s tr.TestCaseSelector) string {
	return stringify([]string{s.ClassName, s.Name})
}
func testPlanText(v string, max int, required bool) bool {
	return utf8.ValidString(v) && len(v) <= max && !strings.ContainsRune(v, 0) && (!required || strings.TrimSpace(v) != "")
}
func testPlanSelectorText(v string, required bool) bool {
	if !testPlanText(v, 500, required) || strings.TrimSpace(v) != v {
		return false
	}
	for _, c := range v {
		if c < 32 || c >= 127 && c <= 159 {
			return false
		}
	}
	return true
}
func validateTestPlanDraft(title, description string, cases []QualityTestCase, source TestPlanSource, complete bool) error {
	if !testPlanText(title, 200, true) || !testPlanText(description, 4000, false) || cases == nil || len(cases) > 100 {
		return invalid(testPlanInvalid)
	}
	raw, err := json.Marshal(cases)
	if err != nil || len(raw) > 128*1024 {
		return invalid(testPlanInvalid)
	}
	keys, selectors, covered := map[string]bool{}, map[string]bool{}, map[int]bool{}
	for _, c := range cases {
		selector := testPlanSelectorKey(c.Selector)
		if !designKey(c.Key) || keys[c.Key] || !testPlanText(c.Title, 200, false) || !testPlanText(c.Description, 4000, false) || !testPlanText(c.Preconditions, 4000, false) || !testPlanText(c.ExpectedResult, 4000, false) || c.Steps == nil || len(c.Steps) > 20 || c.Covers == nil || len(c.Covers) > 100 || !testPlanSelectorText(c.Selector.ClassName, false) || !testPlanSelectorText(c.Selector.Name, complete) || c.Selector.Name != "" && selectors[selector] {
			return invalid(testPlanInvalid)
		}
		if c.Priority != "low" && c.Priority != "medium" && c.Priority != "high" && c.Priority != "urgent" {
			return invalid(testPlanInvalid)
		}
		keys[c.Key] = true
		if c.Selector.Name != "" {
			selectors[selector] = true
		}
		for _, step := range c.Steps {
			if !testPlanText(step, 1000, false) {
				return invalid(testPlanInvalid)
			}
		}
		seen := map[int]bool{}
		for _, index := range c.Covers {
			if index < 1 || index > len(source.Task.AcceptanceCriteria) || seen[index] {
				return invalid(testPlanInvalid)
			}
			seen[index], covered[index] = true, true
		}
		if complete {
			if !testPlanText(c.Title, 200, true) || !testPlanText(c.ExpectedResult, 4000, true) || len(c.Steps) == 0 {
				return invalid(testPlanIncomplete)
			}
			for _, step := range c.Steps {
				if strings.TrimSpace(step) == "" {
					return invalid(testPlanIncomplete)
				}
			}
			if len(source.Task.AcceptanceCriteria) > 0 && len(c.Covers) == 0 {
				return invalid(testPlanIncomplete)
			}
		}
	}
	if complete {
		if len(cases) == 0 {
			return invalid(testPlanIncomplete)
		}
		for i := range source.Task.AcceptanceCriteria {
			if !covered[i+1] {
				return invalid(testPlanIncomplete)
			}
		}
	}
	return nil
}
