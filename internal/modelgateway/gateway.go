// Package modelgateway owns the model HTTP boundary for a single attempt.
// Provider credentials never enter an agent process. Before any upstream write,
// the exact operation is durably recorded; an uncertain operation is not retried.
package modelgateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	mc "github.com/ai-mada/suma/internal/modelcontract"
)

const maxRequestBytes = 2 << 20
const maxResponseBytes = 8 << 20
const maxCachedResponseBytes = 32 << 20

type Config struct {
	Context   context.Context
	RequestID string
	Directory string
	Binding   mc.Binding
	BaseURL   string
	APIKey    string
	// Persist mirrors the already-fsynced receipt into the Runner journal.
	// Failure closes admission; callers must never convert it into a retry.
	Persist func(Snapshot) error
	Abort   context.CancelFunc
}

type Connection struct {
	Protocol      string `json:"protocol"`
	BaseURL       string `json:"base_url"`
	Token         string `json:"token"`
	RequestID     string `json:"request_id"`
	BindingSHA256 string `json:"binding_sha256"`
}

type Record struct {
	mc.RequestEvidence
	ClientSHA256 string `json:"client_sha256"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// An embedded RequestEvidence has a strict UnmarshalJSON method. Decode the
// extended private receipt explicitly instead of promoting that method and
// silently losing or rejecting the journal-only fields.
func (r *Record) UnmarshalJSON(raw []byte) error {
	values, err := object(raw)
	if err != nil {
		return err
	}
	var result Record
	result.ClientSHA256, _ = values["client_sha256"].(string)
	if !mc.ValidDigest(result.ClientSHA256) {
		return errors.New("model_gateway_receipt_invalid")
	}
	delete(values, "client_sha256")
	if status, present := values["http_status"]; present {
		n, valid := integer(status)
		if !valid || n < 100 || n > 599 {
			return errors.New("model_gateway_receipt_invalid")
		}
		result.HTTPStatus = int(n)
		delete(values, "http_status")
	}
	if finish, present := values["finish_reason"]; present {
		text, valid := finish.(string)
		if !valid || !map[string]bool{"stop": true, "tool_calls": true, "length": true, "content_filter": true}[text] {
			return errors.New("model_gateway_receipt_invalid")
		}
		result.FinishReason = text
		delete(values, "finish_reason")
	}
	base, _ := json.Marshal(values)
	if json.Unmarshal(base, &result.RequestEvidence) != nil {
		return errors.New("model_gateway_receipt_invalid")
	}
	*r = result
	return nil
}

// Snapshot intentionally contains no prompts, tool results, URL, key or token.
// Known counters survive a later crash even when another request is unknown.
type Snapshot struct {
	Version     int        `json:"version"`
	RequestID   string     `json:"request_id"`
	Binding     mc.Binding `json:"binding"`
	Closed      bool       `json:"closed"`
	FailureCode string     `json:"failure_code,omitempty"`
	Records     []Record   `json:"records"`
}

type responseCache struct {
	sha  string
	kind string
}

type Gateway struct {
	mu          sync.Mutex
	config      Config
	state       Snapshot
	link        Connection
	server      *http.Server
	client      *http.Client
	cancel      context.CancelFunc
	context     context.Context
	cache       map[int]responseCache
	cachedBytes int64
	finished    chan struct{}
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func Open(cfg Config) (*Gateway, error) {
	if cfg.Context == nil || cfg.Context.Err() != nil || len(cfg.RequestID) != 64 || !mc.ValidDigest(cfg.RequestID) || mc.ValidateBinding(cfg.Binding) != nil || !filepath.IsAbs(cfg.Directory) {
		return nil, errors.New("model_gateway_configuration")
	}
	endpoint, err := mc.CanonicalEndpoint(cfg.BaseURL)
	endpointSHA, digestErr := mc.EndpointDigest(cfg.BaseURL)
	if err != nil || digestErr != nil || endpoint != cfg.BaseURL || endpointSHA != cfg.Binding.Deployment.EndpointConfigSHA256 || cfg.APIKey == "" || strings.ContainsAny(cfg.APIKey, "\x00\r\n") {
		return nil, errors.New("model_gateway_configuration")
	}
	// Snapshot a caller-owned binding so later map/pointer reuse cannot change it.
	raw, _ := json.Marshal(cfg.Binding)
	if json.Unmarshal(raw, &cfg.Binding) != nil {
		return nil, errors.New("model_gateway_configuration")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(cfg.Directory))
	if err != nil || !samePath(parent, filepath.Dir(cfg.Directory)) || os.Mkdir(cfg.Directory, 0700) != nil {
		return nil, errors.New("model_gateway_directory_not_fresh")
	}
	ctx, cancel := context.WithCancel(cfg.Context)
	g := &Gateway{config: cfg, state: Snapshot{Version: 1, RequestID: cfg.RequestID, Binding: cfg.Binding, Records: []Record{}}, context: ctx, cancel: cancel, cache: map[int]responseCache{}, finished: make(chan struct{})}
	if err := g.persist(); err != nil {
		cancel()
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, errors.New("model_gateway_listener")
	}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		_ = listener.Close()
		cancel()
		return nil, err
	}
	g.link = Connection{Protocol: mc.GatewayProtocol, BaseURL: "http://" + listener.Addr().String() + "/v1", Token: hex.EncodeToString(token[:]), RequestID: cfg.RequestID, BindingSHA256: cfg.Binding.BindingSHA256}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	// Never reuse a stale connection or install a request-body replay function.
	// Neither this transport nor a model SDK may retry an uncertain POST.
	transport.DisableKeepAlives = true
	g.client = &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	g.server = &http.Server{Handler: http.HandlerFunc(g.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		defer close(g.finished)
		_ = g.server.Serve(listener)
	}()
	return g, nil
}

func (g *Gateway) Connection() Connection { return g.link }

func (g *Gateway) persist() error {
	raw, err := json.Marshal(g.state)
	if err != nil || len(raw) > 2<<20 {
		return errors.New("model_gateway_receipt_invalid")
	}
	path := filepath.Join(g.config.Directory, "receipt.json")
	temp, err := os.CreateTemp(g.config.Directory, "receipt-*.tmp")
	if err != nil {
		return errors.New("model_gateway_persistence")
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(raw)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil || closeErr != nil || os.Rename(name, path) != nil {
		return errors.New("model_gateway_persistence")
	}
	if g.config.Persist != nil {
		if err = g.config.Persist(g.copySnapshot()); err != nil {
			return errors.New("model_gateway_persistence")
		}
	}
	return nil
}

func (g *Gateway) copySnapshot() Snapshot {
	result := g.state
	result.Records = append([]Record{}, g.state.Records...)
	for i := range result.Records {
		if original := result.Records[i].Usage; original != nil {
			copied := *original
			fields := []**int64{&copied.InputTokens, &copied.OutputTokens, &copied.CachedInputTokens, &copied.CacheWriteInputTokens, &copied.ReasoningOutputTokens}
			for _, field := range fields {
				if *field != nil {
					n := **field
					*field = &n
				}
			}
			result.Records[i].Usage = &copied
		}
	}
	// Binding was validated at construction and is never modified by the gateway.
	raw, _ := json.Marshal(g.state.Binding)
	_ = json.Unmarshal(raw, &result.Binding)
	return result
}

func (g *Gateway) reject(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "Model request could not be confirmed; inspect the original operation."}})
}

func (g *Gateway) fail(code string) {
	if g.state.FailureCode == "" {
		g.state.FailureCode = code
	}
	if g.persist() != nil {
		g.state.FailureCode = "model_gateway_persistence"
	}
	if g.config.Abort != nil {
		g.config.Abort()
	}
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+g.link.Token)) != 1 {
		g.reject(w, http.StatusUnauthorized, "model_gateway_unauthorized")
		return
	}
	if r.Header.Get("Content-Encoding") != "" {
		g.reject(w, http.StatusBadRequest, "model_gateway_request_invalid")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil || len(raw) > maxRequestBytes {
		g.reject(w, http.StatusBadRequest, "model_gateway_request_invalid")
		return
	}
	step, err := strconv.Atoi(r.Header.Get("X-Suma-Model-Step"))
	if err != nil || strconv.Itoa(step) != r.Header.Get("X-Suma-Model-Step") || step < 0 || step >= g.config.Binding.Parameters.MaxSteps {
		g.reject(w, http.StatusBadRequest, "model_gateway_step_invalid")
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Closed || g.context.Err() != nil || g.state.FailureCode != "" {
		g.reject(w, http.StatusConflict, "model_gateway_closed")
		return
	}
	clientSHA := digest(raw)
	if step < len(g.state.Records) {
		previous := g.state.Records[step]
		cached, available := g.cache[step]
		if previous.ClientSHA256 == clientSHA && previous.State == "completed" && available {
			body, err := g.cachedResponse(cached.sha)
			if err != nil {
				g.fail("model_gateway_response_cache_invalid")
				g.reject(w, http.StatusConflict, "model_gateway_original_operation_required")
				return
			}
			w.Header().Set("Content-Type", cached.kind)
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(body)
			return
		}
		g.reject(w, http.StatusConflict, "model_gateway_original_operation_required")
		return
	}
	if step != len(g.state.Records) {
		g.reject(w, http.StatusConflict, "model_gateway_step_invalid")
		return
	}
	if g.cachedBytes > maxCachedResponseBytes-maxResponseBytes {
		g.fail("model_gateway_response_storage_limit")
		g.reject(w, http.StatusTooManyRequests, "model_gateway_response_storage_limit")
		return
	}
	remaining := g.config.Binding.Parameters.MaxOutputTokens
	for _, record := range g.state.Records {
		if record.State != "completed" {
			g.reject(w, http.StatusConflict, "model_gateway_original_operation_required")
			return
		}
		// Missing usage never becomes zero. Keep the full sent allowance charged
		// against admission until a complete authoritative response permits refund.
		consumed := record.OutputAllowance
		if record.UsageState == "complete" && record.Usage != nil && record.Usage.OutputTokens != nil {
			consumed = *record.Usage.OutputTokens
		}
		remaining -= consumed
	}
	body, streaming, allowance, err := prepareRequest(raw, g.config.Binding, remaining)
	if err != nil {
		g.fail("model_gateway_request_invalid")
		g.reject(w, http.StatusBadRequest, "model_gateway_request_invalid")
		return
	}
	g.state.Records = append(g.state.Records, Record{RequestEvidence: mc.RequestEvidence{State: "started", Step: step, RequestSHA256: digest(body), OutputAllowance: allowance, UsageState: "unreported"}, ClientSHA256: clientSHA})
	// The durable started fact is the admission boundary. A disk/journal error
	// cannot silently proceed to a chargeable network operation.
	if err = g.persist(); err != nil {
		g.state.FailureCode = "model_gateway_persistence"
		if g.config.Abort != nil {
			g.config.Abort()
		}
		g.reject(w, http.StatusServiceUnavailable, "model_gateway_persistence")
		return
	}
	request, err := http.NewRequestWithContext(g.context, "POST", g.config.BaseURL+"/chat/completions", io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		g.state.Records[step].State = "rejected"
		g.fail("model_gateway_configuration")
		g.reject(w, http.StatusBadGateway, "model_gateway_configuration")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+g.config.APIKey)
	if streaming {
		request.Header.Set("Accept", "text/event-stream")
	}
	response, err := g.client.Do(request)
	if err != nil {
		g.state.Records[step].State = "unknown"
		g.fail("model_gateway_upstream_unknown")
		g.reject(w, http.StatusBadGateway, "model_gateway_upstream_unknown")
		return
	}
	defer response.Body.Close()
	record := &g.state.Records[step]
	record.HTTPStatus = response.StatusCode
	if response.StatusCode != http.StatusOK {
		record.State = "unknown"
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			record.State = "rejected"
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		g.fail("model_gateway_upstream_rejected")
		g.reject(w, http.StatusBadGateway, "model_gateway_upstream_rejected")
		return
	}
	var content []byte
	var facts responseFacts
	if streaming {
		content, facts, err = readStream(response.Body, w, g.config.Binding.Deployment.Model)
	} else {
		content, err = io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		if err == nil && len(content) <= maxResponseBytes {
			facts, err = parseResponse(content, false)
		} else {
			err = errors.New("model_gateway_response_invalid")
		}
	}
	record.ResponseSHA256 = digest(content)
	record.ProviderModel, record.ProviderRequestID = facts.model, facts.id
	record.Usage, record.UsageState = facts.usage, facts.coverage()
	record.FinishReason = facts.finish
	record.State = "completed"
	if err != nil || facts.model != g.config.Binding.Deployment.Model || facts.id == "" || facts.usage != nil && facts.usage.InputTokens != nil && *facts.usage.InputTokens > g.config.Binding.Deployment.ContextWindow || facts.usage != nil && facts.usage.OutputTokens != nil && *facts.usage.OutputTokens > allowance || facts.finish == "" {
		record.State = "unknown"
		g.fail("model_gateway_response_unverified")
		if !streaming {
			g.reject(w, http.StatusBadGateway, "model_gateway_response_unverified")
		}
		return
	}
	if err = g.storeResponse(record.ResponseSHA256, content); err == nil {
		err = g.persist()
	}
	if err != nil {
		g.state.FailureCode = "model_gateway_persistence"
		if g.config.Abort != nil {
			g.config.Abort()
		}
		if !streaming {
			g.reject(w, http.StatusServiceUnavailable, "model_gateway_persistence")
		}
		return
	}
	if facts.finish == "length" || facts.finish == "content_filter" {
		g.fail("model_gateway_completion_incomplete")
	}
	kind := "application/json"
	if streaming {
		kind = "text/event-stream"
	}
	g.cache[step] = responseCache{sha: record.ResponseSHA256, kind: kind}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "no-store")
	if streaming {
		// Earlier content was genuinely streamed. The completion suffix is held
		// until its full usage and terminal receipt have reached durable storage.
		_, _ = w.Write(facts.suffix)
	} else {
		_, _ = w.Write(content)
	}
}

// Completed responses stay in the private bounded artifact directory, not in
// an O(steps * response-size) heap. Replays verify the original content hash;
// a missing/exchanged artifact never authorizes another provider operation.
func (g *Gateway) storeResponse(sha string, body []byte) error {
	path := filepath.Join(g.config.Directory, "response-"+sha+".body")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		_, err = g.cachedResponse(sha)
		return err
	}
	if err != nil {
		return err
	}
	_, err = file.Write(body)
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	g.cachedBytes += int64(len(body))
	return nil
}

func (g *Gateway) cachedResponse(sha string) ([]byte, error) {
	path := filepath.Join(g.config.Directory, "response-"+sha+".body")
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > maxResponseBytes {
		return nil, errors.New("model_gateway_response_cache_invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(st, opened) {
		return nil, errors.New("model_gateway_response_cache_invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes || digest(body) != sha {
		return nil, errors.New("model_gateway_response_cache_invalid")
	}
	return body, nil
}

func (g *Gateway) Close() (Snapshot, error) {
	g.cancel()
	_ = g.server.Close()
	<-g.finished
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Closed {
		return g.copySnapshot(), nil
	}
	g.client.CloseIdleConnections()
	g.state.Closed = true
	if err := g.persist(); err != nil {
		g.state.FailureCode = "model_gateway_persistence"
		return g.copySnapshot(), err
	}
	return g.copySnapshot(), nil
}

func Evidence(snapshot Snapshot) mc.Evidence {
	b := snapshot.Binding
	result := mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: snapshot.Closed, BindingSHA256: b.BindingSHA256, DeploymentSHA256: b.Deployment.SHA256, ParametersSHA256: b.ParametersSHA256, Model: b.Deployment.Model, Requests: []mc.RequestEvidence{}}
	for _, record := range snapshot.Records {
		request := record.RequestEvidence
		if request.State == "started" {
			request.State = "unknown"
		}
		result.Requests = append(result.Requests, request)
	}
	return result
}

func ReadSnapshot(directory, requestID, bindingSHA string) (Snapshot, error) {
	var snapshot Snapshot
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || !samePath(resolved, directory) {
		return snapshot, errors.New("model_gateway_receipt_invalid")
	}
	info, err := os.Lstat(filepath.Join(directory, "receipt.json"))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return snapshot, errors.New("model_gateway_receipt_invalid")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "receipt.json"))
	if _, decodeErr := object(raw); err != nil || decodeErr != nil || len(raw) > 2<<20 || json.Unmarshal(raw, &snapshot) != nil || snapshot.Version != 1 || snapshot.RequestID != requestID || snapshot.Binding.BindingSHA256 != bindingSHA || mc.ValidateBinding(snapshot.Binding) != nil || len(snapshot.Records) > snapshot.Binding.Parameters.MaxSteps {
		return Snapshot{}, errors.New("model_gateway_receipt_invalid")
	}
	return snapshot, nil
}
