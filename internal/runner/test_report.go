package runner

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

func validateTestReportSpec(spec CommandSpec) error {
	for _, argument := range spec.Args {
		if strings.Contains(argument, "{{test_report_path}}") && !spec.TestReport {
			return fmt.Errorf("test report path placeholders require an enabled report profile")
		}
	}
	if !spec.TestReport {
		return nil
	}
	spec = normalizedCommand(spec)
	if spec.Adapter != "command" || spec.Protocol != "text-v1" || spec.ReadOnly || !spec.TestEvidence || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.CandidateComposition || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.NativeWorkspace != nil || spec.ModelExecutionProtocol != "" || spec.CodexAppServer != nil || spec.OpenCode != nil {
		return fmt.Errorf("test reports require an isolated administrator-designated command test")
	}
	for name := range spec.Env {
		if strings.EqualFold(name, "SUMA_TEST_REPORT_PATH") {
			return fmt.Errorf("test report paths are assigned by the Runner, not command configuration")
		}
	}
	for _, name := range spec.EnvFrom {
		if strings.EqualFold(name, "SUMA_TEST_REPORT_PATH") {
			return fmt.Errorf("test report paths are assigned by the Runner, not command configuration")
		}
	}
	return nil
}

func testReportProtocol(spec CommandSpec) string {
	if spec.TestReport {
		return tr.Protocol
	}
	return ""
}

type testReportCapture struct {
	path        string
	parents     []artifactParent
	failedCases bool
}

// Allocate outside the Git source so report generation cannot dirty the tested
// commit. The directory and file are fresh for this exact attempt; no workflow
// path, existing report, local environment or report filename can select input.
func prepareTestReport(workspace *attemptWorkspace) (*testReportCapture, error) {
	if workspace == nil {
		return nil, fmt.Errorf("test reports require an isolated administrator-designated command test")
	}
	dir := filepath.Join(workspace.Directory, "test-report")
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("test report directory is not fresh or is unavailable")
	}
	file := filepath.Join(dir, "report.xml")
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		return nil, fmt.Errorf("test report directory is not fresh or is unavailable")
	}
	parents, err := artifactParents(workspace.Directory, file)
	if err != nil {
		return nil, fmt.Errorf("test report directory is not fresh or is unavailable")
	}
	return &testReportCapture{path: file, parents: parents}, nil
}

// Read only after confirmed process-tree completion. File/parent identity and
// hardlink checks prevent a producer from substituting an older or external
// file. These checks do not attest an administrator's test implementation.
func (capture *testReportCapture) collect(secrets []string) (*tr.TestReport, error) {
	invalid := func() (*tr.TestReport, error) { return nil, fmt.Errorf("test report is missing, invalid or unsafe") }
	if capture == nil || !artifactParentsStable(capture.parents) {
		return invalid()
	}
	before, err := os.Lstat(capture.path)
	if err != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > tr.MaxXMLBytes {
		return invalid()
	}
	file, err := os.Open(capture.path)
	if err != nil {
		return invalid()
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !artifactSingleLink(file) {
		file.Close()
		return invalid()
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, tr.MaxXMLBytes+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	last, lastErr := os.Lstat(capture.path)
	if readErr != nil || statErr != nil || closeErr != nil || lastErr != nil || int64(len(raw)) != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(opened, last) || last.Mode()&os.ModeSymlink != 0 || !artifactParentsStable(capture.parents) {
		return invalid()
	}
	report := &tr.TestReport{Protocol: tr.Protocol, XML: string(raw), SHA256: digest(raw)}
	parsed, err := tr.Validate(*report)
	if err != nil {
		return invalid()
	}
	capture.failedCases = parsed.Failed > 0 || parsed.Errors > 0
	if testReportSensitive(report.XML, secrets) {
		return nil, fmt.Errorf("test report contains a configured secret and was retained locally")
	}
	return report, nil
}

// The unchanged original is rejected rather than redacted into a new purported
// original. Check decoded XML too so an escaped configured value cannot evade
// the same limited known-secret boundary. This is not a general DLP classifier.
func testReportSensitive(raw string, secrets []string) bool {
	contains := func(value string) bool {
		for _, secret := range secrets {
			if secret != "" && strings.Contains(value, secret) {
				return true
			}
		}
		return false
	}
	if contains(raw) {
		return true
	}
	decoder := xml.NewDecoder(strings.NewReader(raw))
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return true
		}
		switch value := token.(type) {
		case xml.CharData:
			text.Write(value)
		case xml.StartElement:
			for _, attribute := range value.Attr {
				if contains(attribute.Value) {
					return true
				}
			}
		}
	}
	return contains(text.String())
}
