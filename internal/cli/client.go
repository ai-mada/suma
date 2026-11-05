package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

type failure struct {
	Code          string              `json:"code"`
	Message       string              `json:"message"`
	Status        int                 `json:"status,omitempty"`
	Details       any                 `json:"details,omitempty"`
	MessageKey    string              `json:"message_key,omitempty"`
	MessageParams localization.Params `json:"message_params,omitempty"`
	Locale        localization.Locale `json:"locale,omitempty"`
	// nil means unreported; an explicit false must survive the CLI round trip.
	Localized *bool  `json:"localized,omitempty"`
	Fallback  string `json:"fallback,omitempty"`
	remote    bool
}

func (e *failure) Error() string { return e.Message }
func usage(message string) error { return &failure{Code: "usage", Message: message} }

type client struct {
	base, project, token               string
	expectedUser, expectedOrganization string
	timeout                            time.Duration
	http                               *http.Client
	locale                             localization.Locale
}

func newClient(control, project, token string, timeout time.Duration) (*client, error) {
	u, err := url.Parse(control)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, usage("--control must be an http(s) origin without credentials, query, or path")
	}
	if timeout <= 0 || strings.ContainsAny(token, "\r\n") {
		return nil, usage("invalid timeout or token")
	}
	return &client{base: strings.TrimSuffix(control, "/"), project: project, token: token, timeout: timeout,
		http: &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("redirect refused") }}}, nil
}

var originalViewID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Explicit IDs are captured once for this CLI invocation. Retries never fetch a
// newer session to replace the operator's original account/organization view.
func (c *client) setOriginalView(user, organization string) error {
	if user != "" && !originalViewID.MatchString(user) || organization != "" && (!originalViewID.MatchString(organization) || user == "") {
		return usage("original-view IDs must be 32 lowercase hexadecimal characters; organization requires user")
	}
	c.expectedUser, c.expectedOrganization = user, organization
	return nil
}

func (c *client) buildRequest(ctx context.Context, method, path string, body map[string]any, scoped bool) (*http.Request, error) {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "..") || strings.ContainsAny(u.Path, "\\\r\n") || u.Fragment != "" {
		return nil, usage("API path must be a relative /api/v1 resource path")
	}
	q := u.Query()
	for name := range q {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") {
			return nil, usage("credentials must not be included in API URLs")
		}
	}
	if scoped {
		if c.project == "" {
			return nil, usage("--project is required for this command")
		}
		if value := q.Get("project_id"); value != "" && value != c.project {
			return nil, usage("URL project_id differs from --project")
		}
		q.Set("project_id", c.project)
		if body != nil {
			if value, exists := body["project_id"]; exists && value != c.project {
				return nil, usage("JSON project_id differs from --project")
			}
			body["project_id"] = c.project
		}
	}
	u.RawQuery = q.Encode()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, usage("invalid JSON request")
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+u.String(), reader)
	if err != nil {
		return nil, usage("invalid request")
	}
	req.Header.Set("Accept", "application/json")
	locale := c.locale
	if locale == "" {
		locale = localization.DefaultLocale
	}
	req.Header.Set("Accept-Language", string(locale))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	// Explicit expected-view assertions also bind scoped reads. A read-only
	// method must not silently drop the user's requested account/project view.
	if scoped || method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		if c.expectedUser != "" {
			req.Header.Set("X-Suma-Expected-User", c.expectedUser)
		}
		if c.expectedOrganization != "" {
			req.Header.Set("X-Suma-Expected-Organization", c.expectedOrganization)
		}
	}
	return req, nil
}

func (c *client) request(ctx context.Context, method, path string, body map[string]any, scoped bool) (json.RawMessage, error) {
	req, err := c.buildRequest(ctx, method, path, body, scoped)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return nil, controlTransportFailure(ctx, !requestIsReadOnly(method, path))
	}
	defer resp.Body.Close()
	raw, err := readControlResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, controlHTTPFailure(resp.StatusCode, raw)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	if !json.Valid(raw) {
		return nil, &failure{Code: "response", Message: "control response was not valid JSON", Status: resp.StatusCode}
	}
	return raw, nil
}

func controlTransportFailure(ctx context.Context, outcomeUncertain bool) *failure {
	message := "control request failed"
	if ctx.Err() != nil {
		message = "control request cancelled or timed out"
	}
	if outcomeUncertain {
		message += "; outcome may be unknown: retry only with the same operation key and payload"
	}
	return &failure{Code: "transport", Message: message}
}

func readControlResponseBody(resp *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
	if err != nil || len(raw) > 16*1024*1024 {
		return nil, &failure{Code: "response", Message: "unable to read bounded response", Status: resp.StatusCode}
	}
	return raw, nil
}

func controlHTTPFailure(status int, raw []byte) *failure {
	f := &failure{Code: "http_error", Message: http.StatusText(status), Status: status, remote: true}
	var envelope struct {
		Error failure `json:"error"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if json.Valid(raw) && decoder.Decode(&envelope) == nil && envelope.Error.Code != "" {
		*f = envelope.Error
		f.Status, f.remote = status, true
	}
	return f
}

func readObject(file string, in io.Reader) (map[string]any, error) {
	reader := in
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return nil, usage("cannot open JSON file")
		}
		defer f.Close()
		reader = f
	}
	if reader == nil {
		return nil, usage("cannot read JSON input")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, 4*1024*1024+1))
	if err != nil || len(raw) > 4*1024*1024 {
		return nil, usage("JSON file exceeds 4 MiB or cannot be read")
	}
	if !utf8.Valid(raw) {
		return nil, usage("JSON input must be valid UTF-8")
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil || object == nil {
		return nil, usage("JSON file must contain an object")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, usage("JSON file must contain exactly one object")
	}
	return object, nil
}

func (c *client) operation(method, path string, body map[string]any) error {
	if key, exists := body["idempotency_key"]; exists {
		value, ok := key.(string)
		if !ok || len(value) < 8 {
			return usage("idempotency_key must be a string of at least 8 characters")
		}
		return nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return usage("invalid operation payload")
	}
	hash := sha256.Sum256([]byte(c.base + "\n" + c.project + "\n" + method + "\n" + path + "\n" + string(raw)))
	body["idempotency_key"] = "cli-" + hex.EncodeToString(hash[:])
	return nil
}

func writeJSON(w io.Writer, value any, token string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if token != "" {
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return err
		}
		value = redact(decoded, token)
	}
	raw, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(raw))
	return err
}

func redact(value any, token string) any {
	switch value := value.(type) {
	case string:
		return strings.ReplaceAll(value, token, "[redacted]")
	case []any:
		for i := range value {
			value[i] = redact(value[i], token)
		}
		return value
	case map[string]any:
		clean := make(map[string]any, len(value))
		for key, entry := range value {
			clean[strings.ReplaceAll(key, token, "[redacted]")] = redact(entry, token)
		}
		return clean
	default:
		return value
	}
}
