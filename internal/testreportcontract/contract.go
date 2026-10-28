// Package testreportcontract defines the bounded original JUnit report and its
// deterministic case projection. A report is execution evidence, not approval.
package testreportcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const Protocol = "suma-junit-report-v1"
const MaxXMLBytes = 256 * 1024
const MaxCases = 1000
const MaxMessageBytes = 3000

type TestReport struct {
	Protocol string `json:"protocol"`
	XML      string `json:"xml"`
	SHA256   string `json:"sha256"`
}

type TestCaseSelector struct {
	ClassName string `json:"class_name"`
	Name      string `json:"name"`
}

type TestReportCase struct {
	Selector   TestCaseSelector `json:"selector"`
	Status     string           `json:"status"`
	Message    string           `json:"message"`
	DurationMS int64            `json:"duration_ms"`
}

type ParsedTestReport struct {
	Cases   []TestReportCase `json:"cases"`
	Total   int              `json:"total"`
	Passed  int              `json:"passed"`
	Failed  int              `json:"failed"`
	Errors  int              `json:"errors"`
	Skipped int              `json:"skipped"`
}

// Validate binds the projection to the complete, unchanged report bytes. The
// XML cannot declare its own plan version, case identity or acceptance rights.
func Validate(report TestReport) (ParsedTestReport, error) {
	hash := sha256.Sum256([]byte(report.XML))
	if report.Protocol != Protocol || report.SHA256 != hex.EncodeToString(hash[:]) {
		return ParsedTestReport{}, errors.New("JUnit report protocol or original hash is invalid")
	}
	return Parse(report.XML)
}
