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
)

func TestSkillCatalogHelpAndExplicitWriteConfirmation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, locale := range []string{"zh-CN", "en-US"} {
		code, out, stderr := contextInvoke(t, server.URL, locale, "", "skill-catalog", "--help")
		if code != 0 || stderr != "" || !strings.Contains(out, "--confirm") || !strings.Contains(out, "revisions") || locale == "zh-CN" && !strings.Contains(out, "组织技能目录") || locale == "en-US" && !strings.Contains(out, "Organization skill catalog") {
			t.Fatalf("help=%d %s %s", code, out, stderr)
		}
		for _, action := range []string{"promote", "update", "retire", "adopt"} {
			args := []string{"skill-catalog", action}
			if action != "promote" {
				args = append(args, "catalog-1")
			}
			args = append(args, "--file", "-")
			code, _, stderr = contextInvoke(t, server.URL, locale, `{}`, args...)
			if code != 2 || !strings.Contains(stderr, "cli.skill_catalog.confirm_required") {
				t.Fatalf("unconfirmed %s code=%d error=%s", action, code, stderr)
			}
		}
	}
	if calls != 0 {
		t.Fatal("help or unconfirmed write sent a request", calls)
	}
}

func TestSkillCatalogCurrentProjectReadRoutes(t *testing.T) {
	for _, scenario := range []struct {
		args  []string
		path  string
		query map[string]string
	}{
		{[]string{"skill-catalog", "list", "--domain", "engineering", "--scenario", "review", "--tag", "go", "--state", "published", "--query", "原文 & key", "--limit", "5", "--cursor", "next+/="}, "/api/v1/skill-catalog", map[string]string{"domain": "engineering", "scenario": "review", "tag": "go", "state": "published", "q": "原文 & key", "limit": "5", "cursor": "next+/="}},
		{[]string{"skill-catalog", "get", "catalog-1"}, "/api/v1/skill-catalog/catalog-1", map[string]string{}},
		{[]string{"skill-catalog", "revisions", "catalog-1", "--limit", "100", "--cursor", "next+/="}, "/api/v1/skill-catalog/catalog-1/revisions", map[string]string{"limit": "100", "cursor": "next+/="}},
	} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != "GET" || r.URL.Path != scenario.path || r.URL.Query().Get("project_id") != "project-one" || r.Header.Get("Accept-Language") != "en-US" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Suma-Expected-User") != "" || r.ContentLength > 0 {
				t.Errorf("wrong scoped catalog read: %s %s", r.Method, r.URL)
			}
			for name, value := range scenario.query {
				if r.URL.Query().Get(name) != value {
					t.Errorf("query %s changed", name)
				}
			}
			_, _ = w.Write([]byte(`{"version":9007199254740993,"content":"用户技能正文 raw unchanged","allowed_actions":[]}`))
		}))
		code, out, stderr := contextInvoke(t, server.URL, "en-US", "", scenario.args...)
		server.Close()
		if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "用户技能正文 raw unchanged") {
			t.Fatalf("read=%d %d %s %s", code, calls, out, stderr)
		}
	}
}

