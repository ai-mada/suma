package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

type repositorySourceExport struct {
	Bundle        []byte
	BundleSHA256  string
	RequestSHA256 string
}

// The reconcile credential can read the registered pair. Access to Git bundle
// bytes still requires the independently configured export credential.
func fetchRepositorySourceExportBasis(ctx context.Context, cfg RepositoryProviderConfig, alias string) (basis ri.RepositorySourceExportBasis, err error) {
	if !releaseID(alias) || !slices.Contains(cfg.TargetAliases, alias) {
		return basis, forbidden()
	}
	if cfg.ReconcileToken == cfg.SourceExportToken || !repositoryProviderOrigin(cfg.Origin) {
		return basis, repositoryError(http.StatusServiceUnavailable, "provider", repositoryProvider)
	}
	cfg.Origin = strings.TrimRight(cfg.Origin, "/")
	if err = repositoryProviderHTTP(ctx, cfg, http.MethodGet, "/source-export-basis/"+url.PathEscape(alias), nil, &basis); err != nil {
		return ri.RepositorySourceExportBasis{}, err
	}
	if ri.ValidateRepositorySourceExportBasis(alias, basis) != nil {
		return ri.RepositorySourceExportBasis{}, repositoryConflict("target", repositoryTarget)
	}
	return basis, nil
}

// fetchRepositorySourceExport receives bytes for the exact closed request.
// Git object verification belongs to the isolated recipient. No provider
// headers or response body are exposed through public errors.
func fetchRepositorySourceExport(ctx context.Context, cfg RepositoryProviderConfig, q ri.RepositorySourceExportRequest) (out repositorySourceExport, err error) {
	credential := cfg.SourceExportToken
	if len(credential) < 32 || strings.TrimSpace(credential) != credential || strings.ContainsAny(credential, "\r\n") ||
		credential == cfg.ReconcileToken || credential == cfg.SourceReadToken || digest(credential) == cfg.TokenHash || !repositoryProviderOrigin(cfg.Origin) {
		return out, repositoryError(http.StatusServiceUnavailable, "provider", repositoryProvider)
	}
	if q.TenantID != cfg.TenantID || q.ProjectID != cfg.ProjectID || q.ProviderID != cfg.ID || !slices.Contains(cfg.TargetAliases, q.TargetAlias) {
		return out, forbidden()
	}
	requestSHA, err := ri.RepositorySourceExportDigest(q)
	if err != nil {
		return out, repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid)
	}
	request, err := json.Marshal(q)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.Origin, "/")+"/source-exports", bytes.NewReader(request))
	if err != nil {
		return out, repositoryError(http.StatusServiceUnavailable, "provider", repositoryProvider)
	}
	const bundleType = "application/x-git-bundle"
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", bundleType)
	transport := &http.Transport{Proxy: nil, DisableCompression: true}
	defer transport.CloseIdleConnections()
	// The provider can spend 30s in Git and 10s transmitting the bounded body.
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return out, repositoryError(http.StatusBadGateway, "provider", repositoryProvider)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		return out, repositoryConflict("target", repositoryTarget)
	case http.StatusTooManyRequests:
		return out, repositoryError(http.StatusTooManyRequests, "source_export_busy", "Repository source export is busy")
	case http.StatusServiceUnavailable:
		return out, repositoryError(http.StatusServiceUnavailable, "provider", repositoryProvider)
	default:
		return out, repositoryError(http.StatusBadGateway, "provider", repositoryProvider)
	}
	length := resp.ContentLength
	if length < 1 || length > cc.MaxBundleBytes || singleRepositoryExportHeader(resp.Header, "Content-Length") != strconv.FormatInt(length, 10) ||
		singleRepositoryExportHeader(resp.Header, "Content-Type") != bundleType || len(resp.Header.Values("Content-Encoding")) != 0 ||
		singleRepositoryExportHeader(resp.Header, "X-Suma-Source-Request-SHA256") != requestSHA {
		return out, repositoryError(http.StatusBadGateway, "source_export_invalid", "Repository source export does not match the fixed request")
	}
	bundleSHA := singleRepositoryExportHeader(resp.Header, "X-Suma-Bundle-SHA256")
	if !cc.ValidHash(bundleSHA) {
		return out, repositoryError(http.StatusBadGateway, "source_export_invalid", "Repository source export does not match the fixed request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, length+1))
	if err != nil || int64(len(data)) != length {
		return out, repositoryError(http.StatusBadGateway, "source_export_invalid", "Repository source export does not match the fixed request")
	}
	sha := sha256.Sum256(data)
	if hex.EncodeToString(sha[:]) != bundleSHA {
		return out, repositoryError(http.StatusBadGateway, "source_export_invalid", "Repository source export does not match the fixed request")
	}
	return repositorySourceExport{Bundle: data, BundleSHA256: bundleSHA, RequestSHA256: requestSHA}, nil
}

func singleRepositoryExportHeader(headers http.Header, key string) string {
	values := headers.Values(key)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}
