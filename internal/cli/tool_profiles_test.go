package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func toolMatchCLIProfile(t *testing.T) tc.Profile {
	t.Helper()
	profile := tc.Profile{CommandID: "inspect180", Adapter: "command", Protocol: "text-v1", ReadOnly: true, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "controlled180"}
	if err := tc.ValidateProfile(profile, true); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestToolMatchesCLIRejectsIncompleteInputAndResponse(t *testing.T) {
	profile := toolMatchCLIProfile(t)
	request := platform.ToolProfileMatchRequest{ProjectID: "project-one", WorkerID: "worker180", Profile: profile}
	raw, _ := json.Marshal(request)
	calls := 0
	mode := "states"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/tool-profiles/matches" || r.URL.Query().Get("project_id") != request.ProjectID || r.Header.Get("Accept-Language") != "en-US" {
			t.Error("original read transport changed")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 3 {
			t.Fatal("query gained write controls or lost complete request")
		}
		var actual platform.ToolProfileMatchRequest
		encoded, _ := json.Marshal(body)
		_ = json.Unmarshal(encoded, &actual)
		if actual.ProjectID != request.ProjectID || actual.WorkerID != request.WorkerID || !tc.EqualProfile(actual.Profile, profile) {
			t.Fatal("query changed full profile")
		}
		list := platform.ToolProfileMatchList{ProjectID: request.ProjectID, WorkerID: request.WorkerID, Profile: profile, Items: []platform.ToolProfile{}, Total: 0}
		switch mode {
		case "states":
			for _, state := range []string{"paused", "enabled", "revoked"} {
				list.Items = append(list.Items, platform.ToolProfile{ID: state + "180", ProjectID: request.ProjectID, WorkerID: request.WorkerID, Name: "Existing " + state, Version: 2, State: state, Profile: profile, Available: false, Reason: "offline"})
			}
			list.Total = 3
		case "truncated":
			list.Total = 1
		case "same-hash-different-profile":
			list.Profile.ToolVersion = "different180"
		}
		_ = json.NewEncoder(w).Encode(list)
	}))
	defer server.Close()
	run := func(body string, args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		base := []string{"--control", server.URL, "--project", request.ProjectID, "--token", "controlled-session180", "--language", "en-US", "tools", "matches"}
		code := Execute(context.Background(), append(base, args...), strings.NewReader(body), &out, &stderr)
		return code, out.String(), stderr.String()
	}
	mutate := func(change func(map[string]json.RawMessage)) string {
		var body map[string]json.RawMessage
		_ = json.Unmarshal(raw, &body)
		change(body)
		data, _ := json.Marshal(body)
		return string(data)
	}
	bad := []string{
		mutate(func(body map[string]json.RawMessage) { body["project_id"] = json.RawMessage(`"other-project"`) }),
		mutate(func(body map[string]json.RawMessage) { body["profile"] = json.RawMessage(`null`) }),
		mutate(func(body map[string]json.RawMessage) { body["idempotency_key"] = json.RawMessage(`"not-a-write"`) }),
		mutate(func(body map[string]json.RawMessage) {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(body["profile"], &fields)
			delete(fields, "read_only")
			body["profile"], _ = json.Marshal(fields)
		}),
		strings.Replace(string(raw), `"worker_id":"worker180"`, `"worker_id":"worker180","worker_id":"other"`, 1),
		strings.Replace(string(raw), `"tool_version":"controlled180"`, `"tool_version":"\ud800"`, 1),
		strings.Repeat(" ", toolMatchesInputLimit) + string(raw),
	}
	for _, body := range bad {
		if code, _, _ := run(body, "--file", "-"); code != 2 {
			t.Fatal("unsafe match request reached API", code)
		}
	}
	for _, args := range [][]string{{}, {"--file", "-", "--confirm"}} {
		if code, _, _ := run(string(raw), args...); code != 2 {
			t.Fatal("write-style controls accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid input sent HTTP requests", calls)
	}
	file := filepath.Join(t.TempDir(), "profile180.json")
	withoutProject := mutate(func(body map[string]json.RawMessage) { delete(body, "project_id") })
	if err := os.WriteFile(file, []byte(withoutProject), 0600); err != nil {
		t.Fatal(err)
	}
	if code, out, stderr := run("", "--file", file); code != 0 || !strings.Contains(out, "revoked180") || !strings.Contains(out, `"available": false`) {
		t.Fatal("file query lost history or invented health", code, out, stderr)
	}
	for _, badResponse := range []string{"truncated", "same-hash-different-profile"} {
		mode = badResponse
		if code, out, _ := run(string(raw), "--file", "-"); code != 1 || out != "" {
			t.Fatal("incomplete or different result silently became matches", badResponse, code, out)
		}
	}
	if calls != 3 {
		t.Fatal("query triggered extra requests or automatic writes", calls)
	}
}

