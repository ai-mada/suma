package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/workbuddy"
)

// configureWorkBuddyConnections binds an operator-owned application to this
// control plane's fixed callback. User grants still require an explicit browser
// authorization and never become task-dispatch authority by being configured.
// Keeping configuration here also lets desktop and server startup share it.
func configureWorkBuddyConnections(service *platform.Service, publicURL string, getenv func(string) string) (*platform.WorkBuddyConnections, error) {
	if service == nil || getenv == nil {
		return nil, fmt.Errorf("WorkBuddy application configuration unavailable")
	}
	clientID, secret := getenv("SUMA_WORKBUDDY_CLIENT_ID"), getenv("SUMA_WORKBUDDY_CLIENT_SECRET")
	scopesValue := getenv("SUMA_WORKBUDDY_SCOPES")
	if clientID == "" && secret == "" && scopesValue == "" {
		// Disabled installations do not create an unnecessary local key file or
		// demand a shared-database key for an unconfigured provider.
		return platform.NewWorkBuddyConnections(service, workbuddy.OAuthConfig{}, nil)
	}
	origin, err := url.Parse(publicURL)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.RawPath != "" || origin.Path != "" && origin.Path != "/" {
		return nil, fmt.Errorf("WorkBuddy requires a configured public callback origin")
	}
	scopes := strings.FieldsFunc(scopesValue, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	cfg := workbuddy.OAuthConfig{ClientID: clientID, ClientSecret: secret,
		RedirectURI: strings.TrimSuffix(publicURL, "/") + "/api/v1/workbuddy-oauth/callback", Scopes: scopes}
	// Validate before creating any credential file. Protocol validation reports
	// fixed safe diagnostics rather than echoing secret configuration values.
	if _, err := workbuddy.NewOAuthClient(cfg); err != nil {
		return nil, fmt.Errorf("WorkBuddy application configuration invalid")
	}
	key, err := platform.LoadWorkBuddyConnectionKey(service.DataDir, service.Dialect, getenv)
	if err != nil {
		return nil, err
	}
	return platform.NewWorkBuddyConnections(service, cfg, key)
}
