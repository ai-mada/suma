package toolcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixtureProfile() Profile {
	return Profile{CommandID: "codex-read", Adapter: "codex", Protocol: "codex-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "codex-cli 0.164.0", ReadOnly: true}
}
func TestToolContractBounds(t *testing.T) {
	p := fixtureProfile()
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Profile){func(p *Profile) { p.Adapter = "opencode" }, func(p *Profile) { p.ProfileSHA256 = "A" + p.ProfileSHA256[1:] }, func(p *Profile) { p.CommandID = "../escape" }, func(p *Profile) { p.ToolVersion = "" }} {
		q := p
		change(&q)
		if ValidateProfile(q, true) == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
	e := Event{SchemaVersion: 1, CommandID: p.CommandID, ProfileSHA256: p.ProfileSHA256, Protocol: p.Protocol, Kind: "turn_completed", SessionID: "thread-1", Usage: &Usage{InputTokens: 3, CacheWriteInputTokens: 2}}
	if err := ValidateEvent(e); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(e)
	if _, err := DecodeEvent(b); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`null`, `{"schema_version":1,"unknown":true}`, strings.TrimSuffix(string(b), "}") + `,"unexpected":{"raw":"secret"}}`, strings.TrimSuffix(string(b), "}") + `,"schema_version":1}`} {
		if _, err := DecodeEvent([]byte(raw)); err == nil {
			t.Fatal("accepted invalid event")
		}
	}
	for _, change := range []func(*Event){func(e *Event) { e.SchemaVersion = 0 }, func(e *Event) { e.Kind = "grant_permissions" }, func(e *Event) { e.Message = strings.Repeat("x", 8193) }, func(e *Event) { e.SessionID = "bad\nidentity" }, func(e *Event) { e.Usage = &Usage{InputTokens: -1} }} {
		q := e
		change(&q)
		if ValidateEvent(q) == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
}
func TestProfileDigestCanonicalPrivateValues(t *testing.T) {
	a, err := ProfileDigest(map[string]any{"env": map[string]string{"key": "private-a"}, "args": []string{"exec"}}, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ProfileDigest(map[string]any{"args": []string{"exec"}, "env": map[string]string{"key": "private-a"}}, strings.Repeat("a", 64))
	if err != nil || a != b {
		t.Fatal("not canonical")
	}
	c, _ := ProfileDigest(map[string]any{"env": map[string]string{"key": "private-b"}, "args": []string{"exec"}}, strings.Repeat("a", 64))
	if c == a || strings.Contains(a, "private") {
		t.Fatal("digest did not bind config")
	}
}
