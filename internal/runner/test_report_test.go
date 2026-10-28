package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

func reportFixture(t *testing.T, mode string) (ExecConfig, Assignment) {
	t.Helper()
	cfg, a, _, _ := candidateFixture(t)
	spec := cfg.Commands["verify"]
	spec.Args = []string{"-test.run=^TestQualityReportProcessHelper$", "--", "--report={{test_report_path}}"}
	spec.Env = map[string]string{"SUMA_JUNIT_HELPER": mode, "SUMA_REPORT_TEST_TOKEN": "owned-junit-fixture-secret-no-vendor"}
	if err := json.Unmarshal([]byte(`{"test_report":true}`), &spec); err != nil {
		t.Fatal(err)
	}
	cfg.Commands = map[string]CommandSpec{"verify": spec}
	_, executableSHA, err := commandExecutable(normalizedCommand(spec))
	if err != nil {
		t.Fatal(err)
	}
	profileSHA, err := effectiveCommandProfileDigest(cfg, normalizedCommand(spec), executableSHA)
	if err != nil {
		t.Fatal(err)
	}
	a.Config["tool_profile_id"] = "junit-report-fixture-profile"
	a.Config["tool_profile_sha256"] = profileSHA
	return cfg, a
}

func TestJUnitReportConfigurationAndObservedCapability(t *testing.T) {
	cfg, _ := reportFixture(t, "success")
	spec := cfg.Commands["verify"]
	raw, _ := json.Marshal(map[string]CommandSpec{"verify": spec})
	if _, err := DecodeCommands(raw); err != nil {
		t.Fatal("dedicated administrator test configuration rejected", err)
	}
	inventory, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatal(inventory, err)
	}
	wire, _ := json.Marshal(inventory[0].Profile)
	var fields map[string]any
	json.Unmarshal(wire, &fields)
	if fields["test_report_protocol"] != tr.Protocol {
		t.Fatal("observed report protocol missing", string(wire))
	}
	for _, mutate := range []func(*CommandSpec){func(s *CommandSpec) { s.TestEvidence = false }, func(s *CommandSpec) { s.ReadOnly = true }, func(s *CommandSpec) { s.Adapter = "suma"; s.Protocol = "suma-jsonl-v1" }, func(s *CommandSpec) { s.TestReport = false }} {
		copy := spec
		mutate(&copy)
		if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"verify": copy}}) == nil {
			t.Fatal("report capability accepted outside an administrator isolated command test")
		}
	}
}

func TestJUnitReportActualCandidateSuccessFailureAndRawReceipt(t *testing.T) {
	for _, mode := range []string{"success", "failed", "zero-exit-failed"} {
		t.Run(mode, func(t *testing.T) {
			cfg, a := reportFixture(t, mode)
			r := Execute(context.Background(), a, cfg)
			if r.TestReport == nil || r.OutcomeUnknown || r.Stopped {
				t.Fatal("missing actual report", r)
			}
			parsed, err := tr.Validate(*r.TestReport)
			if err != nil || parsed.Total != 2 || parsed.Passed != 1 || parsed.Cases[0].Selector.ClassName != "orders.Export" || r.WorkspaceEvidence["head_commit"] != a.CandidateCommit || r.WorkspaceEvidence["clean"] != true {
				t.Fatal(parsed, r, err)
			}
			if mode == "success" && (r.ExitCode != 0 || parsed.Skipped != 1) || mode == "failed" && (r.ExitCode != 1 || parsed.Failed != 1) {
				t.Fatal("real process outcome/case status changed", r, parsed)
			}
			if mode == "zero-exit-failed" && (r.ExitCode != 0 || parsed.Failed != 1 || r.Error != "test report includes failed or error cases" || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed") {
				t.Fatal("report failure overwrote real process facts or became success", r, parsed)
			}
			encoded, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Result
			if json.Unmarshal(encoded, &decoded) != nil || decoded.TestReport == nil || *decoded.TestReport != *r.TestReport {
				t.Fatal("original wire/journal report lost")
			}
			if strings.Contains(string(encoded), cfg.Workspace) || strings.Contains(r.TestReport.XML, "owned-junit-fixture-secret") {
				t.Fatal("local report paths or credentials leaked")
			}
			again := Execute(context.Background(), a, cfg)
			if again.ExitCode == 0 || again.TestReport != nil {
				t.Fatal("same attempt reexecuted report command")
			}
		})
	}
}

func TestJUnitReportCaptureRejectsMissingInvalidLinkedAndSensitive(t *testing.T) {
	for _, mode := range []string{"missing", "invalid", "hardlink", "sensitive"} {
		t.Run(mode, func(t *testing.T) {
			cfg, a := reportFixture(t, mode)
			r := Execute(context.Background(), a, cfg)
			if mode == "hardlink" && !strings.Contains(r.Output, "linked-report-created") {
				t.Fatal("the helper did not construct a real hardlink", r)
			}
			if r.ExitCode != 0 || r.TestReport != nil || !strings.Contains(strings.ToLower(r.Error), "report") || strings.Contains(r.Error, "owned-junit-fixture-secret") || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed" {
				t.Fatal("unverified report became success", r)
			}
		})
	}
}

func TestQualityReportProcessHelper(t *testing.T) {
	mode := os.Getenv("SUMA_JUNIT_HELPER")
	if mode == "" {
		return
	}
	raw := `<testsuite tests="2" failures="0" errors="0" skipped="1"><testcase classname="orders.Export" name="current project" time="0.125"/><testcase classname="orders.Export" name="optional browser"><skipped message="Fixture only"/></testcase></testsuite>`
	if mode == "failed" || mode == "zero-exit-failed" {
		raw = `<testsuite tests="2" failures="1" errors="0" skipped="0"><testcase classname="orders.Export" name="current project"/><testcase classname="orders.Export" name="cross project"><failure message="Actual assertion failed">Expected a refusal</failure></testcase></testsuite>`
	}
	path := os.Getenv("SUMA_TEST_REPORT_PATH")
	if path == "" || !filepath.IsAbs(path) {
		os.Exit(41)
	}
	argument := ""
	for _, value := range os.Args {
		if strings.HasPrefix(value, "--report=") {
			argument = strings.TrimPrefix(value, "--report=")
		}
	}
	if argument != path {
		os.Exit(45)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		os.Exit(42)
	}
	if mode == "missing" {
		os.Exit(0)
	}
	if mode == "invalid" {
		raw = `<testsuite tests="1"><testcase name="x"><failure/><skipped/></testcase></testsuite>`
	}
	if mode == "sensitive" {
		raw = `<testsuite><testcase name="x"/><system-out>` + os.Getenv("SUMA_REPORT_TEST_TOKEN") + `</system-out></testsuite>`
	}
	if mode == "hardlink" {
		source := filepath.Join(filepath.Dir(path), "original.xml")
		if os.WriteFile(source, []byte(raw), 0600) != nil || os.Link(source, path) != nil {
			os.Exit(43)
		}
		_, _ = os.Stdout.WriteString("linked-report-created\n")
	} else if os.WriteFile(path, []byte(raw), 0600) != nil {
		os.Exit(44)
	}
	if mode == "failed" {
		os.Exit(1)
	}
	os.Exit(0)
}
