package cli

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/platform"
)

// This is a read-only presentation of a validated server snapshot. Keep the
// dispatch prerequisite action and maintenance occupancy recommendation in
// separate sections; neither is a command or a grant to follow automatically.
func writeToolDoctorTable(out io.Writer, report platform.ToolProfileDiagnostics, locale localization.Locale, token string) error {
	text := func(en, zh string) string {
		if locale == localization.EnUS {
			return en
		}
		return zh
	}
	quoted := func(value string) string { return strconv.Quote(value) }
	label := func(code string) string {
		labels := map[string][2]string{
			"authorization": {"Configuration authorization", "配置授权"}, "worker": {"Worker identity", "节点身份"}, "inventory": {"Inventory freshness", "库存时效"}, "profile": {"Exact configuration", "准确配置"},
			"pass": {"Pass", "通过"}, "blocked": {"Blocked", "阻断"}, "not_checked": {"Not checked", "未检查"},
			"none": {"No action", "无操作"}, "review_profile": {"Review profile", "审阅配置"}, "open_worker": {"View exact worker", "查看准确节点"}, "register_profile": {"Register configuration", "登记配置"}, "refresh": {"Read current report", "重读当前报告"},
			"inspect": {"Request node recheck", "请求节点重新检查"}, "view_inspection": {"View original inspection", "查看原检查"}, "recover_inspection": {"Review unknown inspection", "核对未知检查"}, "view_recovery": {"View original recovery", "查看原核对"},
			"inspection_pending": {"Waiting for node inspection", "等待节点检查"}, "inspection_claimed": {"Inspection claimed by node", "节点已领取检查"}, "inspection_unknown": {"Inspection result unknown", "检查结果未知"},
			"recovery_requested": {"Recovery review requested", "已请求核对"}, "recovery_proof_stored": {"Original stop proof stored", "原停止证明已保存"}, "recovery_expired": {"Recovery review expired", "核对授权已过期"},
			"maintenance_supported": {"Maintenance protocol supported", "支持维护协议"}, "maintenance_expired": {"Maintenance observation expired", "维护观察已过期"}, "maintenance_unsupported": {"Maintenance protocol unsupported", "不支持维护协议"}, "maintenance_unknown": {"Maintenance capability unknown", "维护能力未知"},
			"profile_revoked": {"Profile revoked", "配置已撤销"}, "worker_unavailable": {"Worker authorization unavailable", "节点当前授权不可用"},
		}
		pair, ok := labels[code]
		if !ok {
			return code
		}
		return text(pair[0], pair[1]) + " (" + code + ")"
	}
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, text("Tool diagnostics · read-only", "工具体检 · 只读"))
	fmt.Fprintf(&buffer, "%s: %s\n%s: %s · %s: %d\n%s: %s\n", text("Project", "项目"), quoted(report.Tool.ProjectID), text("Profile", "配置"), quoted(report.Tool.ID), text("Version", "版本"), report.Tool.Version, text("Worker", "节点"), quoted(report.Tool.WorkerID))
	availability := text("Unavailable", "不可用")
	if report.Tool.Available {
		availability = text("Available", "可用")
	}
	fmt.Fprintf(&buffer, "%s: %s\n%s: %s\n\n", text("Dispatch prerequisites", "派发前提"), availability, text("Original next action", "原下一步"), label(report.NextAction))
	rows := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
	fmt.Fprintf(rows, "%s\t%s\t%s\n", text("Check", "检查"), text("State", "状态"), text("Reason", "原因"))
	for _, check := range report.Checks {
		fmt.Fprintf(rows, "%s\t%s\t%s\n", label(check.Code), label(check.Status), quoted(check.Reason))
	}
	if err := rows.Flush(); err != nil {
		return err
	}
	resolution := report.MaintenanceResolution
	fmt.Fprintf(&buffer, "\n%s\n%s: %s\n%s: %s\n", text("Maintenance suggestion", "维护建议"), text("Suggested action", "建议动作"), label(resolution.Action), text("Reason", "原因"), label(resolution.Reason))
	fmt.Fprintf(&buffer, "%s: %s · %s: %d\n%s: %s\n", text("Current profile", "当前配置"), quoted(resolution.ProfileID), text("Version", "版本"), resolution.ProfileVersion, text("Target worker", "目标节点"), quoted(resolution.WorkerID))
	if resolution.InspectionID != "" {
		fmt.Fprintf(&buffer, "%s: %s\n", text("Original inspection", "原检查"), quoted(resolution.InspectionID))
	}
	if resolution.RecoveryID != "" {
		fmt.Fprintf(&buffer, "%s: %s\n", text("Original recovery", "原核对"), quoted(resolution.RecoveryID))
	}
	if resolution.InspectionID != "" {
		fmt.Fprintln(&buffer, text("Read the original inspection before acting; it may belong to another profile on this worker.", "后续操作前须读取原检查；它可能属于该节点上的其他配置。"))
	}
	fmt.Fprintf(&buffer, "%s: %s\n%s: %s\n", text("Checked at", "检查时间"), quoted(report.CheckedAt), text("Display expires at", "展示到期时间"), quoted(report.ExpiresAt))
	fmt.Fprintln(&buffer, text("Suggestions locate original records only. No inspection, recovery, enable, stop or repair request was sent. A current read and explicit authorization are required for any later action.", "建议仅定位原记录；未发送检查、核对、启用、停止或修复请求。后续操作仍需读取当前状态并明确授权。"))
	value := buffer.String()
	if token != "" {
		value = strings.ReplaceAll(value, token, "[redacted]")
	}
	_, err := io.WriteString(out, value)
	return err
}
