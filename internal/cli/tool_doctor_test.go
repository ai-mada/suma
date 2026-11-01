package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func toolDoctorReport181(t *testing.T) platform.ToolProfileDiagnostics {
	t.Helper()
	at := time.Now().UTC().Truncate(time.Second)
	profile := toolMatchCLIProfile(t)
	return platform.ToolProfileDiagnostics{
		Tool:      platform.ToolProfile{ID: "profile181", ProjectID: "project-one", WorkerID: "worker181", Name: "Controlled tool", Version: 2, State: "enabled", Profile: profile, EnabledBy: strings.Repeat("a", 32), CreatedAt: at.Add(-time.Hour).Format(time.RFC3339), UpdatedAt: at.Add(-time.Minute).Format(time.RFC3339), Observation: &tc.Observation{Profile: profile, Available: true}, LastSeenAt: at.Format(time.RFC3339), Available: true, Reason: "Current inventory only; model login remains separate"},
		CheckedAt: at.Format(time.RFC3339), ExpiresAt: at.Add(60 * time.Second).Format(time.RFC3339),
		Checks: []platform.ToolProfileDiagnosticCheck{{Code: "authorization", Status: "pass", Reason: "Current approval", NextAction: "none"}, {Code: "worker", Status: "pass", Reason: "Current identity; online state unknown", NextAction: "none"}, {Code: "inventory", Status: "pass", Reason: "Fresh inventory", NextAction: "none"}, {Code: "profile", Status: "pass", Reason: "Exact full profile", NextAction: "none"}}, NextAction: "none",
	}
}

