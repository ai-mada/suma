package localization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

func TestNativeCatalogSnapshotsCannotChangeFormattingOrExpandTheAllowlist(t *testing.T) {
	before := NativeCatalog()
	if len(before) != 65 {
		t.Fatalf("native snapshot count: %d", len(before))
	}
	for _, definition := range before {
		if !IsNativeMessageKey(definition.Key) {
			t.Fatal("native key was not recognized", definition.Key)
		}
	}
	for _, key := range []string{"native.future.new", "native.", "native.endpoint", "common.forbidden", "toString", "native.endpoint.rejected\n", "Native.endpoint.rejected"} {
		if IsNativeMessageKey(key) {
			t.Fatal("untrusted key accepted", key)
		}
	}
	for i := range before {
		before[i].Key = "native.future.new"
		before[i].SourceTemplate = "CORRUPTED"
		before[i].Texts[EnUS] = "CORRUPTED"
		if len(before[i].Parameters) > 0 {
			before[i].Parameters[0].Name = "secret"
		}
	}
	for _, definition := range NativeCatalog() {
		if definition.Key == "native.future.new" || definition.SourceTemplate == "CORRUPTED" || definition.Texts[EnUS] == "CORRUPTED" {
			t.Fatal("snapshot mutated shared native definitions")
		}
	}
	got, err := Format(EnUS, "native.endpoint.rejected", Params{"status": 0})
	if err != nil || !got.Localized || got.Text != "Endpoint rejected request (HTTP 0)" {
		t.Fatal("snapshot mutation affected rendering", got, err)
	}
	if IsNativeMessageKey("native.future.new") {
		t.Fatal("snapshot expanded native allowlist")
	}
}

func TestNativeCatalogRenderingKeepsExactParameterGrammarAndValidation(t *testing.T) {
	for _, definition := range NativeCatalog() {
		params := Params{}
		if definition.Key == "native.endpoint.rejected" {
			params["status"] = int64(0)
		}
		if definition.Key == "native.catalog.probe_exited" {
			params["status"] = "{{.other}} <tag> 原始"
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			got, err := Format(locale, definition.Key, params)
			want := definition.Texts[locale]
			if status, exists := params["status"]; exists {
				text := "0"
				if value, ok := status.(string); ok {
					text = value
				}
				want = strings.ReplaceAll(want, "{{.status}}", text)
			}
			if err != nil || !got.Localized || got.Text != want {
				t.Fatal(definition.Key, locale, got, err)
			}
		}
	}
	for _, status := range []string{"", string([]byte{0xff}), "\x00", "\u0085", strings.Repeat("a", 513), strings.Repeat("码", 171)} {
		if _, err := Format(EnUS, "native.catalog.probe_exited", Params{"status": status}); !errors.Is(err, ErrInvalidParameters) {
			t.Fatal("invalid UTF8/control/oversized text accepted", err)
		}
	}
	for _, status := range []string{strings.Repeat("a", 512), strings.Repeat("码", 170), strings.Repeat("😀", 128)} {
		got, err := Format(EnUS, "native.catalog.probe_exited", Params{"status": status})
		if err != nil || got.Text != "CLI probe exited "+status {
			t.Fatal("valid UTF8 boundary rejected", err)
		}
	}
	for _, params := range []Params{nil, {"status": -1}, {"status": 1.5}, {"status": "429"}, {"status": int64(1 << 53)}, {"status": 429, "secret": "DO_NOT_ECHO"}} {
		if _, err := Format(EnUS, "native.endpoint.rejected", params); !errors.Is(err, ErrInvalidParameters) {
			t.Fatal("invalid native integer parameters accepted", err)
		}
	}
	if _, ok := LookupSource("Agent aborted or timed out"); ok {
		t.Fatal("native English provenance became a source lookup alias")
	}
}

// This receipt binds all 46 original SDK diagnostic texts and parameter kinds
// from 7ff1b1453a4beaf8893855296b39683c907bf048. Only placeholder spelling was
// normalized from {{status}} to the shared named-dot-v1 {{.status}} grammar.
func TestNativeCatalogPreservesCompleteOriginalBilingualContent(t *testing.T) {
	type receipt struct {
		Key    string      `json:"key"`
		En     string      `json:"en"`
		Zh     string      `json:"zh"`
		Params []Parameter `json:"params"`
	}
	rows := []receipt{}
	for _, d := range Catalog() {
		if !strings.HasPrefix(d.Key, "native.") {
			continue
		}
		// The new workspace/planning vocabulary has paired SDK checks. Keep
		// this historical golden bound to the original 46 meanings unchanged.
		switch d.Key {
		case "native.operations.tools_forbidden", "native.operations.output_too_large",
			"native.dispatch_recommendation.request_limit", "native.dispatch_recommendation.invalid_goal", "native.dispatch_recommendation.output_invalid", "native.dispatch_recommendation.tools_forbidden", "native.dispatch_recommendation.output_too_large",
			"native.workspace.config", "native.workspace.catalog", "native.workspace.transport", "native.workspace.effects", "native.request.invalid_purpose", "native.knowledge.tools_forbidden",
			"native.failure.tools_forbidden", "native.workspace.merge_unresolved", "native.workspace.merge_incomplete", "native.execution.invalid_limits",
			"native.model.binding_invalid", "native.model.identity_mismatch", "native.model.context_exceeded":
			continue
		}
		if d.SourceTemplate != d.Texts[EnUS] {
			t.Fatalf("native original English source is not explicit: %s", d.Key)
		}
		rows = append(rows, receipt{d.Key, d.Texts[EnUS], d.Texts[ZhCN], append([]Parameter{}, d.Parameters...)})
	}
	if len(rows) != 46 {
		t.Fatalf("native diagnostic count: got %d, want 46", len(rows))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != "f5024b58f1cf720fc052163bc7735cf392dd36eb9994c325009f339bd45805bb" {
		t.Fatalf("native catalog no longer preserves the 46 original diagnostic definitions: %s", got)
	}
}