type toolMatchTimeout180 struct{}

func (toolMatchTimeout180) RoundTrip(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestToolMatchesCLIReadOnlyTimeoutAndHelp(t *testing.T) {
	request := platform.ToolProfileMatchRequest{ProjectID: "project-one", WorkerID: "worker180", Profile: toolMatchCLIProfile(t)}
	raw, _ := json.Marshal(request)
	c, err := newClient("http://127.0.0.1:1", request.ProjectID, "controlled-session180", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	c.http = &http.Client{Transport: toolMatchTimeout180{}}
	err = toolProfilesCommand(context.Background(), c, []string{"matches", "--file", "-"}, bytes.NewReader(raw), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "outcome may be unknown") {
		t.Fatal("read-only timeout misreported as an unknown write", err)
	}
	if !requestIsReadOnly("POST", "/api/v1/tool-profiles/matches") || requestIsReadOnly("POST", "/api/v1/tool-profiles") || requestIsReadOnly("PUT", "/api/v1/tool-profiles/matches") {
		t.Fatal("read-only allowlist expanded to mutations")
	}
	for _, language := range []string{"zh-CN", "en-US"} {
		var out, stderr bytes.Buffer
		args := []string{"--language", language, "tools", "--help"}
		if code := Execute(context.Background(), args, nil, &out, &stderr); code != 0 || !strings.Contains(out.String(), "--file -") || !strings.Contains(out.String(), "SUMA_TOKEN") {
			t.Fatal("localized login-session help unavailable", code, out.String(), stderr.String())
		}
	}
}

func TestToolMatchesCLIUsesActualLoginSessionAndReadAPI(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "cli180.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &platform.Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
	if err = service.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&platform.Server{Service: service, Engine: engine, WebDir: t.TempDir()}).Handler())
	defer server.Close()
	setup := `{"name":"CLI180 owner","email":"cli180@example.test","password":"controlled-password-180","organization":"CLI180 org","project":"CLI180 project"}`
	response, err := http.Post(server.URL+"/api/v1/auth/setup", "application/json", strings.NewReader(setup))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("normal setup failed", response.StatusCode, string(raw))
	}
	var session struct {
		User     platform.Principal `json:"user"`
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if json.Unmarshal(raw, &session) != nil || len(session.Projects) != 1 {
		t.Fatal("setup source missing")
	}
	var token string
	for _, cookie := range response.Cookies() {
		if cookie.Name == "suma_session" {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("normal browser session not issued")
	}
	t.Setenv("SUMA_TOKEN", token)
	request := platform.ToolProfileMatchRequest{ProjectID: session.Projects[0].ID, WorkerID: "worker180", Profile: toolMatchCLIProfile(t)}
	body, _ := json.Marshal(request)
	var out, stderr bytes.Buffer
	args := []string{"--control", server.URL, "--project", request.ProjectID, "--expected-user", session.User.ID, "--expected-organization", session.User.TenantID, "--language", "en-US", "tools", "matches", "--file", "-"}
	code := Execute(context.Background(), args, bytes.NewReader(body), &out, &stderr)
	if code != 0 {
		t.Fatalf("actual browser-session CLI read failed: %d %s", code, stderr.String())
	}
	var result platform.ToolProfileMatchList
	if json.Unmarshal(out.Bytes(), &result) != nil || result.ProjectID != request.ProjectID || result.WorkerID != request.WorkerID || !tc.EqualProfile(result.Profile, request.Profile) || result.Items == nil || result.Total != 0 {
		t.Fatal("read result lost exact scope/profile", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte(token)) {
		t.Fatal("session credential escaped output")
	}
}