func skillCatalogRequestJSON(action string) string {
	body := map[string]any{"expected_version": json.Number("9007199254740993"), "reason": "明确审核 原文 unchanged", "idempotency_key": "original-skill-operation"}
	switch action {
	case "promote", "update":
		body["knowledge_id"] = "knowledge-1"
		body["expected_knowledge_version"] = json.Number("9007199254740993")
		body["published_revision"] = json.Number("2")
		body["published_sha256"] = strings.Repeat("a", 64)
		body["acknowledge_sharing"] = true
		body["domain"] = "engineering"
		body["scenarios"] = []string{"review"}
		body["tags"] = []string{"go"}
		if action == "promote" {
			body["expected_version"] = json.Number("0")
		}
	case "adopt":
		body["origin_sha256"] = strings.Repeat("b", 64)
		body["acknowledge_draft"] = true
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func TestSkillCatalogExplicitWriteRoutesAndExactBasis(t *testing.T) {
	for _, scenario := range []struct{ action, method, path string }{{"promote", "POST", "/api/v1/skill-catalog"}, {"update", "PATCH", "/api/v1/skill-catalog/catalog-1"}, {"retire", "POST", "/api/v1/skill-catalog/catalog-1/commands"}, {"adopt", "POST", "/api/v1/skill-catalog/catalog-1/adoptions"}} {
		calls := 0
		input := skillCatalogRequestJSON(scenario.action)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			raw, _ := io.ReadAll(r.Body)
			if r.Method != scenario.method || r.URL.Path != scenario.path || r.URL.Query().Get("project_id") != "project-one" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
				t.Errorf("wrong governed write: %s %s", r.Method, r.URL)
			}
			for _, exact := range []string{`"idempotency_key":"original-skill-operation"`, `"project_id":"project-one"`, `"reason":"明确审核 原文 unchanged"`} {
				if !strings.Contains(string(raw), exact) {
					t.Error("write changed exact basis", exact)
				}
			}
			if scenario.action == "promote" {
				if !strings.Contains(string(raw), `"expected_version":0`) {
					t.Error("promotion invented catalog version")
				}
			} else if !strings.Contains(string(raw), `"expected_version":9007199254740993`) {
				t.Error("catalog version rounded")
			}
			if scenario.action == "retire" && !strings.Contains(string(raw), `"action":"retire"`) {
				t.Error("explicit retirement action absent")
			}
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"当前服务端批准被拒绝 raw authority","details":{"version":9007199254740993}}}`))
		}))
		args := []string{"skill-catalog", scenario.action}
		if scenario.action != "promote" {
			args = append(args, "catalog-1")
		}
		args = append(args, "--file", "-", "--confirm")
		code, _, stderr := contextInvoke(t, server.URL, "zh-CN", input, args...)
		server.Close()
		if code != 1 || calls != 1 || !strings.Contains(stderr, "当前服务端批准被拒绝 raw authority") || !strings.Contains(stderr, "9007199254740993") {
			t.Fatalf("write %s=%d %d %s", scenario.action, code, calls, stderr)
		}
		if requestIsReadOnly(scenario.method, scenario.path) {
			t.Fatal("catalog mutation classified as preview", scenario.path)
		}
	}
	if !requestIsReadOnly("POST", "/api/v1/requirements/r-1/design/preview") || requestIsReadOnly("POST", "/api/v1/skill-catalog/catalog-1/preview") {
		t.Fatal("unknown catalog POST received read-only preview authority")
	}
}

func TestSkillCatalogBoundariesAndUnknownWrites(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, scenario := range []struct {
		args []string
		body string
	}{
		{[]string{"get", "../other"}, ""}, {[]string{"get", "id?project_id=other"}, ""}, {[]string{"list", "--limit", "0"}, ""}, {[]string{"list", "--state", "draft"}, ""}, {[]string{"revisions", "catalog-1", "--limit", "101"}, ""},
		{[]string{"adopt", "catalog-1", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("adopt"), `"acknowledge_draft":true`, `"acknowledge_draft":false`, 1)},
		{[]string{"update", "catalog-1", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("update"), `"acknowledge_sharing":true`, `"acknowledge_sharing":false`, 1)},
		{[]string{"adopt", "catalog-1", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("adopt"), `9007199254740993`, `1.0`, 1)},
		{[]string{"adopt", "catalog-1", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("adopt"), `9007199254740993`, `1e0`, 1)},
		{[]string{"promote", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("promote"), `"expected_version":0`, `"expected_version":1`, 1)},
		{[]string{"adopt", "catalog-1", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("adopt"), `original-skill-operation`, `short`, 1)},
		{[]string{"adopt", "catalog-1", "--file", "-", "--confirm"}, strings.Replace(skillCatalogRequestJSON("adopt"), strings.Repeat("b", 64), strings.Repeat("B", 64), 1)},
		{[]string{"retire", "catalog-1", "--file", "-", "--confirm"}, strings.TrimSuffix(skillCatalogRequestJSON("retire"), "}") + `,"action":"publish"}`},
		{[]string{"adopt", "catalog-1", "--file", "-", "--confirm"}, strings.TrimSuffix(skillCatalogRequestJSON("adopt"), "}") + `,"project_id":"other-project"}`},
	} {
		code, _, stderr := contextInvoke(t, server.URL, "en-US", scenario.body, append([]string{"skill-catalog"}, scenario.args...)...)
		if code != 2 || stderr == "" {
			t.Errorf("invalid catalog command accepted: %v code=%d %s", scenario.args, code, stderr)
		}
	}
	if calls != 0 {
		t.Fatal("invalid catalog input reached HTTP", calls)
	}
	var payloads []string
	unknown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		payloads = append(payloads, string(raw))
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	defer unknown.Close()
	for _, locale := range []string{"en-US", "zh-CN"} {
		code, _, stderr := contextInvoke(t, unknown.URL, locale, skillCatalogRequestJSON("adopt"), "skill-catalog", "adopt", "catalog-1", "--file", "-", "--confirm")
		if code != 1 || !strings.Contains(stderr, "outcome may be unknown") && !strings.Contains(stderr, "结果可能未知") {
			t.Fatalf("unknown write=%d %s", code, stderr)
		}
	}
	if len(payloads) != 2 || payloads[0] != payloads[1] || !strings.Contains(payloads[0], `"idempotency_key":"original-skill-operation"`) {
		t.Fatal("automatic retry or changed original request", payloads)
	}
	t.Setenv("SUMA_EXPECTED_USER", "")
	t.Setenv("SUMA_EXPECTED_ORGANIZATION", "")
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "--language", "en-US", "skill-catalog", "get", "catalog-1"}, strings.NewReader(""), &out, &stderr)
	if code != 2 || calls != 0 {
		t.Fatal("organization directory read bypassed project scope", code, stderr.String())
	}
}
