package platform

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func LoadRepositoryProviders(raw string, getenv func(string) string) ([]RepositoryProviderConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if len(raw) > 128<<10 || RejectDuplicateReleaseJSON([]byte(raw)) != nil {
		return nil, fmt.Errorf("invalid repository provider configuration")
	}
	var entries []struct {
		ID                string   `json:"id"`
		TenantID          string   `json:"tenant_id"`
		ProjectID         string   `json:"project_id"`
		TargetAliases     []string `json:"target_aliases"`
		Origin            string   `json:"origin"`
		TokenEnv          string   `json:"token_env"`
		ReconcileTokenEnv string   `json:"reconcile_token_env"`
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&entries) != nil || len(entries) > 32 {
		return nil, fmt.Errorf("repository provider configuration requires at most 32 strict entries")
	}
	out := []RepositoryProviderConfig{}
	audiences, secrets := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		if !releaseID(e.ID) || !releaseID(e.TenantID) || !releaseID(e.ProjectID) || len(e.TargetAliases) < 1 || len(e.TargetAliases) > 32 {
			return nil, fmt.Errorf("repository provider requires valid audience and target aliases")
		}
		audience := e.TenantID + "/" + e.ProjectID + "/" + e.ID
		if audiences[audience] {
			return nil, fmt.Errorf("duplicate repository provider audience")
		}
		audiences[audience] = true
		u, err := url.Parse(e.Origin)
		if err != nil || u.Scheme != "http" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
			return nil, fmt.Errorf("repository provider requires a fixed literal loopback HTTP origin")
		}
		token, reconcile := getenv(e.TokenEnv), getenv(e.ReconcileTokenEnv)
		if e.TokenEnv == "" || e.ReconcileTokenEnv == "" || len(token) < 32 || len(reconcile) < 32 || token == reconcile || secrets[digest(token)] || secrets[digest(reconcile)] {
			return nil, fmt.Errorf("repository provider requires distinct process secrets of at least 32 bytes")
		}
		secrets[digest(token)], secrets[digest(reconcile)] = true, true
		aliases := map[string]bool{}
		for _, alias := range e.TargetAliases {
			if !releaseID(alias) || aliases[alias] {
				return nil, fmt.Errorf("repository target aliases must be unique portable names")
			}
			aliases[alias] = true
		}
		out = append(out, RepositoryProviderConfig{ID: e.ID, TenantID: e.TenantID, ProjectID: e.ProjectID, TargetAliases: append([]string{}, e.TargetAliases...), TokenHash: digest(token), Origin: strings.TrimRight(e.Origin, "/"), ReconcileToken: reconcile})
	}
	return out, nil
}

func (s *Service) repositoryProvider(tenant, project, id, alias string) (RepositoryProviderConfig, error) {
	for _, cfg := range s.RepositoryProviders {
		if cfg.TenantID == tenant && cfg.ProjectID == project && cfg.ID == id && contentDigest.MatchString(cfg.TokenHash) && cfg.Origin != "" && len(cfg.ReconcileToken) >= 32 {
			if alias == "" {
				return cfg, nil
			}
			for _, a := range cfg.TargetAliases {
				if alias == a {
					return cfg, nil
				}
			}
		}
	}
	return RepositoryProviderConfig{}, repositoryError(503, "provider", repositoryProvider)
}
func (s *Service) authenticateRepositoryProvider(token string) (RepositoryProviderConfig, error) {
	if len(token) >= 32 {
		hash := digest(token)
		for _, cfg := range s.RepositoryProviders {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(cfg.TokenHash)) == 1 {
				return cfg, nil
			}
		}
	}
	return RepositoryProviderConfig{}, forbidden()
}

// All network reads happen between authority transactions, against a fixed
// administrator origin; redirects and user-supplied URLs are never followed.
func repositoryProviderHTTP(ctx context.Context, cfg RepositoryProviderConfig, method, path string, request, out any) error {
	var body io.Reader
	if request != nil {
		raw, err := json.Marshal(request)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.Origin+path, body)
	if err != nil {
		return repositoryError(502, "provider", repositoryProvider)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.ReconcileToken)
	req.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return repositoryError(502, "provider", repositoryProvider)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return repositoryError(404, "operation_missing", repositoryUnknown)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || resp.StatusCode != 200 {
		return repositoryError(502, "provider", repositoryProvider)
	}
	if RejectDuplicateReleaseJSON(raw) != nil {
		return repositoryConflict("observation", repositoryObservation)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return repositoryConflict("observation", repositoryObservation)
	}
	return nil
}
func (s *Service) repositoryCurrentTargetProbe(ctx context.Context, tenant string, plan RepositoryIntegrationPlan) error {
	cfg, err := s.repositoryProvider(tenant, plan.ProjectID, plan.Target.ProviderID, plan.Target.TargetAlias)
	if err != nil {
		return err
	}
	observed, err := repositoryProbe(ctx, cfg, plan.Target.TargetAlias)
	if err != nil {
		return err
	}
	old := plan.Target.Observation
	if old == nil || observed.Ref != old.Ref || observed.ConfigSHA256 != old.ConfigSHA256 || observed.HeadCommit != old.HeadCommit || observed.FenceOID != old.FenceOID || observed.Generation != old.Generation {
		return repositoryConflict("target", repositoryTarget)
	}
	return nil
}
func repositoryProbe(ctx context.Context, cfg RepositoryProviderConfig, alias string) (out ri.RepositoryTargetObservation, err error) {
	err = repositoryProviderHTTP(ctx, cfg, "GET", "/targets/"+url.PathEscape(alias), nil, &out)
	if err == nil && (!out.Available || out.Alias != alias || !ri.RepositoryValidRef(out.Ref) || !contentDigest.MatchString(out.ConfigSHA256) || !gitRevision.MatchString(out.HeadCommit) || out.ObjectFormat != "sha1" && out.ObjectFormat != "sha256" || out.Generation < 0 || out.Generation >= 9007199254740990 || out.FenceOID != "" && !gitRevision.MatchString(out.FenceOID) || !validReleaseTime(out.ObservedAt)) {
		err = repositoryConflict("target", repositoryTarget)
	}
	return
}
