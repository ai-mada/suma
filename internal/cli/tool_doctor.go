package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const toolDoctorUsage = "tools doctor requires --id PROFILE_ID and --project; --format must be json or table; use tools --help"
const toolDoctorResponseInvalid = "tool diagnostics response must preserve the requested project, exact complete tool, four checks, exact maintenance resolution and a current display deadline of at most 60 seconds"

func toolDoctorID(id string) bool {
	return len(id) <= 100 && contextID(id) && !strings.ContainsFunc(id, unicode.IsControl)
}

func toolDoctorCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	flags := commandFlags("tools doctor")
	id := flags.String("id", "", "exact existing tool profile ID")
	format := flags.String("format", "json", "json or table")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !toolDoctorID(*id) || !toolDoctorID(c.project) || *format != "json" && *format != "table" {
		return usage(toolDoctorUsage)
	}
	// Capture this invocation's source before the read. This report never
	// authorizes another request or follows any returned management action.
	project, toolID := c.project, *id
	started := time.Now()
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/tool-profiles/"+toolID+"/diagnostics", nil, true)
	if err != nil {
		return err
	}
	var report platform.ToolProfileDiagnostics
	if strictjson.Decode(raw, &report, 16<<20) != nil || !validToolDiagnostics(report, project, toolID, time.Since(started)) || !toolDoctorMaintenanceIDsPresent(raw) {
		return &failure{Code: "response", Message: toolDoctorResponseInvalid}
	}
	if *format == "table" {
		return writeToolDoctorTable(out, report, c.locale, c.token)
	}
	return writeJSON(out, raw, c.token)
}

// The shared value validator owns identity, ID shape and the closed matrix.
// Retain only this wire-presence check locally: optional string decoding alone
// cannot distinguish an omitted ID from an explicitly empty one.
func toolDoctorMaintenanceIDsPresent(raw []byte) bool {
	var envelope struct {
		MaintenanceResolution map[string]json.RawMessage `json:"maintenance_resolution"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.MaintenanceResolution == nil {
		return false
	}
	for _, key := range []string{"inspection_id", "recovery_id"} {
		if value, present := envelope.MaintenanceResolution[key]; present {
			var id string
			if json.Unmarshal(value, &id) != nil || id == "" {
				return false
			}
		}
	}
	return true
}

func validToolDiagnostics(report platform.ToolProfileDiagnostics, project, id string, elapsed time.Duration) bool {
	tool := report.Tool
	if tool.ID != id || tool.ProjectID != project || !tc.ValidCommandID(tool.WorkerID) || strings.TrimSpace(tool.Name) == "" || tool.Version < 1 || tc.ValidateProfile(tool.Profile, true) != nil || (tool.State != "paused" && tool.State != "enabled" && tool.State != "revoked") {
		return false
	}
	if !platform.ValidToolProfileMaintenanceResolution(report.MaintenanceResolution, tool) {
		return false
	}
	for _, stamp := range []string{tool.CreatedAt, tool.UpdatedAt} {
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			return false
		}
	}
	checked, err := time.Parse(time.RFC3339Nano, report.CheckedAt)
	if err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339Nano, report.ExpiresAt)
	if err != nil || !expires.After(checked) || expires.Sub(checked) > time.Minute || elapsed < 0 || elapsed >= expires.Sub(checked) {
		return false
	}
	if len(report.Checks) != 4 || !toolDiagnosticAction(report.NextAction) {
		return false
	}
	firstAction := "none"
	allPass := true
	for index, code := range []string{"authorization", "worker", "inventory", "profile"} {
		check := report.Checks[index]
		if check.Code != code || strings.TrimSpace(check.Reason) == "" || !toolDiagnosticAction(check.NextAction) {
			return false
		}
		switch check.Status {
		case "pass":
			if check.NextAction != "none" {
				return false
			}
		case "blocked":
			allPass = false
			if check.NextAction == "none" {
				return false
			}
			if firstAction == "none" {
				firstAction = check.NextAction
			}
		case "not_checked":
			allPass = false
			if index != 3 || report.Checks[2].Status != "blocked" || check.NextAction != report.Checks[2].NextAction {
				return false
			}
		default:
			return false
		}
	}
	if report.NextAction != firstAction || tool.Available != allPass {
		return false
	}
	if report.Checks[2].Status == "pass" {
		seen, err := time.Parse(time.RFC3339Nano, tool.LastSeenAt)
		if err != nil || seen.After(checked) || !checked.Before(seen.Add(time.Minute)) || expires.After(seen.Add(time.Minute)) {
			return false
		}
	} else if report.Checks[3].Status != "not_checked" {
		return false
	}
	if report.Checks[3].Status == "pass" && (tool.Observation == nil || !tool.Observation.Available || !tc.EqualProfile(tool.Observation.Profile, tool.Profile)) {
		return false
	}
	return true
}

func toolDiagnosticAction(action string) bool {
	switch action {
	case "none", "review_profile", "open_worker", "register_profile", "refresh":
		return true
	}
	return false
}
