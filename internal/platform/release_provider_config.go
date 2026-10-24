package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

// LoadReleaseProviders reads administrator-owned references to process secrets.
// Only hashes and fixed audiences become control-plane provider configuration.
// A wrong or incomplete configuration fails startup, never downgrades auth.
func LoadReleaseProviders(raw string, getenv func(string) string) ([]ReleaseProviderConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if len(raw) > 128<<10 {
		return nil, fmt.Errorf("release provider configuration exceeds 128 KiB")
	}
	if err := RejectDuplicateReleaseJSON([]byte(raw)); err != nil {
		return nil, fmt.Errorf("release provider configuration JSON is invalid")
	}
	var entries []struct {
		ID                string   `json:"id"`
		TenantID          string   `json:"tenant_id"`
		ProjectID         string   `json:"project_id"`
		SiteIDs           []string `json:"site_ids"`
		TokenEnv          string   `json:"token_env"`
		Origin            string   `json:"origin"`
		ReconcileTokenEnv string   `json:"reconcile_token_env"`
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&entries); err != nil || len(entries) > 32 {
		return nil, fmt.Errorf("release provider configuration has unknown fields or exceeds 32 entries")
	}
	out := []ReleaseProviderConfig{}
	seen := map[string]bool{}
	secretAudiences := map[string]bool{}
	for _, e := range entries {
		if !rc.ValidProfileID(e.ID) || !rc.ValidProfileID(e.TenantID) || !rc.ValidProfileID(e.ProjectID) || len(e.SiteIDs) == 0 || len(e.SiteIDs) > 32 {
			return nil, fmt.Errorf("release provider requires valid IDs and 1–32 site IDs")
		}
		key := e.TenantID + "/" + e.ProjectID + "/" + e.ID
		if seen[key] {
			return nil, fmt.Errorf("duplicate release provider audience")
		}
		seen[key] = true
		sites := map[string]bool{}
		for _, id := range e.SiteIDs {
			if !rc.ValidProfileID(id) || sites[id] {
				return nil, fmt.Errorf("release provider site IDs must be unique portable names")
			}
			sites[id] = true
		}
		token := getenv(e.TokenEnv)
		if e.TokenEnv == "" || len(token) < 32 {
			return nil, fmt.Errorf("release provider %s requires a process secret of at least 32 bytes", e.ID)
		}
		cfg := ReleaseProviderConfig{ID: e.ID, TenantID: e.TenantID, ProjectID: e.ProjectID, SiteIDs: e.SiteIDs, TokenHash: digest(token)}
		if secretAudiences[cfg.TokenHash] {
			return nil, fmt.Errorf("release providers require a distinct secret for each fixed audience")
		}
		secretAudiences[cfg.TokenHash] = true
		if e.Origin != "" || e.ReconcileTokenEnv != "" {
			u, err := url.Parse(e.Origin)
			if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
				return nil, fmt.Errorf("release provider %s requires a fixed HTTP literal loopback origin for local static deployment", e.ID)
			}
			reconcile := getenv(e.ReconcileTokenEnv)
			if len(reconcile) < 32 || reconcile == token {
				return nil, fmt.Errorf("release provider %s requires a separate read-only reconciliation secret", e.ID)
			}
			cfg.Origin = strings.TrimRight(e.Origin, "/")
			cfg.ReconcileToken = reconcile
		}
		out = append(out, cfg)
	}
	return out, nil
}
