package repositoryprovider

import (
	"bytes"
	"context"
	"encoding/json"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryGitAtomicReceiptAndNegativeFence(t *testing.T) {
	for _, negative := range []bool{false, true} {
		name := "positive"
		if negative {
			name = "negative"
		}
		t.Run(name, func(t *testing.T) {
			f := newGitFixture(t)
			prepared := prepareFixture(t, f)
			q := f.request(t, "apply", "apply-0000001", prepared)
			hash, _ := rc.RepositoryRequestDigest(q)
			if negative {
				op := operation{Request: rc.RepositoryNonSecretRequest{RepositoryExecution: q.RepositoryExecution, Assignment: q.Assignment}, Grant: &rc.RepositoryAuthorizationGrant{AuthorizationID: "authorization-fixed"}, Observation: rc.RepositoryObservation{Protocol: rc.RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: hash, Phase: q.Phase, State: "unknown", Version: 1, Execution: q.RepositoryExecution, ObservedAt: now()}}
				op.Observation.ObservationSHA256, _ = rc.RepositoryObservationDigest(op.Observation)
				if e := f.s.save(&op); e != nil {
					t.Fatal(e)
				}
				before := op.Observation
				_, read := providerCall(t, f.s, "GET", "/operations/"+q.OperationID, nil, true)
				if read.State != "unknown" {
					t.Fatal("missing marker became known")
				}
				loaded, _ := f.s.load(q.OperationID)
				if loaded.Observation != before {
					t.Fatal("GET mutated durable state")
				}
				code, o := providerCall(t, f.s, "POST", "/operations/"+q.OperationID+"/reconcile", rc.RepositoryReconcileRequest{OperationID: q.OperationID, RequestSHA256: hash, ReconciliationTicket: "once"}, true)
				if code != 200 || o.State != "not_applied" {
					t.Fatalf("negative %d %#v", code, o)
				}
				if e := rc.RepositoryValidateObservation(o, q.RepositoryExecution, hash); e != nil {
					t.Fatal(e)
				}
				if e := f.s.commitGit(context.Background(), f.s.targets["main"], &op, false, "orphan-effect"); e == nil {
					t.Fatal("late orphan committed after negative fence")
				}
			} else {
				code, o := providerCall(t, f.s, "POST", "/operations", q, false)
				if code != 200 || o.State != "applied" {
					t.Fatalf("apply %d %#v", code, o)
				}
				if e := rc.RepositoryValidateObservation(o, q.RepositoryExecution, hash); e != nil {
					t.Fatal(e)
				}
				_, again := providerCall(t, f.s, "POST", "/operations", q, false)
				if again.ReceiptOID != o.ReceiptOID || f.effects != 1 {
					t.Fatal("replay repeated effect")
				}
				f.s.Close()
				f.s = nil
				var e error
				f.s, e = New(f.cfg)
				if e != nil {
					t.Fatal(e)
				}
				_, again = providerCall(t, f.s, "GET", "/operations/"+q.OperationID, nil, true)
				if again.State != "applied" || again.ReceiptOID != o.ReceiptOID {
					t.Fatal("restart lost original Git receipt")
				}
			}
			target, e := f.s.Target(context.Background(), "main")
			if e != nil {
				t.Fatal(e)
			}
			want := f.head
			if negative {
				want = f.base
			}
			if target.HeadCommit != want || target.Generation != 1 {
				t.Fatalf("target/fence %#v", target)
			}
		})
	}
}
func TestRepositoryGitRejectsDriftAndUnreviewableSource(t *testing.T) {
	t.Run("bundle", func(t *testing.T) {
		f := newGitFixture(t)
		q := f.request(t, "prepare", "prepare-bad1", "")
		f.bundle = append(f.bundle, 'x')
		code, o := providerCall(t, f.s, "POST", "/operations", q, false)
		if code != 200 || o.State != "blocked" {
			t.Fatal(code, o.State)
		}
		target, _ := f.s.Target(context.Background(), "main")
		if target.HeadCommit != f.base {
			t.Fatal("invalid bundle changed target")
		}
	})
	t.Run("config", func(t *testing.T) {
		f := newGitFixture(t)
		file := filepath.Join(f.target, "config")
		b, _ := os.ReadFile(file)
		if e := os.WriteFile(file, append(b, []byte("\n# drift\n")...), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := f.s.Target(context.Background(), "main"); e == nil {
			t.Fatal("configuration drift accepted")
		}
	})
	t.Run("auth", func(t *testing.T) {
		f := newGitFixture(t)
		q := f.request(t, "prepare", "prepare-auth", "")
		b, _ := json.Marshal(q)
		r := httptest.NewRequest("POST", "/operations", bytes.NewReader(b))
		w := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	})
}

func TestRepositoryGitLostAckAndAtomicOrphanRace(t *testing.T) {
	t.Run("lost_ack", func(t *testing.T) {
		f := newGitFixture(t)
		prepared := prepareFixture(t, f)
		q := f.request(t, "apply", "apply-lost-ack", prepared)
		f.s.afterGit = func() { panic("injected process crash after Git transaction") }
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("crash injection did not run")
				}
			}()
			providerCall(t, f.s, "POST", "/operations", q, false)
		}()
		f.s.afterGit = nil
		stored, e := f.s.load(q.OperationID)
		if e != nil || stored.Observation.State != "applying" {
			t.Fatal("checkpoint not durable before effect")
		}
		f.s.Close()
		f.s = nil
		f.s, e = New(f.cfg)
		if e != nil {
			t.Fatal(e)
		}
		_, read := providerCall(t, f.s, "GET", "/operations/"+q.OperationID, nil, true)
		if read.State != "applied" {
			t.Fatal("read-only inspect lost actual Git proof")
		}
		_, readAgain := providerCall(t, f.s, "GET", "/operations/"+q.OperationID, nil, true)
		if readAgain.ObservationSHA256 != read.ObservationSHA256 || readAgain.Version != read.Version {
			t.Fatal("GET changed immutable proof version/hash")
		}
		still, _ := f.s.load(q.OperationID)
		if still.Observation.State != "applying" {
			t.Fatal("GET wrote recovered observation")
		}
		code, known := providerCall(t, f.s, "POST", "/operations", q, false)
		if code != 200 || known.State != "applied" || f.effects != 1 {
			t.Fatal("recovery repeated effect", code, known.State, f.effects)
		}
	})
	t.Run("orphan_race", func(t *testing.T) {
		f := newGitFixture(t)
		prepared := prepareFixture(t, f)
		q := f.request(t, "apply", "apply-race-0001", prepared)
		hash, _ := rc.RepositoryRequestDigest(q)
		op := operation{Request: rc.RepositoryNonSecretRequest{RepositoryExecution: q.RepositoryExecution, Assignment: q.Assignment}, Observation: rc.RepositoryObservation{Protocol: rc.RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: hash, Phase: q.Phase, State: "unknown", Version: 1, Execution: q.RepositoryExecution}}
		start := make(chan struct{})
		done := make(chan bool, 2)
		for _, negative := range []bool{false, true} {
			go func(negative bool) {
				copy := op
				<-start
				done <- f.s.commitGit(context.Background(), f.s.targets["main"], &copy, negative, "race-grant") == nil
			}(negative)
		}
		close(start)
		wins := 0
		for range 2 {
			if <-done {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("Git transactions did not have exactly one winner", wins)
		}
		known, e := f.s.inspect(context.Background(), f.s.targets["main"], op)
		if e != nil || known.Receipt == nil {
			t.Fatal("race lost exact operation proof", e)
		}
		target, e := f.s.Target(context.Background(), "main")
		if e != nil {
			t.Fatal(e)
		}
		expected := f.base
		if known.State == "applied" {
			expected = f.head
		}
		if target.HeadCommit != expected || target.Generation != 1 {
			t.Fatal("target and receipt diverged")
		}
	})
}

