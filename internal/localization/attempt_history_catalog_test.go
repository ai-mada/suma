package localization

import (
	"errors"
	"testing"
)

func TestAttemptHistoryDiagnosticsKeepSourceAndRequireExactProof(t *testing.T) {
	for _, tc := range []struct{ key, source string }{
		{"attempt_history.query", "invalid: invalid attempt history query"},
		{"attempt_history.cursor", "invalid: invalid attempt history cursor"},
		{"attempt_history.identity", "invalid durable attempt identity"},
		{"attempt_history.event_size", "selected tool event exceeds 2 MiB"},
		{"attempt_history.receipt", "saved runner receipt does not match durable attempt identity"},
		{"attempt_history.raw_encoding", "selected tool event is not valid UTF-8"},
		{"attempt_history.http_query", "工具历史请求参数无效"},
	} {
		cause := errors.New(tc.source)
		annotated := AnnotateError(cause, tc.key, nil)
		for _, locale := range []Locale{ZhCN, EnUS} {
			got, ok := FormatError(locale, annotated)
			if !ok || !got.Localized || got.Key != tc.key || got.Text == "" {
				t.Fatalf("missing %s %s: %+v", locale, tc.key, got)
			}
			if !errors.Is(annotated, cause) || annotated.Error() != tc.source {
				t.Fatal("display changed original cause", tc.key)
			}
			if _, ok = FormatError(locale, AnnotateError(errors.New(tc.source+" arbitrary tool content"), tc.key, nil)); ok {
				t.Fatal("partial source falsely translated", tc.key)
			}
		}
	}
}