func TestToolDoctorCLIReadOnlyAndStrictSource(t *testing.T) {
	report := toolDoctorReport181(t)
	response, _ := json.Marshal(report)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "GET" || r.URL.Path != "/api/v1/tool-profiles/profile181/diagnostics" || r.URL.RawQuery != "project_id=project-one" || len(body) != 0 || r.Header.Get("Authorization") != "Bearer controlled-session181" || r.Header.Get("Accept-Language") != "en-US" {
			t.Error("doctor changed original read transport or sent write controls")
		}
		_, _ = w.Write(response)
	}))
	defer server.Close()
	run := func(args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		base := []string{"--control", server.URL, "--project", "project-one", "--token", "controlled-session181", "--language", "en-US", "tools", "doctor"}
		code := Execute(context.Background(), append(base, args...), nil, &out, &stderr)
		return code, out.String(), stderr.String()
	}
	for _, args := range [][]string{{}, {"--id", "../escape"}, {"--id", "profile181", "--confirm"}, {"--id", "profile181", "--operation", "write181"}, {"--id", "profile181", "extra"}} {
		if code, out, _ := run(args...); code != 2 || out != "" {
			t.Fatal("invalid or write-style doctor options accepted", args, code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid doctor options reached API")
	}
	if code, out, stderr := run("--id", "profile181"); code != 0 || !strings.Contains(out, "online state unknown") || !strings.Contains(out, `"available": true`) {
		t.Fatal("valid read-only facts rejected", code, out, stderr)
	}
	for _, change := range []struct {
		name string
		edit func(*platform.ToolProfileDiagnostics)
	}{
		{"project", func(r *platform.ToolProfileDiagnostics) { r.Tool.ProjectID = "other-project" }},
		{"id", func(r *platform.ToolProfileDiagnostics) { r.Tool.ID = "other-profile" }},
		{"worker", func(r *platform.ToolProfileDiagnostics) { r.Tool.WorkerID = "../other" }},
		{"profile", func(r *platform.ToolProfileDiagnostics) { r.Tool.Profile.ExecutableSHA256 = "" }},
		{"version", func(r *platform.ToolProfileDiagnostics) { r.Tool.Version = 0 }},
		{"state", func(r *platform.ToolProfileDiagnostics) { r.Tool.State = "ready" }},
		{"count", func(r *platform.ToolProfileDiagnostics) { r.Checks = r.Checks[:3] }},
		{"order", func(r *platform.ToolProfileDiagnostics) { r.Checks[0], r.Checks[1] = r.Checks[1], r.Checks[0] }},
		{"status", func(r *platform.ToolProfileDiagnostics) { r.Checks[0].Status = "healthy" }},
		{"action", func(r *platform.ToolProfileDiagnostics) { r.Checks[0].NextAction = "run_command" }},
		{"overall_action", func(r *platform.ToolProfileDiagnostics) { r.NextAction = "enable" }},
		{"reason", func(r *platform.ToolProfileDiagnostics) { r.Checks[0].Reason = "" }},
		{"long_deadline", func(r *platform.ToolProfileDiagnostics) {
			r.ExpiresAt = time.Now().Add(5 * time.Minute).Format(time.RFC3339)
		}},
		{"expired", func(r *platform.ToolProfileDiagnostics) { r.ExpiresAt = r.CheckedAt }},
		{"invalid_time", func(r *platform.ToolProfileDiagnostics) { r.CheckedAt = "yesterday" }},
		{"inventory_deadline", func(r *platform.ToolProfileDiagnostics) {
			r.Tool.LastSeenAt = time.Now().Add(-30 * time.Second).Format(time.RFC3339)
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			bad := toolDoctorReport181(t)
			change.edit(&bad)
			response, _ = json.Marshal(bad)
			if code, out, _ := run("--id", "profile181"); code != 1 || out != "" {
				t.Fatal("unbound or incomplete diagnostic escaped", code, out)
			}
		})
	}
	valid, _ := json.Marshal(report)
	for _, raw := range []string{strings.Replace(string(valid), `"available":true`, `"available":true,"available":false`, 1), strings.Replace(string(valid), `,"read_only":true`, "", 1), strings.Replace(string(valid), `"reason":"Current approval"`, `"reason":"\ud800"`, 1)} {
		response = []byte(raw)
		if code, out, _ := run("--id", "profile181"); code != 1 || out != "" {
			t.Fatal("permissive JSON changed original contract", code, out)
		}
	}
}

func TestToolDoctorCLIDeadlineTimeoutAndHelp(t *testing.T) {
	report := toolDoctorReport181(t)
	// Database wall time need not match this machine. Only the database TTL
	// minus this invocation's monotonic network duration bounds display use.
	for _, offset := range []time.Duration{-24 * time.Hour, 24 * time.Hour} {
		shifted := report
		at := time.Now().UTC().Add(offset)
		shifted.CheckedAt = at.Format(time.RFC3339Nano)
		shifted.ExpiresAt = at.Add(time.Minute).Format(time.RFC3339Nano)
		shifted.Tool.LastSeenAt = shifted.CheckedAt
		if !validToolDiagnostics(shifted, shifted.Tool.ProjectID, shifted.Tool.ID, time.Second) {
			t.Fatal("local wall clock incorrectly invalidated database-relative TTL", offset)
		}
	}
	at := time.Now().UTC()
	report.CheckedAt, report.ExpiresAt = at.Format(time.RFC3339Nano), at.Add(time.Millisecond).Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(report)
	}))
	defer server.Close()
	c, err := newClient(server.URL, report.Tool.ProjectID, "controlled-session181", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = toolProfilesCommand(context.Background(), c, []string{"doctor", "--id", report.Tool.ID}, nil, &out); err == nil || out.Len() != 0 {
		t.Fatal("network duration did not consume display deadline", err, out.String())
	}
	c.http = &http.Client{Transport: toolMatchTimeout180{}}
	c.timeout = time.Millisecond
	err = toolProfilesCommand(context.Background(), c, []string{"doctor", "--id", report.Tool.ID}, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "outcome may be unknown") {
		t.Fatal("GET timeout became an uncertain write", err)
	}
	for _, language := range []string{"zh-CN", "en-US"} {
		for _, tail := range [][]string{{"tools", "--help"}, {"tools", "doctor", "--help"}} {
			var stdout, stderr bytes.Buffer
			if code := Execute(context.Background(), append([]string{"--language", language}, tail...), nil, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "tools doctor --id") || !strings.Contains(stdout.String(), "SUMA_TOKEN") {
				t.Fatal("localized doctor login-session help missing", language, code, stdout.String(), stderr.String())
			}
		}
	}
}

