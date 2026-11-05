package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
)

type cursorRecoveryLostAck struct {
	next    http.RoundTripper
	bodies  [][]byte
	dropped bool
}

func (f *cursorRecoveryLostAck) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(req.URL.Path, "/results") {
		return f.next.RoundTrip(req)
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(body, 64<<10))
	_ = body.Close()
	if err != nil {
		return nil, err
	}
	f.bodies = append(f.bodies, raw)
	response, err := f.next.RoundTrip(req)
	if err == nil && response.StatusCode == http.StatusOK && len(f.bodies) == 1 {
		// The original service committed. The caller loses only the receipt.
		f.dropped = true
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, io.ErrUnexpectedEOF
	}
	return response, err
}

func TestCursorUnknownForegroundResultReplaysOriginalClaim(t *testing.T) {
	for _, pool := range []bool{false, true} {
		name := "worker"
		if pool {
			name = "pool"
		}
		t.Run(name, func(t *testing.T) {
			h, request := cursorACPFixture189(t, pool)
			f := h.f
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			var assignment runner.Assignment
			claim := cursorACPClaim189(t, h)
			if json.Unmarshal([]byte(stringify(claim["assignment"])), &assignment) != nil || assignment.RunID != run.ID || assignment.NodeID != "develop" {
				t.Fatal("original Cursor assignment unavailable")
			}
			server := httptest.NewServer(f.handler)
			defer server.Close()
			fault := &cursorRecoveryLostAck{next: server.Client().Transport}
			client := &runner.Client{BaseURL: server.URL, Token: h.token, ProjectID: f.project, HTTP: &http.Client{Transport: fault, Timeout: 5 * time.Second}}
			if pool {
				client.Pool = &runner.PoolContext{PoolID: assignment.PoolID, RunnerID: assignment.PoolRunnerID, Assignment: &assignment}
			}
			path := t.TempDir()
			journal, err := runner.OpenJournal(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if journal != nil {
					_ = journal.Close()
				}
			}()
			if err = journal.Accept(assignment); err != nil {
				t.Fatal(err)
			}
			result := runner.Result{Assignment: assignment, OperationID: newID(), ExitCode: -1, OutcomeUnknown: true, Error: "Cursor foreground host shutdown requires reconciliation"}
			if err = journal.Finish(result); err != nil {
				t.Fatal(err)
			}
			if err = runner.Replay(ctx, client, journal); err == nil || !fault.dropped || len(journal.Pending()) != 1 {
				t.Fatal("lost completion receipt did not leave the original result pending")
			}
			readRun := func() dag.Run {
				t.Helper()
				current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
				if e != nil || current.State != "running" {
					t.Fatalf("unverified stop advanced the run: %s %v", current.State, e)
				}
				found := false
				for _, node := range current.Nodes {
					if node.ID == assignment.NodeID {
						found = true
						if node.State != "reconciling" || node.Attempt != assignment.Attempt || node.Fence != assignment.Fence || node.WorkerID != assignment.WorkerID || len(node.WorkspaceEvidence) != 0 {
							t.Fatal("the original attempt identity or lack of workspace evidence was lost")
						}
					}
					if node.ID == "test" && node.Attempt != 0 {
						t.Fatal("independent test ran on an unverified candidate")
					}
				}
				if !found {
					t.Fatal("the original development node disappeared")
				}
				return current
			}
			first := readRun()
			if err = journal.Close(); err != nil {
				t.Fatal(err)
			}
			journal, err = runner.OpenJournal(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = runner.Replay(ctx, client, journal); err != nil || len(journal.Pending()) != 0 || len(fault.bodies) != 2 || !bytes.Equal(fault.bodies[0], fault.bodies[1]) {
				t.Fatal("restart did not recover the byte-identical result receipt", err)
			}
			if readRun().Version != first.Version || cursorACPClaim189(t, h)["assignment"] != nil {
				t.Fatal("receipt replay changed the run or reissued the unresolved work")
			}
			usage, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
			if err != nil || len(usage.Items) != 1 || usage.Items[0].State != "reconciling" || usage.Items[0].ReservedTokens != 0 {
				t.Fatal("replay lost the original observation ledger or created a second attempt", err)
			}
		})
	}
}
