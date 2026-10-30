package platform

import (
	"context"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudLookupRequest(h *cloudFixture) wc.CloudLookupRequest {
	return wc.CloudLookupRequest{ProjectID: h.request.ProjectID, DelegationID: h.request.DelegationID, Assignment: h.request.Assignment}
}

func cloudLookupCall(ctx context.Context, h *cloudFixture, req wc.CloudLookupRequest) (wc.CloudLookupResult, error) {
	if h.poolToken != "" {
		return h.w.LookupPoolCloudSubmission(ctx, h.poolToken, req)
	}
	return h.w.LookupCloudSubmission(ctx, h.actor, h.f.project, req)
}

func cloudLookupStoredFingerprint(t *testing.T, h *cloudFixture) string {
	t.Helper()
	var state []any
	for _, table := range []string{"suma_workbuddy_cloud_delegations", "suma_workbuddy_cloud_submissions"} {
		rows, err := h.f.s.DB.Query("SELECT payload FROM "+table+" WHERE tenant_id=? AND project_id=? ORDER BY id", h.f.user.TenantID, h.f.project)
		if err != nil {
			t.Fatal(err)
		}
		var records []string
		for rows.Next() {
			var payload string
			if err = rows.Scan(&payload); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			records = append(records, payload)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		state = append(state, records)
	}
	for _, table := range []string{"suma_operations", "suma_activity"} {
		var count int
		if err := h.f.s.DB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE tenant_id=? AND project_id=?", h.f.user.TenantID, h.f.project).Scan(&count); err != nil {
			t.Fatal(err)
		}
		state = append(state, count)
	}
	return digest(stringify(state))
}

func TestWorkBuddyCloudLookupReturnsOriginalFactsWithoutEffects(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "pool"}[pooled], func(t *testing.T) {
			h := newCloudFixtureMode(t, nil, pooled)
			ctx := context.Background()
			req := cloudLookupRequest(h)
			before := cloudLookupStoredFingerprint(t, h)
			out, err := cloudLookupCall(ctx, h, req)
			if err != nil || out.Validate() != nil || out.State != "not_found" || out.Submission != nil {
				t.Fatal("original absence was not read", err)
			}
			if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 0 || h.queries.Load() != 0 {
				t.Fatal("absence lookup changed business state or contacted provider")
			}
			var sub wc.CloudSubmission
			if pooled {
				sub, err = h.w.CreatePoolCloudTask(ctx, h.poolToken, h.request)
			} else {
				sub, err = h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
			}
			if err != nil {
				t.Fatal("fixture create", err)
			}
			before = cloudLookupStoredFingerprint(t, h)
			out, err = cloudLookupCall(ctx, h, req)
			if err != nil || out.Validate() != nil || out.State != "task_known" || out.Submission == nil || out.Submission.ID != sub.ID || out.Submission.Assignment != h.request.Assignment {
				t.Fatal("lost response could not locate exact persisted submission", err)
			}
			if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 1 || h.queries.Load() != 0 {
				t.Fatal("known lookup changed business state or repeated provider work")
			}
		})
	}
}
