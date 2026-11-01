package cli

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

	"github.com/ai-mada/suma/internal/platform"
)

func TestToolReplacementCLIConfirmedOriginalOperationAndHistory(t *testing.T) {
	profile := toolMatchCLIProfile(t)
	request := platform.ToolProfileReplacementRequest{ProjectID: "project200", ExpectedVersion: 2, WorkerID: "worker-new200", Name: "Reviewed replacement", Profile: profile, PauseOriginal: true, IdempotencyKey: "original-tool-replacement200"}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	original := platform.ToolProfile{ID: "old200", ProjectID: request.ProjectID, WorkerID: "worker-old200", Name: "Original", Version: 3, State: "paused", Profile: profile, CreatedAt: stamp, UpdatedAt: stamp}
	target := platform.ToolProfile{ID: "new200", ProjectID: request.ProjectID, WorkerID: request.WorkerID, Name: request.Name, Version: 1, State: "paused", Profile: profile, CreatedAt: stamp, UpdatedAt: stamp}
	reply := platform.ToolProfileReplacement{ID: "relation200", ProjectID: request.ProjectID, Original: original, Replacement: target, Disposition: "created", PauseOriginal: true, CreatedAt: stamp, CreatedBy: strings.Repeat("a", 32)}
	response, _ := json.Marshal(reply)
	posts, gets := 0, 0
	drop := false
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) || r.Header.Get("Authorization") != "Bearer controlled-session200" || r.URL.Query().Get("project_id") != request.ProjectID {
			t.Error("original caller or project view changed")
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPost {
			posts++
			bodies = append(bodies, string(body))
			if r.URL.Path != "/api/v1/tool-profiles/old200/replacements" {
				t.Error("replacement changed original path")
			}
			if drop {
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = connection.Close()
				return
			}
			w.WriteHeader(201)
			_, _ = w.Write(response)
			return
		}
		gets++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tool-profiles/old200/replacements" || len(body) != 0 {
			t.Error("history sent a write or changed original target")
		}
		direction := r.URL.Query().Get("direction")
		if direction != "outgoing" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("cursor") != "opaque200" {
			t.Error("history changed filters or cursor")
		}
		_ = json.NewEncoder(w).Encode(platform.ToolProfileReplacementList{ProjectID: request.ProjectID, ProfileID: "old200", Direction: direction, Items: []platform.ToolProfileReplacement{reply}})
	}))
	defer server.Close()
	run := func(body, language string, args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), append([]string{"--control", server.URL, "--project", request.ProjectID, "--token", "controlled-session200", "--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32), "--language", language, "tools"}, args...), strings.NewReader(body), &out, &stderr)
		return code, out.String(), stderr.String()
	}
	raw, _ := json.Marshal(request)
	for _, args := range [][]string{{"replace", "--id", "old200", "--file", "-"}, {"replace", "--id", "../escape", "--file", "-", "--confirm"}, {"replace", "--id", "old200", "--confirm"}} {
		if code, out, _ := run(string(raw), "en-US", args...); code != 2 || out != "" {
			t.Fatal("unconfirmed replacement accepted", code, out)
		}
	}
	mutate := func(edit func(map[string]json.RawMessage)) string {
		var value map[string]json.RawMessage
		_ = json.Unmarshal(raw, &value)
		edit(value)
		changed, _ := json.Marshal(value)
		return string(changed)
	}
	for _, body := range []string{
		mutate(func(v map[string]json.RawMessage) { delete(v, "pause_original") }),
		mutate(func(v map[string]json.RawMessage) { delete(v, "idempotency_key") }),
		mutate(func(v map[string]json.RawMessage) { v["project_id"] = json.RawMessage(`"other"`) }),
		mutate(func(v map[string]json.RawMessage) { v["expected_version"] = json.RawMessage(`2.0`) }),
		mutate(func(v map[string]json.RawMessage) { v["profile"] = json.RawMessage(`null`) }),
		mutate(func(v map[string]json.RawMessage) { v["reuse"] = json.RawMessage(`null`) }),
		mutate(func(v map[string]json.RawMessage) {
			v["reuse"] = json.RawMessage(`{"profile_id":"new200","expected_version":1}`)
			v["name"] = json.RawMessage(`""`)
		}),
		strings.Replace(string(raw), `"pause_original":true`, `"pause_original":true,"pause_original":false`, 1),
	} {
		if code, out, _ := run(body, "en-US", "replace", "--id", "old200", "--file", "-", "--confirm"); code != 2 || out != "" {
			t.Fatal("unsafe replacement body sent", code, out)
		}
	}
	if posts != 0 || gets != 0 {
		t.Fatal("invalid replacement reached HTTP")
	}
	drop = true
	code, out, stderr := run(string(raw), "en-US", "replace", "--id", "old200", "--file", "-", "--confirm")
	if code != 1 || out != "" || posts != 1 || !strings.Contains(stderr, "original") {
		t.Fatalf("lost ACK was retried or lost recovery guidance: %d %s %s", code, out, stderr)
	}
	drop = false
	code, out, stderr = run(string(raw), "en-US", "replace", "--id", "old200", "--file", "-", "--confirm")
	if code != 0 || stderr != "" || posts != 2 || bodies[0] != bodies[1] || !strings.Contains(out, "relation200") {
		t.Fatalf("same original operation recovery failed: %d %s %s", code, out, stderr)
	}
	var sent platform.ToolProfileReplacementRequest
	if json.Unmarshal([]byte(bodies[1]), &sent) != nil || sent.IdempotencyKey != request.IdempotencyKey || sent.PauseOriginal != request.PauseOriginal {
		t.Fatal("CLI invented key or changed original intent")
	}
	for _, change := range []struct {
		name string
		edit func(*platform.ToolProfileReplacement)
	}{
		{"project", func(r *platform.ToolProfileReplacement) { r.ProjectID = "other" }},
		{"original", func(r *platform.ToolProfileReplacement) { r.Original.ID = "other" }},
		{"worker", func(r *platform.ToolProfileReplacement) { r.Replacement.WorkerID = "other" }},
		{"profile", func(r *platform.ToolProfileReplacement) { r.Replacement.Profile.ToolVersion = "changed" }},
		{"created_enabled", func(r *platform.ToolProfileReplacement) { r.Replacement.State = "enabled" }},
		{"pause", func(r *platform.ToolProfileReplacement) { r.PauseOriginal = false }},
		{"actor", func(r *platform.ToolProfileReplacement) { r.CreatedBy = strings.Repeat("b", 32) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			bad := reply
			change.edit(&bad)
			response, _ = json.Marshal(bad)
			if code, out, stderr := run(string(raw), "en-US", "replace", "--id", "old200", "--file", "-", "--confirm"); code != 1 || out != "" || !strings.Contains(stderr, "original") {
				t.Fatal("unbound ACK accepted or original recovery guidance lost", code, out, stderr)
			}
		})
	}
	response, _ = json.Marshal(reply)
	// Reuse is explicit, and false is a required intent rather than a missing
	// boolean default. Neither mode may silently create or enable another tool.
	falseRequest := request
	falseRequest.PauseOriginal = false
	falseRequest.IdempotencyKey = "explicit-no-pause200"
	falseReply := reply
	falseReply.PauseOriginal = false
	falseReply.Original.Version = request.ExpectedVersion
	falseReply.Original.State = "enabled"
	response, _ = json.Marshal(falseReply)
	falseRaw, _ := json.Marshal(falseRequest)
	if code, out, stderr := run(string(falseRaw), "en-US", "replace", "--id", "old200", "--file", "-", "--confirm"); code != 0 || stderr != "" || !strings.Contains(out, `"pause_original": false`) {
		t.Fatal("explicit false intent was dropped", code, out, stderr)
	}
	reuseRequest := request
	reuseRequest.Name = ""
	reuseRequest.Reuse = &platform.ToolProfileReplacementReuse{ProfileID: "existing200", ExpectedVersion: 4}
	reuseRequest.IdempotencyKey = "explicit-reuse200"
	reuseReply := reply
	reuseReply.Disposition = "reused"
	reuseReply.Replacement.ID = "existing200"
	reuseReply.Replacement.Version = 4
	reuseReply.Replacement.State = "enabled"
	reuseReply.Replacement.Name = "Existing reviewed profile"
	response, _ = json.Marshal(reuseReply)
	reuseRaw, _ := json.Marshal(reuseRequest)
	if code, out, stderr := run(string(reuseRaw), "en-US", "replace", "--id", "old200", "--file", "-", "--confirm"); code != 0 || stderr != "" || !strings.Contains(out, `"disposition": "reused"`) {
		t.Fatal("explicit existing target changed", code, out, stderr)
	}
	response, _ = json.Marshal(reply)
	beforePosts := posts
	code, out, stderr = run("", "zh-CN", "replacements", "--id", "old200", "--direction", "outgoing", "--limit", "2", "--cursor", "opaque200")
	if code != 0 || stderr != "" || gets != 1 || posts != beforePosts || !strings.Contains(out, "relation200") {
		t.Fatal("read-only replacement history failed", code, out, stderr)
	}
	if code, out, _ := run("", "en-US", "replacements", "--id", "old200", "--direction", "all"); code != 2 || out != "" || gets != 1 {
		t.Fatal("invalid history filters reached API")
	}
	for _, language := range []string{"zh-CN", "en-US"} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), []string{"--language", language, "tools", "replace", "--help"}, nil, &out, &stderr); code != 0 || !strings.Contains(out.String(), "--confirm") || !strings.Contains(out.String(), "replacements") || !strings.Contains(out.String(), "SUMA_TOKEN") {
			t.Fatal("replacement localized help missing", language, code, out.String(), stderr.String())
		}
	}
}
