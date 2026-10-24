package localization

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestAnnotatedErrorPreservesIdentityAndSource(t *testing.T) {
	cause := errors.New("输入无效")
	err := AnnotateError(cause, "common.invalid_input", nil)
	if err.Error() != cause.Error() || !errors.Is(err, cause) {
		t.Fatal("raw cause changed")
	}
	zh, ok := FormatError(ZhCN, err)
	if !ok || zh.Text != cause.Error() {
		t.Fatal(zh, ok)
	}
	en, ok := FormatError(EnUS, err)
	if !ok || en.Text != "The input is invalid." {
		t.Fatal(en, ok)
	}
	if AnnotateError(nil, "common.invalid_input", nil) != nil {
		t.Fatal("nil became error")
	}
	for _, bad := range []error{
		AnnotateError(cause, "common.forbidden", nil),
		AnnotateError(cause, "missing.key", nil),
		AnnotateError(cause, "common.invalid_input", Params{"secret": "private"}),
		fmt.Errorf("context: %w", err), errors.Join(err, errors.New("foreign")),
	} {
		if got, ok := FormatError(EnUS, bad); ok || got.Params != nil {
			t.Fatal("unproven diagnostic exposed", got)
		}
	}
}

type changingDiagnostic struct{ text string }

func (e *changingDiagnostic) Error() string { return e.text }

func TestAnnotatedParametersSnapshotAndConcurrentLocale(t *testing.T) {
	cause := errors.New("缺少project_id")
	params := Params{"field": "project_id"}
	err := AnnotateError(cause, "validation.field_required", params)
	params["field"] = "private"
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			locale := EnUS
			want := "project_id is required."
			if i%2 == 0 {
				locale = ZhCN
				want = cause.Error()
			}
			got, ok := FormatError(locale, err)
			if !ok || got.Text != want || got.Params["field"] != "project_id" {
				t.Errorf("mixed locale or mutable parameters: %#v", got)
			}
			got.Params["field"] = "changed"
		}(i)
	}
	wg.Wait()
	for _, params := range []Params{{"field": ""}, {"field": "a\nb"}, {"field": string(make([]byte, 513))}, {"field": "x", "extra": "secret"}, {"field": 1}} {
		if got, ok := FormatError(EnUS, AnnotateError(cause, "validation.field_required", params)); ok || got.Params != nil {
			t.Fatal("invalid parameters exposed", got)
		}
	}
	changed := &changingDiagnostic{text: "输入无效"}
	annotated := AnnotateError(changed, "common.invalid_input", nil)
	changed.text = "changed source"
	if _, ok := FormatError(EnUS, annotated); ok {
		t.Fatal("mutated cause translated")
	}
}

func TestCanonicalTemplateGrammarAndCatalogSnapshot(t *testing.T) {
	for _, source := range []string{"{{printf .field}}", "{{.unknown}}", "{{if .field}}x{{end}}"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("accepted executable or undeclared template", source)
				}
			}()
			compileTemplate(Definition{Key: "test.template", Parameters: []Parameter{{Name: "field", Kind: Identifier}}}, source)
		}()
	}
	for _, d := range Catalog() {
		if d.SourceTemplate != "" {
			before := definitions[d.Key].SourceTemplate
			d.SourceTemplate = "changed"
			d.Texts[EnUS] = "changed"
			if definitions[d.Key].SourceTemplate != before || definitions[d.Key].Texts[EnUS] == "changed" {
				t.Fatal("catalog mutation")
			}
			break
		}
	}
}
