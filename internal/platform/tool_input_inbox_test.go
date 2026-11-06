package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func TestTaskToolInputInboxRoutesOriginalAttempt(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "pool"}[pooled], func(t *testing.T) {
			h, binding := newCodexToolInteractionFixture(t, pooled, true)
			f, a, ctx := h.f, h.assignment, context.Background()
			ownerCookie := f.cookie
			member := func(email, role string) (Actor, string) {
				t.Helper()
				invitation := governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": role}})
				governanceAccept(f, invitation, email)
				token := f.cookie.Value
				actor, err := f.s.AuthenticateActor(ctx, token)
				if err != nil {
					t.Fatal(err)
				}
				f.cookie = ownerCookie
				f.call("GET", "/auth/session", nil, 200)
				return actor, token
			}
			follower, followerToken := member("tool-follower@example.test", "developer")
			viewer, viewerToken := member("tool-viewer@example.test", "viewer")
			late, lateToken := member("tool-late@example.test", "developer")
			owner, err := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{
				"title": "Question owner", "kind": "human", "data": map[string]any{"user_id": f.user.ID},
			}))
			if err != nil {
				t.Fatal(err)
			}
			task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{
				"title": "Formal task awaiting input", "owner_id": owner.ID,
			}))
			if err != nil {
				t.Fatal(err)
			}
			for _, actor := range []Actor{f.actor, follower, viewer} {
				subscriptionCommand(t, f, actor, task.ID, 0, "follow", newID())
			}
			open := func(request ic.ToolInputRequest, want int) map[string]any {
				t.Helper()
				if pooled {
					return f.bearer("POST", "/pool-runner/tool-inputs", h.token, PoolToolInputOpenRequest{
						PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: a.PoolRunnerID, TenantID: a.TenantID, ProjectID: f.project, Assignment: a},
						Request:                     request,
					}, want)
				}
				return f.bearer("POST", "/runner/tool-inputs", h.token, ToolInputOpenRequest{ProjectID: f.project, Assignment: a, Request: request}, want)
			}
			noticeCount := func(want int) {
				t.Helper()
				var count int
				err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_notification_inbox WHERE kind=?`, taskToolInputInboxKind).Scan(&count)
				if err != nil || count != want {
					t.Fatalf("inbox has %d tool input events, want %d: %v", count, want, err)
				}
			}
			unbound := toolInputRequest187(binding, "unbound-input-264")
			unboundID := open(unbound, 200)["id"]
			noticeCount(0)
			if err = f.s.AttachRun(ctx, f.actor, f.project, task.ID, a.RunID); err != nil {
				t.Fatal(err)
			}
			if replay := open(unbound, 200); replay["id"] != unboundID {
				t.Fatal("unbound input replay changed the original identity")
			}
			nonblocking := toolInputRequest187(binding, "nonblocking-input-264")
			nonblocking.Codex.IsBlocking = false
			open(nonblocking, 200)
			noticeCount(0)

			const privateQuestion = "Private question body 264"
			const privateAnswer = "Private answer body 264"
			request := toolInputRequest187(binding, "blocking-input-264")
			request.Codex.Questions[0].Question = privateQuestion
			if _, err = f.s.DB.Exec(`CREATE TRIGGER fail_tool_input_inbox_264 BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_tool_input_requested' BEGIN SELECT RAISE(ABORT,'controlled inbox write failure'); END`); err != nil {
				t.Fatal(err)
			}
			inputsBefore := domainACount(t, f.s, "suma_tool_inputs")
			open(request, 500)
			if got := domainACount(t, f.s, "suma_tool_inputs"); got != inputsBefore {
				t.Fatalf("failed inbox insert left a partial input row: %d, want %d", got, inputsBefore)
			}
			noticeCount(0)
			if _, err = f.s.DB.Exec(`DROP TRIGGER fail_tool_input_inbox_264`); err != nil {
				t.Fatal(err)
			}
			opened := open(request, 200)
			inputID, ok := opened["id"].(string)
			if !ok || opened["state"] != "pending" {
				t.Fatal("blocking input did not open")
			}
			noticeCount(2)
			subscriptionCommand(t, f, late, task.ID, 0, "follow", newID())
			if replay := open(request, 200); replay["id"] != inputID {
				t.Fatal("blocking input replay changed the original identity")
			}
			noticeCount(2)

			projectPath := "/notification-inbox?project_id=" + f.project
			readNotice := func(token string, want int) []NotificationInboxItem {
				t.Helper()
				page := f.bearer("GET", projectPath, token, nil, 200)
				raw := stringify(page)
				if strings.Contains(raw, privateQuestion) || strings.Contains(raw, privateAnswer) || strings.Contains(raw, inputID) ||
					strings.Contains(raw, request.Codex.ParamsSHA256) {
					t.Fatal("notification response exposed a private question, answer or native binding")
				}
				var parsed NotificationInboxPage
				if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
					t.Fatal(err)
				}
				var matches []NotificationInboxItem
				for _, item := range parsed.Items {
					if item.Kind == taskToolInputInboxKind {
						matches = append(matches, item)
					}
				}
				if len(matches) != want {
					t.Fatalf("read %d tool input notifications, want %d", len(matches), want)
				}
				return matches
			}
			checkSource := func(item NotificationInboxItem) {
				t.Helper()
				target := item.ToolInputTarget
				if !item.SourceAvailable || item.Read || item.Archived || item.CurrentTask == nil || item.CurrentTask.ID != task.ID ||
					item.RunID != "" || target == nil || target.RunID != a.RunID || target.NodeID != a.NodeID || target.Attempt != a.Attempt ||
					item.SourceResourceID == inputID || item.SourceResourceID == a.RunID {
					t.Fatalf("wrong tool input notification state or target: %+v", item)
				}
			}
			ownerNotice := readNotice(ownerCookie.Value, 1)[0]
			followerNotice := readNotice(followerToken, 1)[0]
			checkSource(ownerNotice)
			checkSource(followerNotice)
			if ownerNotice.SourceResourceID == followerNotice.SourceResourceID {
				t.Fatal("opaque source fingerprint is not bound to the recipient")
			}
			readNotice(viewerToken, 0)
			readNotice(lateToken, 0)
			answer := ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID(),
				Answers: []cc.CodexUserInputSelection{{QuestionID: "design", Kind: "other", Text: privateAnswer}},
			}
			f.call("POST", "/tool-inputs/"+inputID+"/answer", answer, 200)
			observation := delivery.Observation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, RequestResolved: true, ExecutionClosed: true}
			if pooled {
				_, err = f.s.ObservePoolToolInput(ctx, h.token, f.project, a, inputID, newID(), observation)
			} else {
				_, err = f.s.ObserveToolInput(ctx, h.actor, f.project, a, inputID, newID(), observation)
			}
			if err != nil {
				t.Fatal(err)
			}
			history := readNotice(followerToken, 1)[0]
			checkSource(history)
			if history.ID != followerNotice.ID || history.Version != followerNotice.Version {
				t.Fatal("opening and answering changed the personal inbox record")
			}

			organization := f.bearer("GET", "/notification-inbox/organization", followerToken, nil, 200)
			var aggregate NotificationOrganizationInboxPage
			if err = json.Unmarshal([]byte(stringify(organization)), &aggregate); err != nil {
				t.Fatal(err)
			}
			if len(aggregate.Items) != 1 || aggregate.Items[0].ProjectID != f.project || aggregate.Items[0].Notification.ID != history.ID {
				t.Fatal("organization inbox did not carry the exact project and notification")
			}
			checkSource(aggregate.Items[0].Notification)

			if _, err = f.s.DB.Exec(`UPDATE suma_task_run_provenance SET payload='{}' WHERE run_id=?`, a.RunID); err != nil {
				t.Fatal(err)
			}
			lostSource := readNotice(followerToken, 1)[0]
			if lostSource.SourceAvailable || lostSource.CurrentTask != nil || lostSource.ToolInputTarget != nil {
				t.Fatal("corrupt provenance exposed an original attempt link or task")
			}
			if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, follower.credential.subjectID); err != nil {
				t.Fatal(err)
			}
			f.bearer("GET", projectPath, followerToken, nil, 403)
			afterRevoke := f.bearer("GET", "/notification-inbox/organization", followerToken, nil, 200)
			if rows := afterRevoke["items"].([]any); len(rows) != 0 {
				t.Fatal("organization inbox included a revoked project's private event")
			}
		})
	}
}
