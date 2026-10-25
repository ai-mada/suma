package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The provider runs after the source preflight has committed. With the fixture's
// one-connection pool, logout also proves network IO holds no SQL connection.
// Both empty and populated replies require a new current credential snapshot.
func TestAuthoritySearchReplyRechecksAfterProviderIO(t *testing.T) {
	for _, empty := range []bool{true, false} {
		name := "populated"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			doc := f.call("POST", "/documents", f.body(map[string]any{"title": "private reply source"}), 201)
			logoutResults := make(chan error, 1)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				logoutResults <- logoutCredentialForTest(r.Context(), f.s, f.cookie.Value)
				items := []any{}
				if !empty {
					items = append(items, map[string]any{"source_ref": doc["id"], "revision": doc["version"], "snippet": "private provider body"})
				}
				json.NewEncoder(w).Encode(map[string]any{"items": items})
			}))
			defer provider.Close()
			t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
			t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fixture-provider-only")
			result, err := f.s.SearchIndex(context.Background(), f.actor, f.project, "private")
			var logoutErr error
			select {
			case logoutErr = <-logoutResults:
			default:
				t.Fatalf("provider was not reached: result=%v err=%v", result, err)
			}
			var api *APIError
			if logoutErr != nil || !errors.As(err, &api) || api.Status != 401 || result != nil {
				t.Fatalf("provider reply bypassed logout: result=%v err=%v logout=%v", result, err, logoutErr)
			}
		})
	}
}

func TestAuthorityScopedAgentReadSurvivesBrowserLogoutOnlyWithinGrant(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/tasks", f.body(map[string]any{"title": "agent visible task"}), 201)
	credential, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "agent", Label: "browser", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.AuthenticateActor(context.Background(), credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	search, err := f.s.SearchKeywords(context.Background(), actor, f.project, "agent")
	if err != nil || search["total"] != 1 {
		t.Fatalf("independent agent read lost: %v %v", search, err)
	}
	if _, err = f.s.SearchKeywords(context.Background(), actor, "foreign-project", "agent"); err == nil {
		t.Fatal("agent read escaped project grant")
	}
	// General business writes remain unavailable to an agent read credential.
	if _, err = f.s.Create(context.Background(), actor, f.project, "tasks", map[string]any{"title": "forbidden", "idempotency_key": newID()}); err == nil {
		t.Fatal("agent read acquired general task write")
	}
}
