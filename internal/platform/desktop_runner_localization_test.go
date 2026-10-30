package platform

import (
	"net/http/httptest"
	"testing"

	"github.com/ai-mada/suma/internal/localization"
)

func TestDesktopRunnerDiagnosticsAreBilingualWithoutChangingCodes(t *testing.T) {
	for _, locale := range []localization.Locale{localization.ZhCN, localization.EnUS} {
		for _, code := range []string{"invalid", "unavailable", "proof_invalid", "proof_expired", "repository_unavailable", "repository_unsafe", "head_unavailable", "repository_changed", "context_changed", "worker_exists"} {
			w := httptest.NewRecorder()
			w.Header().Set("Content-Language", string(locale))
			err := desktopRunnerError(409, code).(*APIError)
			message := localizedAPIError(w, err, err)
			if message["localized"] != true || message["code"] != "desktop_runner_"+code || message["message_key"] != "desktop_runner."+code || message["message"] == err.Message {
				t.Fatalf("untranslated stable diagnostic %s/%s: %v", locale, code, message)
			}
		}
	}
}
