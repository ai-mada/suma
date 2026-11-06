package platform

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestUsageAttemptFactsHTTP(t *testing.T) {
	h := newUsageFixture(t)
	f := h.f
	ctx := context.Background()
	usageRateForTest(t, h)

	workflow := f.call(http.MethodPost, "/workflows", f.body(map[string]any{
		"name": "Usage receipt history",
		"nodes": []dag.Node{{
			ID:      "agent",
			Type:    "agent",
			Command: []string{h.profile.Profile.CommandID},
			Config: map[string]any{
				"goal":                "Record the original and late observations",
				"tool_profile_id":     h.profile.ID,
				"tool_profile_sha256": h.profile.Profile.ProfileSHA256,
			},
		}},
	}), http.StatusCreated)
	start := func() dag.Assignment {
		f.call(http.MethodPost, "/runs", f.body(map[string]any{"workflow_id": workflow["id"]}), http.StatusCreated)
		return h.claim(t)
	}

	assignment := start()
	original := h.result(assignment, "Private model output for the original observation")
	original.OutcomeUnknown = true
	original.ToolEvidence.Terminal = "unknown"
	original.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(10), CachedInputTokens: usageInt(0)}
	original.ToolEvidence.UsageState = toolcontract.UsagePartial
	h.complete(t, original)

	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ref := dag.AccountingAttempt{
		TenantID: f.user.TenantID, ProjectID: f.project, RunID: assignment.RunID,
		NodeID: assignment.NodeID, WorkerID: assignment.WorkerID, Attempt: assignment.Attempt, Fence: assignment.Fence,
		Spec: dag.Node{Type: "agent", Command: assignment.Command, Config: assignment.Config},
	}
	if err := f.s.RecordResolutionTx(ctx, tx, ref, "not_started"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	attempt := usageAttemptForTest(t, h, assignment.RunID)
	confirm := UsageConfirmRequest{
		ProjectID:        f.project,
		ExpectedVersion:  attempt.Version,
		Usage:            toolcontract.Usage{InputTokens: usageInt(12), OutputTokens: usageInt(2), CachedInputTokens: usageInt(0)},
		SourceRef:        "private operator receipt location",
		Reason:           "Private details from the operator's investigation",
		AcknowledgeUsage: true,
		IdempotencyKey:   newID(),
	}
	attemptPath := "/usage/attempts/" + attempt.ID
	f.call(http.MethodPost, attemptPath+"/confirm", confirm, http.StatusOK)

	late := h.result(assignment, "")
	late.Stopped = true
	late.ToolEvidence.Terminal = "unknown"
	late.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(25)}
	late.ToolEvidence.UsageState = toolcontract.UsagePartial
	h.complete(t, late)
	h.complete(t, late)

	// Put three real persisted facts at the same timestamp to exercise the
	// ID tie-breaker across a page boundary while preserving their payloads.
	const earlier, later = "2026-11-06T01:00:00Z", "2026-11-06T02:00:00Z"
	lateSHA := digest(stringify(late))
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_usage_facts SET created_at=CASE WHEN identity_sha256=? THEN ? ELSE ? END WHERE attempt_id=?`, lateSHA, later, earlier, attempt.ID); err != nil {
		t.Fatal(err)
	}

	fact := func(kind, sha, at string, originalResult bool, state string, usage map[string]any) map[string]any {
		t.Helper()
		var id string
		if err := f.s.DB.QueryRowContext(ctx, `SELECT id FROM suma_usage_facts WHERE attempt_id=? AND kind=? AND identity_sha256=?`, attempt.ID, kind, sha).Scan(&id); err != nil {
			t.Fatal(err)
		}
		item := map[string]any{
			"id": id, "kind": kind, "identity_sha256": sha, "created_at": at, "original_result": originalResult,
		}
		if state != "" {
			item["usage_state"] = state
		}
		if usage != nil {
			item["usage"] = usage
		}
		return item
	}
	lateFact := fact("result", lateSHA, later, false, "partial", map[string]any{"input_tokens": float64(25)})
	tied := []map[string]any{
		fact("result", digest(stringify(original)), earlier, true, "partial", map[string]any{"input_tokens": float64(10), "cached_input_tokens": float64(0)}),
		fact("resolution", digest("not_started"), earlier, false, "", nil),
		fact("confirmation", digest(stringify(confirm)), earlier, false, "complete", map[string]any{
			"input_tokens": float64(12), "output_tokens": float64(2), "cached_input_tokens": float64(0),
		}),
	}
	sort.Slice(tied, func(i, j int) bool { return tied[i]["id"].(string) > tied[j]["id"].(string) })

	factsPath := func(attemptID, projectID string) string {
		return "/usage/attempts/" + attemptID + "/facts?project_id=" + url.QueryEscape(projectID)
	}
	assertPage := func(path, attemptID, cursor string, items []any) {
		t.Helper()
		got := f.call(http.MethodGet, path, nil, http.StatusOK)
		want := map[string]any{"project_id": f.project, "attempt_id": attemptID, "next_cursor": cursor, "items": items}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected facts page\n got %s\nwant %s", stringify(got), stringify(want))
		}
	}
	path := factsPath(attempt.ID, f.project)
	cursor := tied[0]["id"].(string)
	assertPage(path+"&limit=2", attempt.ID, cursor, []any{lateFact, tied[0]})
	assertPage(path+"&limit=2&before="+url.QueryEscape(cursor), attempt.ID, "", []any{tied[1], tied[2]})
	assertPage(path, attempt.ID, "", []any{lateFact, tied[0], tied[1], tied[2]})

	second := usageAttemptForTest(t, h, start().RunID)
	secondPath := factsPath(second.ID, f.project)
	assertPage(secondPath, second.ID, "", []any{})
	f.call(http.MethodGet, secondPath+"&before="+url.QueryEscape(cursor), nil, http.StatusBadRequest)
	f.call(http.MethodGet, path+"&before="+newID(), nil, http.StatusBadRequest)
	other := f.call(http.MethodPost, "/projects", map[string]any{
		"title": "Unrelated usage project", "idempotency_key": newID(),
	}, http.StatusCreated)["id"].(string)
	f.call(http.MethodGet, factsPath(attempt.ID, other), nil, http.StatusNotFound)
	f.bearer(http.MethodGet, path, h.token, nil, http.StatusForbidden)

	const viewerEmail = "usage-facts-viewer@example.test"
	invitation := governanceInvite(f, viewerEmail, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	viewerID := governanceAccept(f, invitation, viewerEmail)["user"].(map[string]any)["id"].(string)
	assertPage(path, attempt.ID, "", []any{lateFact, tied[0], tied[1], tied[2]})
	// The persisted RFC3339Nano strings differ in length. Across a one-row
	// cursor boundary, .11 must precede .1, and a fractional time must
	// precede the exact second. Two facts at .11 still break ties by ID.
	const secondOnly = "2026-11-06T02:00:00Z"
	const tenth = "2026-11-06T02:00:00.1Z"
	const elevenHundredths = "2026-11-06T02:00:00.11Z"
	ordered := []map[string]any{lateFact, {}, {}, {}}
	for _, item := range tied {
		switch item["kind"] {
		case "confirmation":
			ordered[1] = item
		case "resolution":
			ordered[2] = item
		case "result":
			ordered[3] = item
		}
	}
	for i, at := range []string{elevenHundredths, elevenHundredths, tenth, secondOnly} {
		item := ordered[i]
		if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_usage_facts SET created_at=? WHERE attempt_id=? AND id=?`, at, attempt.ID, item["id"]); err != nil {
			t.Fatal(err)
		}
		item["created_at"] = at
	}
	sort.Slice(ordered[:2], func(i, j int) bool { return ordered[i]["id"].(string) > ordered[j]["id"].(string) })
	before := ""
	for i, item := range ordered {
		next := ""
		if i+1 < len(ordered) {
			next = item["id"].(string)
		}
		assertPage(path+"&limit=1"+before, attempt.ID, next, []any{item})
		before = "&before=" + url.QueryEscape(next)
	}
	if _, err := f.s.DB.ExecContext(ctx, `DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, viewerID); err != nil {
		t.Fatal(err)
	}
	f.call(http.MethodGet, path, nil, http.StatusForbidden)
}
