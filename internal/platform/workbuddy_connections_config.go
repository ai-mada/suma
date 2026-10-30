package platform

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"github.com/ai-mada/suma/internal/workbuddy"
)

func NewWorkBuddyConnections(s *Service, cfg workbuddy.OAuthConfig, key []byte) (*WorkBuddyConnections, error) {
	if s == nil {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	w := &WorkBuddyConnections{s: s}
	if cfg.BaseURL == "" && cfg.ClientID == "" && cfg.ClientSecret == "" && cfg.RedirectURI == "" && len(cfg.Scopes) == 0 && cfg.HTTPClient == nil {
		return w, nil
	}
	client, err := workbuddy.NewOAuthClient(cfg)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	authURL, err := client.AuthorizationURL("configuration-validation")
	if err != nil {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	cfg.BaseURL = u.Scheme + "://" + u.Host
	cfg.Scopes = append([]string(nil), cfg.Scopes...)
	sort.Strings(cfg.Scopes)
	if cfg.HTTPClient != nil {
		clone := *cfg.HTTPClient
		cfg.HTTPClient = &clone
	}
	w.app = cfg
	w.key = append([]byte(nil), key...)
	w.configured = true
	w.appDigest = digest(stringify([]string{"workbuddy-application-v1", cfg.ClientID, cfg.BaseURL, cfg.RedirectURI}))
	return w, nil
}

// Only explicitly configured task scopes are available to this application.
func (w *WorkBuddyConnections) oauth(scopes []string) (*workbuddy.OAuthClient, error) {
	if w == nil || !w.configured || len(w.key) != 32 {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	canonical, err := workBuddyCanonicalScopes(scopes, false)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, scope := range w.app.Scopes {
		allowed[scope] = true
	}
	for _, scope := range canonical {
		if !allowed[scope] {
			return nil, fmt.Errorf("%s", workBuddyInvalid)
		}
	}
	cfg := w.app
	cfg.Scopes = canonical
	return workbuddy.NewOAuthClient(cfg)
}

func workBuddyCanonicalScopes(scopes []string, empty bool) ([]string, error) {
	if len(scopes) > 2 || !empty && len(scopes) == 0 {
		return nil, fmt.Errorf("%s", workBuddyInvalid)
	}
	out := append([]string{}, scopes...)
	seen := map[string]bool{}
	for _, scope := range out {
		if (scope != "user.task.readable" && scope != "user.task.invokable") || seen[scope] {
			return nil, fmt.Errorf("%s", workBuddyInvalid)
		}
		seen[scope] = true
	}
	sort.Strings(out)
	return out, nil
}

// Shared databases require the same explicit administrator-managed key. A
// partial existing local file is never replaced or silently repaired.
func LoadWorkBuddyConnectionKey(dataDir, dialect string, getenv func(string) string) ([]byte, error) {
	if getenv == nil {
		return nil, fmt.Errorf("WorkBuddy key configuration unavailable")
	}
	if value := getenv("SUMA_WORKBUDDY_MASTER_KEY"); value != "" {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != value {
			return nil, fmt.Errorf("SUMA_WORKBUDDY_MASTER_KEY must encode exactly 32 bytes")
		}
		return key, nil
	}
	if dialect != "sqlite" {
		return nil, fmt.Errorf("SUMA_WORKBUDDY_MASTER_KEY is required for shared databases")
	}
	if dataDir == "" {
		return nil, fmt.Errorf("WorkBuddy encryption requires a persistent data directory")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("WorkBuddy key directory unavailable")
	}
	path := filepath.Join(dataDir, "workbuddy-connections.key")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return readWorkBuddyConnectionKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("WorkBuddy key creation failed")
	}
	key := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, key); err == nil {
		_, err = f.Write(key)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("WorkBuddy key persistence failed")
	}
	return key, nil
}
func readWorkBuddyConnectionKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 32 {
		return nil, fmt.Errorf("existing WorkBuddy key must be a regular 32-byte file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("existing WorkBuddy key unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("WorkBuddy key changed while opening")
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("existing WorkBuddy key is incomplete")
	}
	return key, nil
}
