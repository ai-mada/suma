package workbuddy

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

// oauthRoundTrip limits one SDK exchange to one exact POST and validates the
// wire response before the SDK's permissive decoder or error formatter sees it.
// A fresh boundary is allocated per call, never shared by simultaneous grants.
type oauthRoundTrip struct {
	base            http.RoundTripper
	endpoint        string
	scopes          []string
	refresh         bool
	startedAt       time.Time
	sent, validated bool
	grant           TokenGrant
	failure         *Error
}

func (b *oauthRoundTrip) fail(code string, status int) (*http.Response, error) {
	b.failure = &Error{Code: code, StatusCode: status, OutcomeUnknown: b.sent}
	return nil, b.failure
}
func (b *oauthRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	if b.sent || req.Method != http.MethodPost || req.URL.String() != b.endpoint {
		return b.fail("invalid_request", 0)
	}
	if req.Context().Err() != nil {
		return b.fail("cancelled", 0)
	}
	request := req.Clone(req.Context())
	request.GetBody = nil // never make the grant exchange replayable by a transport
	request.Header.Del("Authorization")
	request.Header.Del("Cookie")
	request.Header.Del("Idempotency-Key")
	request.Header.Del("X-Idempotency-Key")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	b.sent = true
	response, err := b.base.RoundTrip(request)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return b.fail("transport", 0)
	}
	if response == nil || response.Body == nil {
		return b.fail("invalid_response", 0)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return b.fail("http_status", response.StatusCode)
	}
	content, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || content != "application/json" {
		return b.fail("invalid_response", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return b.fail("invalid_response", response.StatusCode)
	}
	grant, expires, err := decodeOAuthGrant(raw, b.scopes, b.refresh, b.startedAt)
	if err != nil {
		return b.fail("invalid_response", response.StatusCode)
	}
	// Re-encode only checked, exact-case fields. Unknown or differently-cased
	// fields must not override validated values in the SDK's case-insensitive JSON
	// decoder, and vendor diagnostics must never reach oauth2.RetrieveError.
	safe := map[string]any{"access_token": grant.AccessToken, "token_type": "Bearer", "expires_in": expires}
	if grant.RefreshTokenPresent {
		safe["refresh_token"] = grant.RefreshToken
	}
	body, err := json.Marshal(safe)
	if err != nil {
		return b.fail("invalid_response", response.StatusCode)
	}
	copyResponse := *response
	copyResponse.Header = response.Header.Clone()
	copyResponse.Header.Set("Content-Type", "application/json")
	copyResponse.Header.Del("Content-Length")
	copyResponse.ContentLength = int64(len(body))
	copyResponse.Body = io.NopCloser(bytes.NewReader(body))
	b.grant = grant
	b.validated = true
	return &copyResponse, nil
}
func decodeOAuthGrant(raw []byte, requested []string, refresh bool, started time.Time) (TokenGrant, int, error) {
	fail := func() (TokenGrant, int, error) { return TokenGrant{}, 0, localError("invalid_response") }
	fields, err := object(raw)
	if err != nil {
		return fail()
	}
	if _, present := fields["error"]; present {
		return fail()
	}
	access, err := stringField(fields, "access_token", true, 8192)
	if err != nil || !validToken(access) || strings.TrimRight(access, "=") == "" {
		return fail()
	}
	tokenType, err := stringField(fields, "token_type", true, 32)
	if err != nil || !strings.EqualFold(tokenType, "Bearer") {
		return fail()
	}
	id, err := stringField(fields, "open_id", true, 512)
	if err != nil || !validOAuthOpaque(id, 512) {
		return fail()
	}
	scope, err := stringField(fields, "scope", true, 128)
	if err != nil {
		return fail()
	}
	scopes := strings.Split(scope, " ")
	if !validOAuthScopes(scopes) {
		return fail()
	}
	allowed := map[string]bool{}
	for _, s := range requested {
		allowed[s] = true
	}
	for _, s := range scopes {
		if !allowed[s] {
			return fail()
		}
	}
	expires, err := integerField(fields, "expires_in")
	// The SDK represents expiration as int32 seconds. Reject values it would
	// silently clamp; this is a local representation bound, not a vendor quota.
	if err != nil || expires < 1 || int64(expires) > 2147483647 {
		return fail()
	}
	_, present := fields["refresh_token"]
	if !refresh && !present {
		return fail()
	}
	next := ""
	if present {
		next, err = stringField(fields, "refresh_token", true, 8192)
		if err != nil || !validOAuthOpaque(next, 8192) {
			return fail()
		}
	}
	return TokenGrant{AccessToken: access, RefreshToken: next, RefreshTokenPresent: present, OpenID: id, Scopes: scopes, ExpiresAt: started.Add(time.Duration(expires) * time.Second)}, expires, nil
}
