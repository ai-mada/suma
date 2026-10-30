package platform

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudLookupPreparedAndUnknownNeverResend(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	h := newCloudFixture(t, func(w http.ResponseWriter, _ *http.Request) { close(entered); <-release; w.WriteHeader(503) })
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { _, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("prepare did not reach original provider request")
	}
	req := cloudLookupRequest(h)
	before := cloudLookupStoredFingerprint(t, h)
	out, err := cloudLookupCall(ctx, h, req)
	if err != nil || out.State != "prepared" || out.Submission == nil || out.Submission.Task != nil || out.Validate() != nil {
		t.Fatal("prepared fact was not read", err)
	}
	if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatal("prepared lookup mutated or contacted provider")
	}
	originalID := out.Submission.ID
	once.Do(func() { close(release) })
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("fixture failure was hidden")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("original create did not finish")
	}
	before = cloudLookupStoredFingerprint(t, h)
	out, err = cloudLookupCall(ctx, h, req)
	if err != nil || out.State != "unknown" || out.Submission == nil || out.Submission.ID != originalID || out.Validate() != nil {
		t.Fatal("unknown did not preserve original identity", err)
	}
	if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatal("unknown lookup retried provider or wrote a fact")
	}
}

func cloudLookupChangeConnection(t *testing.T, h *cloudFixture, change func(*workBuddyConnectionRow)) {
	t.Helper()
	ctx, auth, close, err := h.w.workBuddyCommandAuthority(context.Background(), h.f.actor, h.f.project, h.delegation.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	row, err := h.w.rowTx(ctx, auth.tx, h.f.user.TenantID, h.f.project, h.delegation.ConnectionID, true)
	if err != nil {
		t.Fatal(err)
	}
	before := row.Version
	change(&row)
	row.Version++
	if err = h.w.saveRowTx(ctx, auth, row, before); err != nil {
		t.Fatal(err)
	}
	if err = auth.tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkBuddyCloudLookupReadsExpiredOriginalWithoutExecutionAuthority(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "pool"}[pooled], func(t *testing.T) {
			h := newCloudFixtureMode(t, nil, pooled)
			ctx := context.Background()
			var sub wc.CloudSubmission
			var err error
			if pooled {
				sub, err = h.w.CreatePoolCloudTask(ctx, h.poolToken, h.request)
			} else {
				sub, err = h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, h.request.Assignment.RunID, h.request.Assignment.NodeID); err != nil {
				t.Fatal(err)
			}
			out, err := cloudLookupCall(ctx, h, cloudLookupRequest(h))
			if err != nil || out.Submission == nil || out.Submission.ID != sub.ID {
				t.Fatal("expired original fact could not be recovered", err)
			}
			refresh := wc.CloudRefreshRequest{ProjectID: h.f.project, ExpectedVersion: sub.Version, Assignment: h.request.Assignment, IdempotencyKey: newID()}
			if pooled {
				if _, err = h.w.CreatePoolCloudTask(ctx, h.poolToken, h.request); err == nil {
					t.Fatal("read restored create authority")
				}
				if _, err = h.w.RefreshPoolCloudTask(ctx, h.poolToken, sub.ID, refresh); err == nil {
					t.Fatal("read restored refresh authority")
				}
			} else {
				if _, err = h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request); err == nil {
					t.Fatal("read restored create authority")
				}
				if _, err = h.w.RefreshCloudTask(ctx, h.actor, h.f.project, sub.ID, refresh); err == nil {
					t.Fatal("read restored refresh authority")
				}
			}
			d, err := h.w.GetCloudDelegation(ctx, h.f.actor, h.f.project, h.delegation.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = h.w.RevokeCloudDelegation(ctx, h.f.actor, h.f.project, d.ID, wc.CloudRevokeRequest{ProjectID: h.f.project, ExpectedVersion: d.Version, IdempotencyKey: newID()}); err != nil {
				t.Fatal(err)
			}
			cloudLookupChangeConnection(t, h, func(row *workBuddyConnectionRow) {
				row.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
			})
			// The wrong encryption key makes opening the stored OAuth grant fail.
			// A lookup must not open it in order to return previously saved facts.
			other, err := NewWorkBuddyConnections(h.f.s, h.w.app, []byte(strings.Repeat("x", 32)))
			if err != nil {
				t.Fatal(err)
			}
			h.w = other
			for _, state := range []string{"reconciling", "waiting", "succeeded"} {
				if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET state=? WHERE run_id=? AND id=?`, state, h.request.Assignment.RunID, h.request.Assignment.NodeID); err != nil {
					t.Fatal(err)
				}
				before := cloudLookupStoredFingerprint(t, h)
				out, err = cloudLookupCall(ctx, h, cloudLookupRequest(h))
				if err != nil || out.Submission == nil || out.Submission.ID != sub.ID || out.Validate() != nil {
					t.Fatal("retired same-attempt fact was not readable", state, err)
				}
				if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 1 || h.queries.Load() != 0 {
					t.Fatal("retired read changed business state")
				}
			}
		})
	}
}

func TestWorkBuddyCloudLookupKeepsCurrentGrantAndCredentialBoundaries(t *testing.T) {
	for _, kind := range []string{"new_credential", "new_grant", "pool_grant", "pool_credential", "pool_original_lease", "new_fence", "new_attempt", "new_worker", "consumed_without_record"} {
		t.Run(kind, func(t *testing.T) {
			pooled := strings.HasPrefix(kind, "pool_")
			h := newCloudFixtureMode(t, nil, pooled)
			ctx := context.Background()
			var err error
			if kind != "consumed_without_record" {
				if pooled {
					_, err = h.w.CreatePoolCloudTask(ctx, h.poolToken, h.request)
				} else {
					_, err = h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			req := cloudLookupRequest(h)
			switch kind {
			case "new_credential":
				token := h.f.call("POST", "/auth/tokens", h.f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
				h.actor = domainCActor(t, h.f.s, token)
			case "new_grant":
				c, e := h.w.Get(ctx, h.f.actor, h.f.project, h.delegation.ConnectionID)
				if e != nil {
					t.Fatal(e)
				}
				_, err = h.w.StartAuthorization(ctx, h.f.actor, h.f.project, WorkBuddyAuthorizationRequest{ProjectID: h.f.project, ConnectionID: c.ID, ExpectedVersion: c.Version, Scopes: c.Scopes, IdempotencyKey: newID()})
			case "pool_grant":
				_, err = h.f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET version=version+1 WHERE id=?`, req.Assignment.PoolGrantID)
			case "pool_credential":
				_, err = h.f.s.DB.Exec(`UPDATE suma_execution_pool_runners SET credential_version=credential_version+1 WHERE id=?`, req.Assignment.PoolRunnerID)
			case "pool_original_lease":
				req.Assignment.LeaseUntil = "2099-01-01T00:00:00Z"
			case "new_fence":
				_, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, req.Assignment.RunID, req.Assignment.NodeID)
			case "new_attempt":
				_, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET attempt=attempt+1 WHERE run_id=? AND id=?`, req.Assignment.RunID, req.Assignment.NodeID)
			case "new_worker":
				_, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET worker_id='other-worker' WHERE run_id=? AND id=?`, req.Assignment.RunID, req.Assignment.NodeID)
			case "consumed_without_record":
				err = func() error {
					b, tx, done, e := h.f.s.beginAuthorityTx(ctx)
					if e != nil {
						return e
					}
					defer done()
					defer tx.Rollback()
					d, e := h.w.cloudDelegationTx(b, tx, h.f.user.TenantID, h.f.project, h.delegation.ID, true)
					if e != nil {
						return e
					}
					d.Version++
					d.CreatesConsumed = 1
					if e = h.w.saveCloudDelegationTx(b, tx, d, d.Version-1); e != nil {
						return e
					}
					return tx.Commit()
				}()
			}
			if err != nil {
				t.Fatal("fixture boundary change", err)
			}
			before := cloudLookupStoredFingerprint(t, h)
			if out, e := cloudLookupCall(ctx, h, req); e == nil || out.State != "" {
				t.Fatal("changed current authority exposed original fact", kind, e)
			}
			if before != cloudLookupStoredFingerprint(t, h) || h.queries.Load() != 0 {
				t.Fatal("denied lookup wrote or contacted provider")
			}
		})
	}
}

func TestWorkBuddyCloudLookupSameGrantRefreshRemainsReadable(t *testing.T) {
	h := newCloudFixture(t, nil)
	ctx := context.Background()
	sub, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.w.Get(ctx, h.f.actor, h.f.project, h.delegation.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := h.w.Refresh(ctx, h.f.actor, h.f.project, c.ID, WorkBuddyRefreshRequest{ProjectID: h.f.project, ExpectedVersion: c.Version, IdempotencyKey: newID()})
	if err != nil || next.TokenGeneration <= c.TokenGeneration || next.GrantGeneration != c.GrantGeneration {
		t.Fatal("fixture normal refresh", err)
	}
	before := cloudLookupStoredFingerprint(t, h)
	out, err := cloudLookupCall(ctx, h, cloudLookupRequest(h))
	if err != nil || out.Submission == nil || out.Submission.ID != sub.ID {
		t.Fatal("normal token refresh blocked original fact", err)
	}
	if before != cloudLookupStoredFingerprint(t, h) || h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatal("refresh lookup performed extra work")
	}
}
