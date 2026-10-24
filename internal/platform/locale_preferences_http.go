package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/ai-mada/suma/internal/localization"
)

var preferenceVersionLexeme = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

func decodeLocalePreference(r *http.Request) (LocalePreferenceUpdateRequest, error) {
	var out LocalePreferenceUpdateRequest
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return out, invalid("语言偏好请求无法读取或超过大小限制")
	}
	if err = RejectDuplicateReleaseJSON(raw); err != nil {
		return out, invalid("语言偏好请求必须是无重复字段的 JSON 对象")
	}
	var fields map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(raw))
	if err = d.Decode(&fields); err != nil || fields == nil {
		return out, invalid("语言偏好请求必须是无重复字段的 JSON 对象")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return out, invalid("语言偏好请求必须是无重复字段的 JSON 对象")
	}
	if len(fields) != 3 || fields["locale"] == nil || fields["expected_version"] == nil || fields["idempotency_key"] == nil {
		return out, invalid("语言偏好请求必须且仅能包含 locale、expected_version 和 idempotency_key")
	}
	if bytes.Equal(bytes.TrimSpace(fields["locale"]), []byte("null")) || json.Unmarshal(fields["locale"], &out.Locale) != nil {
		return out, invalid("语言偏好的 locale 必须为字符串，空字符串表示继承")
	}
	version := string(fields["expected_version"])
	if !preferenceVersionLexeme.MatchString(version) {
		return out, invalid("语言偏好的 expected_version 必须为 0–2147483646 的整数")
	}
	v, err := strconv.ParseInt(version, 10, 32)
	if err != nil || v > 2147483646 {
		return out, invalid("语言偏好的 expected_version 必须为 0–2147483646 的整数")
	}
	out.ExpectedVersion = int(v)
	if bytes.Equal(bytes.TrimSpace(fields["idempotency_key"]), []byte("null")) || json.Unmarshal(fields["idempotency_key"], &out.IdempotencyKey) != nil {
		return out, invalid("语言偏好需要 8–128 字节且不含控制字符的 idempotency_key")
	}
	_, err = validateLocalePreference(out)
	return out, err
}

func (s *Server) localePreferencesHTTP(w http.ResponseWriter, r *http.Request, p Principal, scope, path string) bool {
	if path != "account/preferences" && path != "organization/preferences" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != "GET" && r.Method != "PATCH" {
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, &APIError{405, "method_not_allowed", "语言偏好接口仅支持 GET 和 PATCH", nil})
		return true
	}
	var out any
	var err error
	if r.Method == "GET" {
		if path == "account/preferences" {
			out, err = s.Service.AccountPreferences(r.Context(), p, tokenOf(r))
		} else {
			out, err = s.Service.OrganizationPreferences(r.Context(), p, tokenOf(r))
		}
	} else {
		var request LocalePreferenceUpdateRequest
		request, err = decodeLocalePreference(r)
		if err == nil {
			organization := path == "organization/preferences"
			expected := localePreferenceExpectation{users: r.Header.Values("X-Suma-Expected-User"), organizations: r.Header.Values("X-Suma-Expected-Organization")}
			var record localePreferenceState
			record, err = s.Service.updateLocalePreference(r.Context(), p, tokenOf(r), request, organization, expected)
			if organization {
				out = OrganizationPreferences{p.TenantID, record.Locale, record.Version, record.UpdatedAt}
			} else {
				out = AccountPreferences{p.ID, record.Locale, record.Version, record.UpdatedAt}
			}
		}
	}
	if err != nil {
		writeError(w, err)
	} else {
		writeJSON(w, 200, out)
	}
	return true
}

// Resolve persisted presentation defaults only for an authenticated browser
// identity. A valid explicit header avoids all preference reads. Failures leave
// the already negotiated header in place and cannot change business authority.
func (s *Server) applyLocalePreferences(w http.ResponseWriter, r *http.Request, p Principal, token, scope string) {
	if scope != "" || p.internal {
		return
	}
	if _, ok := localization.Normalize(r.Header.Get("X-Suma-Language")); ok {
		return
	}
	var account, organization, expires string
	err := s.Service.DB.QueryRowContext(r.Context(), s.Service.q(`SELECT COALESCE(a.locale,''),COALESCE(o.locale,''),t.expires_at FROM suma_sessions t JOIN suma_users u ON u.id=t.user_id JOIN suma_org_members m ON m.user_id=u.id AND m.tenant_id=t.tenant_id LEFT JOIN suma_account_preferences a ON a.user_id=u.id LEFT JOIN suma_organization_preferences o ON o.tenant_id=m.tenant_id WHERE t.token_hash=? AND t.user_id=? AND t.tenant_id=? AND t.label='browser' AND NOT EXISTS(SELECT 1 FROM suma_token_scopes ts WHERE ts.token_hash=t.token_hash)`), digest(token), p.ID, p.TenantID).Scan(&account, &organization, &expires)
	if err != nil {
		return
	}
	// Authentication has already validated expiry; the session-response hooks
	// pass a newly created credential for the selected organization.
	selection := localization.Resolve(localization.Preferences{Explicit: r.Header.Get("X-Suma-Language"), User: account, Organization: organization, AcceptLanguage: r.Header.Get("Accept-Language")})
	w.Header().Set("Content-Language", string(selection.Locale))
}
