package workbuddy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// OAuthConfig describes one confidential application, its exact registered
// callback, and the explicit task scopes it may request. It does not authorize
// any Suma project or prove provider support for PKCE.
type OAuthConfig struct {
	BaseURL      string       `json:"base_url"`
	ClientID     string       `json:"client_id"`
	ClientSecret string       `json:"-"`
	RedirectURI  string       `json:"redirect_uri"`
	Scopes       []string     `json:"scopes"`
	HTTPClient   *http.Client `json:"-"`
}

// TokenGrant is a protocol result, not permission to persist or use a grant.
// The domain must recheck current authorization, scope and expected OpenID.
// RefreshTokenPresent preserves wire absence even though oauth2 fills in an
// old refresh token when the response omits it.
type TokenGrant struct {
	AccessToken         string    `json:"-"`
	RefreshToken        string    `json:"-"`
	OpenID              string    `json:"open_id"`
	Scopes              []string  `json:"scopes"`
	ExpiresAt           time.Time `json:"expires_at"`
	RefreshTokenPresent bool      `json:"refresh_token_present"`
}

type OAuthClient struct {
	config    oauth2.Config
	http      http.Client
	transport http.RoundTripper
}

func (OAuthConfig) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddy.OAuthConfig{credentials:redacted}")
}
func (OAuthClient) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddy.OAuthClient{credentials:redacted}")
}
func (TokenGrant) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddy.TokenGrant{credentials:redacted}")
}

func NewOAuthClient(cfg OAuthConfig) (*OAuthClient, error) {
	origin, err := canonicalOrigin(cfg.BaseURL)
	if err != nil || !validOAuthOpaque(cfg.ClientID, 512) || !validOAuthOpaque(cfg.ClientSecret, 8192) || !validOAuthCallback(cfg.RedirectURI) || !validOAuthScopes(cfg.Scopes) {
		return nil, localError("invalid_config")
	}
	var client http.Client
	if cfg.HTTPClient != nil {
		client = *cfg.HTTPClient
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout <= 0 {
		client.Timeout = 30 * time.Second
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	// AuthStyle must be fixed: SDK discovery retries a failed token request with
	// another authentication style, which is unsafe for one-use grant exchanges.
	config := oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURI,
		Scopes:   append([]string(nil), cfg.Scopes...),
		Endpoint: oauth2.Endpoint{AuthURL: origin + "/openapi/v2/authorize", TokenURL: origin + "/openapi/v2/token", AuthStyle: oauth2.AuthStyleInParams},
	}
	return &OAuthClient{config: config, http: client, transport: transport}, nil
}

// AuthorizationURL requires caller-generated, unpredictable state. The domain
// owns its single-use, session/project binding and expiry; this client cannot
// verify them. No undocumented PKCE parameters or fallback are advertised.
func (c *OAuthClient) AuthorizationURL(state string) (string, error) {
	if c == nil || c.config.Endpoint.AuthURL == "" {
		return "", localError("invalid_config")
	}
	if !validOAuthOpaque(state, 512) {
		return "", localError("invalid_request")
	}
	return c.config.AuthCodeURL(state), nil
}
func (c *OAuthClient) ExchangeCode(ctx context.Context, code string) (TokenGrant, error) {
	return c.exchange(ctx, code, false)
}

// Refresh makes one explicit exchange. The caller must hold its own persistent
// refresh permit and publish rotated credentials with current authority/CAS.
// This client installs no long-lived automatic refresh transport or retry loop.
func (c *OAuthClient) Refresh(ctx context.Context, refreshToken string) (TokenGrant, error) {
	return c.exchange(ctx, refreshToken, true)
}
func (c *OAuthClient) exchange(ctx context.Context, value string, refresh bool) (TokenGrant, error) {
	if c == nil || c.config.Endpoint.TokenURL == "" {
		return TokenGrant{}, localError("invalid_config")
	}
	if ctx == nil || !validOAuthOpaque(value, 8192) {
		return TokenGrant{}, localError("invalid_request")
	}
	if ctx.Err() != nil {
		return TokenGrant{}, localError("cancelled")
	}
	// Every invocation owns its response metadata and network boundary. Sharing
	// SDK TokenSource or response state here could mix concurrent user grants.
	boundary := &oauthRoundTrip{base: c.transport, endpoint: c.config.Endpoint.TokenURL, scopes: c.config.Scopes, refresh: refresh, startedAt: time.Now()}
	client := c.http
	client.Transport = boundary
	requestContext := context.WithValue(ctx, oauth2.HTTPClient, &client)
	var token *oauth2.Token
	var err error
	if refresh {
		// A fresh, already-expired token source performs exactly one explicit refresh
		// and is discarded. Its inherited RefreshToken must not escape the boundary.
		old := &oauth2.Token{RefreshToken: value, Expiry: time.Unix(1, 0)}
		token, err = c.config.TokenSource(requestContext, old).Token()
	} else {
		token, err = c.config.Exchange(requestContext, value)
	}
	if boundary.failure != nil {
		return TokenGrant{}, boundary.failure
	}
	if err != nil || token == nil || !boundary.validated {
		code := "invalid_response"
		if !boundary.sent {
			code = "cancelled"
		}
		return TokenGrant{}, &Error{Code: code, OutcomeUnknown: boundary.sent}
	}
	// Do not expose SDK raw fields or errors, both of which can retain arbitrary
	// server content and secrets. The whitelist was validated before SDK parsing.
	grant := boundary.grant
	if token.AccessToken != grant.AccessToken || token.TokenType != "Bearer" {
		return TokenGrant{}, &Error{Code: "invalid_response", OutcomeUnknown: true}
	}
	return grant, nil
}
func validOAuthOpaque(value string, max int) bool {
	if len(value) == 0 || len(value) > max {
		return false
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func validOAuthScopes(scopes []string) bool {
	if len(scopes) == 0 || len(scopes) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if scope != "user.task.readable" && scope != "user.task.invokable" || seen[scope] {
			return false
		}
		seen[scope] = true
	}
	return true
}
func validOAuthCallback(raw string) bool {
	if len(raw) == 0 || len(raw) > 2048 || !validText(raw, 2048, false) || strings.TrimSpace(raw) != raw {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" || u.Hostname() == "" || u.Fragment != "" || strings.Contains(raw, "#") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	// This is an explicit test/local callback exception, not evidence that the
	// provider accepts dynamic loopback callbacks for confidential applications.
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && ip != nil && ip.IsLoopback()
}
