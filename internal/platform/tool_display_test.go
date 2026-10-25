package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// Removing producer metadata, attaching it to the wrong branch, or deriving it
// from translated text must fail these real authorized reads.
func TestToolDisplayEligibilityBranches(t *testing.T) {
	cases := []struct{ key, reason string }{
		{"profile_revoked", "工具档案已永久撤销"}, {"profile_paused", "工具档案已暂停，尚未授权派发"},
		{"activation_revoked", "工具档案启用人的当前审批权限已失效"}, {"worker_expired", "执行节点凭证已过期或撤销"},
		{"worker_creator_revoked", "执行节点登记人的当前项目权限已失效"}, {"worker_unregistered", "执行节点未登记、未获项目授权或机器凭证已撤销"},
		{"pool_paused", "资源池已暂停，停止新派发"}, {"pool_unavailable", "资源池当前状态不允许新派发"}, {"pool_expired", "资源池执行节点凭证已过期"},
		{"inventory_missing", "执行节点尚未上报工具清单"}, {"inventory_stale", "工具清单已过期，等待执行节点重新探测"},
		{"command_missing", "当前执行节点未声明此命令"}, {"profile_mismatch", "当前工具配置、版本或程序摘要与固定档案不匹配"},
		{"reported_unavailable", "执行节点报告工具不可用：vendor {{.reason}} native.run.failed"},
		{"protocol_unverified", "OpenCode 当前版本尚未通过平台执行协议验收"}, {"dispatchable", "执行节点已报告可领取；模型认证、执行与验收结果需分别核验"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			var f *fixture
			var p ToolProfile
			var pool *poolMachineFixture
			if strings.HasPrefix(c.key, "pool_") {
				pool = newPoolMachineFixture(t)
				f = pool.f
				var err error
				p, err = f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: pool.credential.Runner.WorkerID, Name: "pool", Profile: toolTestProfile(), IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				f, _, p = toolFixture(t)
			}
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := f.s.DB.Exec(q, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`UPDATE suma_tool_profiles SET state='enabled',enabled_by=? WHERE id=?`, f.user.ID, p.ID)
			o := toolcontract.Observation{Profile: p.Profile, Available: true}
			switch c.key {
			case "profile_revoked":
				exec(`UPDATE suma_tool_profiles SET state='revoked' WHERE id=?`, p.ID)
			case "profile_paused":
				exec(`UPDATE suma_tool_profiles SET state='paused' WHERE id=?`, p.ID)
			case "activation_revoked":
				exec(`UPDATE suma_tool_profiles SET enabled_by='missing-approver' WHERE id=?`, p.ID)
			case "worker_expired":
				exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash IN (SELECT token_hash FROM suma_release_workers WHERE worker_id=?)`, p.WorkerID)
			case "worker_creator_revoked":
				exec(`UPDATE suma_release_workers SET created_by='missing-creator' WHERE worker_id=?`, p.WorkerID)
			case "worker_unregistered":
				exec(`DELETE FROM suma_release_workers WHERE worker_id=?`, p.WorkerID)
			case "pool_paused":
				exec(`UPDATE suma_execution_pools SET status='paused' WHERE id=?`, pool.pool.ID)
			case "pool_unavailable":
				exec(`UPDATE suma_execution_pools SET status='revoked' WHERE id=?`, pool.pool.ID)
			case "pool_expired":
				exec(`UPDATE suma_execution_pool_credentials SET expires_at='2000-01-01T00:00:00Z' WHERE runner_id=?`, pool.credential.Runner.ID)
			case "command_missing":
				o.Profile.CommandID = "other-command"
			case "profile_mismatch":
				o.Profile.ToolVersion = "drift"
			case "reported_unavailable":
				o.Available = false
				o.Reason = "vendor {{.reason}} native.run.failed"
			case "protocol_unverified":
				p.Profile.Adapter = "opencode"
				p.Profile.Protocol = "opencode-jsonl-v1"
				o.Profile = p.Profile
				exec(`UPDATE suma_tool_profiles SET profile=? WHERE id=?`, stringify(p.Profile), p.ID)
			}
			if c.key != "inventory_missing" {
				at := time.Now().UTC().Format(time.RFC3339)
				if c.key == "inventory_stale" {
					at = "2000-01-01T00:00:00Z"
				}
				exec(`INSERT INTO suma_tool_inventories(tenant_id,project_id,worker_id,observed_at,items)VALUES(?,?,?,?,?)`, f.user.TenantID, f.project, p.WorkerID, at, stringify([]toolcontract.Observation{o}))
			}
			got, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Reason != c.reason || got.Available != (c.key == "dispatchable") {
				t.Fatalf("wrong eligibility: reason=%q available=%v", got.Reason, got.Available)
			}
			for _, locale := range []string{"zh-CN", "en-US"} {
				read := displayHTTP(t, f, "GET", "/tool-profiles/"+p.ID+"?project_id="+f.project, locale, nil, 200)
				assertDisplayWire(t, read, "display.tool."+c.key, c.reason)
				listed := displayHTTP(t, f, "GET", "/tool-profiles?project_id="+f.project, locale, nil, 200)
				items, ok := listed["items"].([]any)
				if !ok || len(items) != 1 {
					t.Fatalf("missing list profile: %v", listed)
				}
				assertDisplayWire(t, items[0].(map[string]any), "display.tool."+c.key, c.reason)
			}
			raw, _ := json.Marshal(got)
			var wire map[string]any
			json.Unmarshal(raw, &wire)
			d, ok := wire["reason_diagnostic"].(map[string]any)
			if !ok || d["message_key"] != "display.tool."+c.key || d["schema_version"] != float64(1) {
				t.Errorf("missing/wrong trusted diagnostic: %s", raw)
			}
		})
	}
}
func TestToolDisplayProjectionPreservesRawReason(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		t.Run(locale, func(t *testing.T) {
			w := httptest.NewRecorder()
			w.Header().Set("Content-Language", locale)
			p := ToolProfile{Reason: "资源池已暂停，停止新派发"}
			got := localizePresentation(w, p).(ToolProfile)
			list := localizePresentation(w, ToolProfileList{Items: []ToolProfile{p}}).(ToolProfileList)
			if got.Reason != p.Reason || list.Items[0].Reason != p.Reason {
				t.Fatalf("raw reason translated: %q / %q", got.Reason, list.Items[0].Reason)
			}
		})
	}
}
