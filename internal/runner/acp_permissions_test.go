package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

func acpPermissionFixture(t *testing.T) (Assignment, tc.Profile, pc.Request) {
	t.Helper()
	spec := acpRunnerSpec()
	capability, _ := localACPCapability(spec)
	profile := tc.Profile{CommandID: "qoder-local", Adapter: "qoder", Protocol: ac.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: spec.ACP.ReviewedVersion, ACPExecution: capability}
	a := Assignment{RunID: "run170", NodeID: "node170", Attempt: 1, Fence: 1, WorkerID: "worker170", DispatchID: "dispatch170", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"acp_execution": capability, "tool_profile_sha256": profile.ProfileSHA256}}
	r := pc.Request{IdempotencyKey: "request170-immutable", Protocol: ac.Protocol, ACP: &ac.NativeSource{SessionID: "session170", ToolCallID: "tool170", Title: "Review local change", Kind: "edit", Details: "exact safe operation details", DetailsComplete: true, Options: []ac.PermissionOption{{ID: "once", Name: "Allow once", Kind: "allow_once"}, {ID: "reject", Name: "Reject once", Kind: "reject_once"}}}}
	return a, profile, r
}

func TestACPPermissionJournalPreservesOriginalRequestAndCrashIsUnknown(t *testing.T) {
	a, profile, request := acpPermissionFixture(t)
	directory := filepath.Join(t.TempDir(), "journal")
	j, err := OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.beginToolPermissionExecution(a, profile); err != nil {
		t.Fatal(err)
	}
	if err = j.recordToolPermission(a, request); err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.ACP = &ac.NativeSource{}
	*changed.ACP = *request.ACP
	changed.ACP.Details = "different body"
	if j.recordToolPermission(a, changed) == nil {
		t.Fatal("same operation key accepted different review details")
	}
	if j.beginToolPermissionExecution(a, profile) == nil {
		t.Fatal("original process send permit was reconstructed")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	pending := j.Pending()
	if len(pending) != 1 || !pending[0].OutcomeUnknown {
		t.Fatal("restart recovered permission by rerunning the agent")
	}
	j.mu.Lock()
	saved := j.entries[key(a)].ToolPermissions.Requests[request.IdempotencyKey].Request
	j.mu.Unlock()
	if saved.ACP.Details != request.ACP.Details || saved.IdempotencyKey != request.IdempotencyKey {
		t.Fatal("original safe request was not retained")
	}
}

func TestACPPermissionHTTPRecoveryUsesSameBodyAndCurrentSource(t *testing.T) {
	a, profile, request := acpPermissionFixture(t)
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	_ = j.Accept(a)
	_ = j.beginToolPermissionExecution(a, profile)
	var mu sync.Mutex
	var opens [][]byte
	polls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			ProjectID    string     `json:"project_id"`
			Assignment   Assignment `json:"assignment"`
			Request      pc.Request `json:"request"`
			PermissionID string     `json:"permission_id"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid permission HTTP request")
			w.WriteHeader(400)
			return
		}
		if body.ProjectID != "project170" || body.Assignment.RunID != a.RunID || body.Assignment.Fence != a.Fence {
			t.Error("permission machine execution source changed")
		}
		permission := pc.Permission{ID: "permission170", ProjectID: "project170", RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: profile.ProfileSHA256, Version: 1, State: "pending", Request: request, CreatedAt: "2026-10-31T00:00:00Z", ExpiresAt: "2026-10-31T00:10:00Z", AllowedActions: []string{}}
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/runner/tool-permissions":
			raw, _ := json.Marshal(body.Request)
			opens = append(opens, raw)
			if len(opens) == 1 {
				w.WriteHeader(503)
				return
			}
		case "/api/v1/runner/tool-permissions/poll":
			polls++
			permission.State = "decided"
			permission.Version = 2
			permission.Decision = &pc.Decision{Choice: pc.Choice{Protocol: ac.Protocol, ACP: &ac.Choice{OptionID: "once", Kind: "allow_once"}}, ActorID: "human170", DecidedAt: "2026-10-31T00:00:01Z"}
		default:
			t.Error("unexpected permission endpoint")
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(permission)
	}))
	defer s.Close()
	w := &Worker{Client: &Client{BaseURL: s.URL, ProjectID: "project170", Token: "private-runner-token", HTTP: s.Client()}, Journal: j}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	decision, err := w.waitToolPermission(ctx, a, request, nil)
	if err != nil || decision.ACP.OptionID != "once" {
		t.Fatalf("original permission recovery failed: %v", err)
	}
	if len(opens) != 2 || string(opens[0]) != string(opens[1]) || polls != 1 {
		t.Fatal("permission recovery changed the body/key or issued a second decision")
	}
}

func TestACPPermissionSanitizingCannotApproveAlteredDetails(t *testing.T) {
	_, _, request := acpPermissionFixture(t)
	request.ACP.Details = "execute with private-credential170"
	safe, err := safeToolPermission(request, []string{"private-credential170"})
	if err != nil || safe.ACP.DetailsComplete || strings.Contains(safe.ACP.Details, "private-credential170") {
		t.Fatal("redacted review details remained approvable")
	}
	request.ACP.Options[0].ID = "private-credential170"
	if _, err = safeToolPermission(request, []string{"private-credential170"}); err == nil {
		t.Fatal("a redacted opaque option ID was silently rewritten")
	}
}
