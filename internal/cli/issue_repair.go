package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	"github.com/ai-mada/suma/internal/toolcontract"
)

const issueRepairCLIUsage = "issue-repair requires show ISSUE with --project. Use issue-repair --help."
const issueRepairCLIResponse = "issue repair response did not match the requested issue, project or original case contract"

var issueRepairCaseRunID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func issueRepairCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") ||
		len(args) == 2 && args[0] == "show" && args[1] == "--help" {
		_, err := io.WriteString(out, c.text(
			"  issue-repair show ISSUE   读取当前项目的缺陷修复及原始用例引用（将 --project ID 放在命令前）\n",
			"  issue-repair show ISSUE   Read issue repair and its original case reference (put --project ID before the command).\n",
		))
		return err
	}
	if len(args) != 2 || args[0] != "show" || !compositionCLIIdentifier(args[1], 64) ||
		strings.HasPrefix(args[1], "-") || strings.Contains(args[1], "..") {
		return usage(issueRepairCLIUsage)
	}

	issue := args[1]
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/issues/"+url.PathEscape(issue)+"/repair", nil, true)
	if err != nil {
		return err
	}
	if !validIssueRepairResponse(raw, c.project, issue) {
		return &failure{Code: "response", Message: issueRepairCLIResponse}
	}
	return writeJSON(out, raw, c.token)
}

func validIssueRepairResponse(raw json.RawMessage, project, issue string) bool {
	if !toolcontract.ValidJSONUnicode(raw) {
		return false
	}
	fields := responseObjectFields(raw)
	record := responseObjectFields(fields["issue"])
	if !recommendationHistoryCanonicalFields(fields, "issue") ||
		!recommendationHistoryCanonicalFields(record, "id", "project_id", "version") {
		return false
	}
	var id, projectID string
	var version int
	if json.Unmarshal(record["id"], &id) != nil || id != issue ||
		json.Unmarshal(record["project_id"], &projectID) != nil || projectID != project ||
		json.Unmarshal(record["version"], &version) != nil || version < 1 {
		return false
	}

	for name := range fields {
		if name != "original_case" && strings.EqualFold(name, "original_case") {
			return false
		}
	}
	source, ok := fields["original_case"]
	if !ok {
		return true
	}
	var origin platform.IssueRepairCaseOrigin
	return strictjson.Decode(source, &origin, 1024) == nil &&
		testPlanCatalogID.MatchString(origin.PlanID) && issueRepairCaseRunID.MatchString(origin.RunID) &&
		testPlanCatalogID.MatchString(origin.CaseKey) && origin.PlanRevision >= 1 && origin.PlanRevision < 2147483647 &&
		toolcontract.ValidDigest(origin.PlanSHA256)
}
