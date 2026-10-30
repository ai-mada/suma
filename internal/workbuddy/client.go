package workbuddy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	endpoint string
	token    string
	http     *http.Client
}

func (Client) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddy.Client{credentials:redacted}")
}

func NewClient(cfg Config) (*Client, error) {
	origin, err := canonicalOrigin(cfg.BaseURL)
	if err != nil || !validToken(cfg.Token) {
		return nil, localError("invalid_config")
	}
	var client http.Client
	if cfg.HTTPClient != nil {
		client = *cfg.HTTPClient
	}
	// Cookies and redirect behavior from an unrelated caller's client must never
	// widen this credential's destination. The original client is not mutated.
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout <= 0 {
		client.Timeout = 30 * time.Second
	}
	return &Client{endpoint: origin + tasksPath, token: cfg.Token, http: &client}, nil
}
func canonicalOrigin(raw string) (string, error) {
	if raw == "" {
		return defaultOrigin, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || u.Path != "" && u.Path != "/" {
		return "", localError("invalid_config")
	}
	if u.Scheme == "https" && (u.Host == "www.workbuddy.cn" || u.Host == "www.workbuddy.cn:443") {
		return defaultOrigin, nil
	}
	// Only an explicitly supplied literal loopback origin is available for local
	// fixtures. No arbitrary hostname lookup or private-network exception exists.
	if u.Scheme != "http" || strings.HasSuffix(u.Host, ":") {
		return "", localError("invalid_config")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return "", localError("invalid_config")
	}
	port := u.Port()
	if port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return "", localError("invalid_config")
		}
	}
	if u.Host == "" {
		return "", localError("invalid_config")
	}
	host := ip.String()
	if port != "" && port != "80" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return "http://" + host, nil
}
func validToken(token string) bool {
	if len(token) == 0 || len(token) > 8192 || token[0] == '=' {
		return false
	}
	padding := false
	for _, r := range token {
		if r == '=' {
			padding = true
			continue
		}
		if padding || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~+/", r)) {
			return false
		}
	}
	return true
}
func (c *Client) Endpoint() string {
	if c == nil {
		return ""
	}
	return c.endpoint
}

func (c *Client) CreateTask(ctx context.Context, request CreateRequest) (Task, error) {
	raw, err := encodeCreateRequest(request)
	if err != nil {
		return Task{}, err
	}
	response, err := c.request(ctx, http.MethodPost, "", raw, http.StatusCreated)
	if err != nil {
		return Task{}, err
	}
	task, err := decodeTask(response)
	if err != nil {
		return Task{}, &Error{Code: "invalid_response", OutcomeUnknown: true}
	}
	return task, nil
}
func (c *Client) GetTask(ctx context.Context, id string) (Task, error) {
	if !validTaskID(id) {
		return Task{}, localError("invalid_request")
	}
	response, err := c.request(ctx, http.MethodGet, "/"+id, nil, http.StatusOK)
	if err != nil {
		return Task{}, err
	}
	task, err := decodeTask(response)
	if err != nil || task.ID != id {
		return Task{}, localError("invalid_response")
	}
	return task, nil
}
func (c *Client) ListTasks(ctx context.Context, page, size int) (TaskPage, error) {
	if page < 1 || page > 2147483647 || size < 1 || size > 100 {
		return TaskPage{}, localError("invalid_request")
	}
	query := url.Values{"page": {strconv.Itoa(page)}, "size": {strconv.Itoa(size)}}
	response, err := c.request(ctx, http.MethodGet, "?"+query.Encode(), nil, http.StatusOK)
	if err != nil {
		return TaskPage{}, err
	}
	return decodePage(response, page, size)
}
func (c *Client) request(ctx context.Context, method, suffix string, body []byte, expectedStatus int) ([]byte, error) {
	if c == nil || c.http == nil || c.endpoint == "" || ctx == nil {
		return nil, localError("invalid_config")
	}
	if ctx.Err() != nil {
		return nil, localError("cancelled")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+suffix, bytes.NewReader(body))
	if err != nil {
		return nil, localError("invalid_request")
	}
	// Disable transport replay of a non-idempotent submission. No synthetic
	// Idempotency-Key is sent; provider support has not been established.
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if ctx.Err() != nil {
		return nil, localError("cancelled")
	}
	// Crossing Do is the uncertainty boundary even when the caller cannot tell
	// whether a request reached the provider. Never retry or expose its raw error.
	response, err := c.http.Do(req)
	unknown := method == http.MethodPost
	if err != nil {
		return nil, &Error{Code: "transport", OutcomeUnknown: unknown}
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		return nil, &Error{Code: "http_status", StatusCode: response.StatusCode, OutcomeUnknown: unknown}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, &Error{Code: "invalid_response", OutcomeUnknown: unknown}
	}
	if response.ContentLength > maxResponseBytes {
		return nil, &Error{Code: "invalid_response", OutcomeUnknown: unknown}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return nil, &Error{Code: "invalid_response", OutcomeUnknown: unknown}
	}
	return raw, nil
}