func TestToolDoctorCLIActualSQLiteLoginSessionGET(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "doctor181.db"))
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
	handler := (&platform.Server{Service: service, Engine: engine, WebDir: t.TempDir()}).Handler()
	doctorGETs, otherCLIRequests := 0, 0
	observing := false
	var views [][2]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if observing {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/diagnostics") {
				doctorGETs++
				views = append(views, [2]string{r.Header.Get("X-Suma-Expected-User"), r.Header.Get("X-Suma-Expected-Organization")})
			} else {
				otherCLIRequests++
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	response, err := http.Post(server.URL+"/api/v1/auth/setup", "application/json", strings.NewReader(`{"name":"CLI181 owner","email":"cli181@example.test","password":"controlled-password-181","organization":"CLI181 org","project":"CLI181 project"}`))
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
		t.Fatal("normal session missing scope")
	}
	var token string
	for _, cookie := range response.Cookies() {
		if cookie.Name == "suma_session" {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("normal browser session missing")
	}
	ctx := context.Background()
	actor, err := service.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	project := session.Projects[0].ID
	worker, err := service.RegisterReleaseWorker(ctx, actor, project, map[string]any{"project_id": project, "worker_id": "worker181", "profiles": []any{}, "deployment": false, "idempotency_key": "worker-registration181"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := service.CreateToolProfile(ctx, actor, project, platform.ToolProfileCreateRequest{ProjectID: project, WorkerID: "worker181", Name: "Actual local diagnostics", Profile: toolMatchCLIProfile(t), IdempotencyKey: "profile-registration181"})
	if err != nil {
		t.Fatal(err)
	}
	workerActor, err := service.AuthenticateActor(ctx, worker["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReportToolInventory(ctx, workerActor, project, platform.ToolInventoryRequest{ProjectID: project, WorkerID: "worker181", OperationID: "inventory-report181", Items: []tc.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	before := ""
	if err = db.QueryRow(`SELECT state || ':' || version || ':' || updated_at FROM suma_tool_profiles WHERE id=?`, profile.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUMA_TOKEN", token)
	var out, stderr bytes.Buffer
	observing = true
	code := Execute(ctx, []string{"--control", server.URL, "--project", project, "--expected-user", session.User.ID, "--expected-organization", session.User.TenantID, "--language", "en-US", "tools", "doctor", "--id", profile.ID}, nil, &out, &stderr)
	observing = false
	if code != 0 {
		t.Fatal("actual SQLite/session diagnostic read failed", code, stderr.String())
	}
	var result platform.ToolProfileDiagnostics
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Tool.ID != profile.ID || result.Tool.ProjectID != project || !tc.EqualProfile(result.Tool.Profile, profile.Profile) || result.Tool.State != "paused" || result.Tool.Available || len(result.Checks) != 4 || result.Checks[0].Status != "blocked" || result.NextAction != "review_profile" {
		t.Fatal("actual read lost exact current facts", out.String())
	}
	for _, check := range result.Checks[1:] {
		if check.Status != "pass" || check.NextAction != "none" {
			t.Fatal("paused authorization hid independent facts", check)
		}
	}
	after := ""
	if err = db.QueryRow(`SELECT state || ':' || version || ':' || updated_at FROM suma_tool_profiles WHERE id=?`, profile.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if doctorGETs != 1 || otherCLIRequests != 0 || len(views) != 1 || views[0] != [2]string{session.User.ID, session.User.TenantID} || before != after || bytes.Contains(out.Bytes(), []byte(token)) {
		t.Fatal("doctor performed extra requests, mutation or exposed credential")
	}
	for _, view := range [][2]string{{strings.Repeat("f", 32), session.User.TenantID}, {session.User.ID, strings.Repeat("f", 32)}} {
		out.Reset()
		stderr.Reset()
		observing = true
		code = Execute(ctx, []string{"--control", server.URL, "--project", project, "--expected-user", view[0], "--expected-organization", view[1], "--language", "en-US", "tools", "doctor", "--id", profile.ID}, nil, &out, &stderr)
		observing = false
		var denied struct {
			Error failure `json:"error"`
		}
		if code != 1 || out.Len() != 0 || json.Unmarshal(stderr.Bytes(), &denied) != nil || denied.Error.Status != http.StatusForbidden || denied.Error.Code != "view_context_changed" || views[len(views)-1] != view || bytes.Contains(stderr.Bytes(), []byte(token)) {
			t.Fatal("actual GET ignored original account/organization view", code, out.String(), stderr.String())
		}
	}
	if doctorGETs != 3 || otherCLIRequests != 0 {
		t.Fatal("view rejection triggered unexpected requests")
	}
	t.Log("Actual Service.Initialize/SQLite + auth/setup browser session: 1 successful diagnostics GET + 2 denied account/organization views, 0 other CLI requests, paused profile unchanged; no model/CLI/vendor execution")
}
