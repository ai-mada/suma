package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	ic "github.com/ai-mada/suma/internal/indexcontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

// The private index is a recall service, never an authorization authority. This
// client validates the closed wire only; business callers re-read source data
// and current authorization after this network boundary.
func (s *Service) queryTypedIndex(ctx context.Context, request ic.SearchRequest) (ic.SearchResult, error) {
	var result ic.SearchResult
	if err := ic.ValidateSearchRequest(request); err != nil {
		return result, invalid("查询参数或来源范围无效")
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return result, err
	}
	status, body, err := callTypedIndex(ctx, "search", raw, 2<<20)
	if err != nil && status == http.StatusServiceUnavailable {
		if phase := typedIndexPhaseError(body); phase != nil {
			return result, phase
		}
	}
	if err != nil || strictjson.Decode(body, &result, 2<<20) != nil || ic.ValidateSearchResult(result) != nil || len(result.Items) > request.Limit {
		return ic.SearchResult{}, typedIndexUnavailable()
	}
	seen := map[string]bool{}
	for _, hit := range result.Items {
		if hit.Source.Kind != request.SourceKind {
			return ic.SearchResult{}, typedIndexUnavailable()
		}
		key := stringify(hit)
		if seen[key] {
			return ic.SearchResult{}, typedIndexUnavailable()
		}
		seen[key] = true
	}
	return result, nil
}

func typedIndexPhaseError(body []byte) error {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	message := ""
	switch envelope.Error.Code {
	case "index_rebuild_required":
		message = "索引需要重建；关键词发现仍可使用"
	case "index_rebuilding":
		message = "索引正在重建；关键词发现仍可使用"
	case "index_blocked":
		message = "索引重建已阻塞；请联系管理员"
	case "index_busy":
		message = "检索请求较多，请稍后重试"
	default:
		return nil
	}
	return &APIError{503, envelope.Error.Code, message, nil}
}

func typedIndexUnavailable() error {
	return &APIError{503, "index_unavailable", "检索服务暂不可用；资料原件与关键词发现仍可访问", nil}
}

func typedIndexEndpoint() (string, string, error) {
	endpoint := strings.TrimRight(os.Getenv("SUMA_KNOWLEDGE_URL"), "/")
	token := os.Getenv("SUMA_KNOWLEDGE_TOKEN")
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || len(endpoint) > 2048 || strings.TrimSpace(endpoint) != endpoint || token == "" || strings.ContainsAny(token, "\r\n") {
		return "", "", fmt.Errorf("typed index configuration is unavailable")
	}
	allowedHTTP, err := typedIndexHTTPOriginAllowed(u)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || allowedHTTP)) {
		return "", "", fmt.Errorf("typed index endpoint requires HTTPS, loopback or an explicitly allowed HTTP origin")
	}
	return endpoint, token, nil
}

// Explicit HTTP destinations are an operator decision for a private service
// network. Match only scheme+Host: no suffix matching, wildcard, userinfo or
// URL-prefix grant can accidentally forward the internal bearer credential.
func typedIndexHTTPOriginAllowed(endpoint *url.URL) (bool, error) {
	raw := os.Getenv("SUMA_KNOWLEDGE_HTTP_ORIGINS")
	if raw == "" {
		return false, nil
	}
	invalidOrigins := func() (bool, error) {
		return false, fmt.Errorf("typed index HTTP origins require exact credential-free HTTP origins")
	}
	if len(raw) > 8192 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return invalidOrigins()
	}
	origins := strings.Split(raw, ",")
	if len(origins) > 32 {
		return invalidOrigins()
	}
	allowed := false
	for _, value := range origins {
		origin := strings.TrimSpace(value)
		u, err := url.Parse(origin)
		if err != nil || u == nil || u.Scheme != "http" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.Contains(u.Host, "*") || origin != u.Scheme+"://"+u.Host {
			return invalidOrigins()
		}
		if endpoint.Scheme == u.Scheme && endpoint.Host == u.Host {
			allowed = true
		}
	}
	return allowed, nil
}

// Return the actual provider status separately so only permanent rejections
// may dead-letter an event. Transport errors and invalid ACKs retain its intent.
func callTypedIndex(ctx context.Context, path string, raw []byte, maximum int64) (int, []byte, error) {
	if path != "index" && path != "search" {
		return 0, nil, fmt.Errorf("invalid internal index route")
	}
	endpoint, token, err := typedIndexEndpoint()
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/"+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil || int64(len(body)) > maximum {
		return response.StatusCode, nil, fmt.Errorf("typed index response exceeds its bounded contract")
	}
	if response.StatusCode/100 != 2 {
		return response.StatusCode, body, fmt.Errorf("typed index provider rejected request: HTTP %d", response.StatusCode)
	}
	return response.StatusCode, body, nil
}

func (s *Service) postTypedIndex(ctx context.Context, request ic.Request, raw []byte) (ic.Receipt, int, error) {
	var receipt ic.Receipt
	if ic.ValidateRequest(request) != nil {
		return receipt, 0, fmt.Errorf("invalid persisted typed index request")
	}
	status, body, err := callTypedIndex(ctx, "index", raw, 64<<10)
	if err != nil {
		return receipt, status, err
	}
	if strictjson.Decode(body, &receipt, 64<<10) != nil || !ic.ReceiptMatches(receipt, request, raw) {
		return ic.Receipt{}, 0, fmt.Errorf("original typed index persistence was not acknowledged")
	}
	return receipt, status, nil
}