func TestRepositoryReconcileMissingIntentAndBoundedReview(t *testing.T) {
	t.Run("prepare_closed", func(t *testing.T) {
		f := newGitFixture(t)
		q := f.request(t, "prepare", "prepare-close-01", "")
		hash, _ := rc.RepositoryRequestDigest(q)
		request := rc.RepositoryNonSecretRequest{RepositoryExecution: q.RepositoryExecution, Assignment: q.Assignment}
		code, o := providerCall(t, f.s, "POST", "/operations/"+q.OperationID+"/reconcile", rc.RepositoryReconcileRequest{OperationID: q.OperationID, RequestSHA256: hash, ReconciliationTicket: "current-close", Request: &request}, true)
		if code != 200 || o.State != "blocked" || o.Reason != "preparation_closed_by_reconciliation" || o.Receipt != nil || o.EffectGrantID == "" {
			t.Fatal("prepare closure", code, o)
		}
		_, again := providerCall(t, f.s, "POST", "/operations", q, false)
		if again.ObservationSHA256 != o.ObservationSHA256 || again.State != "blocked" {
			t.Fatal("closed preparation resumed")
		}
		target, e := f.s.Target(context.Background(), "main")
		if e != nil || target.HeadCommit != f.base || target.Generation != 0 || f.effects != 0 {
			t.Fatal("closing preparation changed target/fence")
		}
		if _, e = f.s.git(context.Background(), f.s.targets["main"], nil, 4096, "cat-file", "-e", f.head); e == nil {
			t.Fatal("closed preparation imported/executed source")
		}
	})
	t.Run("missing_intent", func(t *testing.T) {
		f := newGitFixture(t)
		q := f.request(t, "apply", "absent-op-0001", strings.Repeat("f", 64))
		hash, _ := rc.RepositoryRequestDigest(q)
		code, _ := providerCall(t, f.s, "GET", "/operations/"+q.OperationID, nil, true)
		if code != 404 {
			t.Fatal("unexpected prior operation", code)
		}
		request := rc.RepositoryNonSecretRequest{RepositoryExecution: q.RepositoryExecution, Assignment: q.Assignment}
		code, o := providerCall(t, f.s, "POST", "/operations/"+q.OperationID+"/reconcile", rc.RepositoryReconcileRequest{OperationID: q.OperationID, RequestSHA256: hash, ReconciliationTicket: "current-once", Request: &request}, true)
		if code != 200 || o.State != "not_applied" || o.Receipt == nil {
			t.Fatalf("negative missing-intent %d %#v", code, o)
		}
		if e := rc.RepositoryValidateObservation(o, q.RepositoryExecution, hash); e != nil {
			t.Fatal(e)
		}
		if f.effects != 0 {
			t.Fatal("negative reconciliation dispatched positive effect")
		}
	})
	for name, content := range map[string]string{"binary": "\x00unreviewable", "encoded_bound": strings.Repeat("\x01", 200<<10)} {
		t.Run(name, func(t *testing.T) {
			f := newGitFixtureContent(t, content)
			q := f.request(t, "prepare", "review-limit-0001", "")
			code, o := providerCall(t, f.s, "POST", "/operations", q, false)
			if code != 200 || o.State != "blocked" || o.Preview != nil {
				t.Fatal("unreviewable candidate escaped", code, o.State)
			}
			target, e := f.s.Target(context.Background(), "main")
			if e != nil || target.HeadCommit != f.base || target.Generation != 0 {
				t.Fatal("blocked review changed target")
			}
		})
	}
}
