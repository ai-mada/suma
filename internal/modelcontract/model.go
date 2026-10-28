// Package modelcontract defines the non-secret, immutable native model binding.
package modelcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Protocol = "suma-model-execution-v1"
const ProviderProtocol = "openai-chat-completions-v1"
const GatewayProtocol = "suma-model-gateway-v1"
const MaxSteps = 128
const MaxOutputTokens int64 = 131072
const MaxSafeInteger int64 = 9007199254740991

type ModelDeployment struct {
	ID                   string `json:"deployment_id"`
	Revision             int    `json:"revision"`
	SHA256               string `json:"deployment_sha256"`
	Protocol             string `json:"protocol"`
	Model                string `json:"model"`
	EndpointOrigin       string `json:"endpoint_origin"`
	EndpointConfigSHA256 string `json:"endpoint_config_sha256"`
	ContextWindow        int64  `json:"context_window"`
}
type ModelParameters struct {
	MaxSteps        int   `json:"max_steps"`
	MaxOutputTokens int64 `json:"max_output_tokens"`
}
type ModelBudget struct {
	TokenLimit      *int64 `json:"token_limit,omitempty"`
	CostLimitMicros *int64 `json:"cost_limit_micros,omitempty"`
	Currency        string `json:"currency,omitempty"`
}
type ModelSelection struct {
	DeploymentID       string     `json:"deployment_id"`
	DeploymentRevision int        `json:"deployment_revision"`
	DeploymentSHA256   string     `json:"deployment_sha256"`
	Parameters         Parameters `json:"parameters"`
	Budget             *Budget    `json:"budget,omitempty"`
}
type ModelRateSnapshot struct {
	ID                         string `json:"id"`
	Version                    int    `json:"version"`
	Currency                   string `json:"currency"`
	InputRateMicrosPerMillion  int64  `json:"input_rate_micros_per_million"`
	OutputRateMicrosPerMillion int64  `json:"output_rate_micros_per_million"`
}
type ModelBinding struct {
	Protocol               string        `json:"protocol"`
	Deployment             Deployment    `json:"deployment"`
	Parameters             Parameters    `json:"parameters"`
	ParametersSHA256       string        `json:"parameters_sha256"`
	BindingSHA256          string        `json:"binding_sha256"`
	InputTokenReservation  int64         `json:"input_token_reservation"`
	OutputTokenReservation int64         `json:"output_token_reservation"`
	Budget                 *Budget       `json:"budget,omitempty"`
	Rate                   *RateSnapshot `json:"rate,omitempty"`
}
type ModelRequestEvidence struct {
	State             string             `json:"state"`
	Step              int                `json:"step"`
	RequestSHA256     string             `json:"request_sha256"`
	ResponseSHA256    string             `json:"response_sha256,omitempty"`
	ProviderModel     string             `json:"provider_model,omitempty"`
	ProviderRequestID string             `json:"provider_request_id,omitempty"`
	OutputAllowance   int64              `json:"output_allowance"`
	UsageState        string             `json:"usage_state"`
	Usage             *ModelRequestUsage `json:"usage,omitempty"`
}

// ModelRequestUsage contains host-observed exact counters. Missing counters are
// absent: neither an SDK aggregate nor a failed request implies zero usage.
type ModelRequestUsage struct {
	InputTokens           *int64 `json:"input_tokens,omitempty"`
	OutputTokens          *int64 `json:"output_tokens,omitempty"`
	CachedInputTokens     *int64 `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens,omitempty"`
}
type ModelEvidence struct {
	GatewayProtocol  string            `json:"gateway_protocol"`
	GatewayClosed    bool              `json:"gateway_closed"`
	BindingSHA256    string            `json:"binding_sha256"`
	DeploymentSHA256 string            `json:"deployment_sha256"`
	ParametersSHA256 string            `json:"parameters_sha256"`
	Model            string            `json:"model"`
	Requests         []RequestEvidence `json:"requests"`
}

type Deployment = ModelDeployment
type Parameters = ModelParameters
type Budget = ModelBudget
type Selection = ModelSelection
type RateSnapshot = ModelRateSnapshot
type Binding = ModelBinding
type RequestEvidence = ModelRequestEvidence
type RequestUsage = ModelRequestUsage
type Evidence = ModelEvidence

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidDigest(v string) bool { return shaPattern.MatchString(v) }
func Digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func DeploymentDigest(v Deployment) string { v.SHA256 = ""; return Digest(v) }
func ParametersDigest(v Parameters) string { return Digest(v) }
func BindingDigest(v Binding) string       { v.BindingSHA256 = ""; return Digest(v) }

// CanonicalEndpoint fixes the full API path; origin alone cannot identify it.
func CanonicalEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.TrimSpace(raw) != raw || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return "", fmt.Errorf("invalid model endpoint")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func EndpointDigest(raw string) (string, error) {
	endpoint, err := CanonicalEndpoint(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:]), nil
}
func ValidateDeployment(v Deployment) error {
	endpoint, err := CanonicalEndpoint(v.EndpointOrigin)
	u, _ := url.Parse(v.EndpointOrigin)
	if !idPattern.MatchString(v.ID) || v.Revision < 1 || v.Revision > 2147483647 || v.Protocol != ProviderProtocol || v.Model == "" || !boundedText(v.Model, 256) || strings.TrimSpace(v.Model) != v.Model || err != nil || endpoint != v.EndpointOrigin || u.Path != "" || !ValidDigest(v.EndpointConfigSHA256) || v.ContextWindow < 1 || v.ContextWindow > 10000000 || v.SHA256 != DeploymentDigest(v) {
		return fmt.Errorf("invalid immutable model deployment")
	}
	return nil
}
func ValidateParameters(v Parameters) error {
	if v.MaxSteps < 1 || v.MaxSteps > MaxSteps || v.MaxOutputTokens < 1 || v.MaxOutputTokens > MaxOutputTokens {
		return fmt.Errorf("invalid model execution parameters")
	}
	return nil
}
func ValidateBudget(v *Budget) error {
	if v == nil {
		return nil
	}
	if v.TokenLimit == nil && v.CostLimitMicros == nil || v.TokenLimit != nil && (*v.TokenLimit < 0 || *v.TokenLimit > MaxSafeInteger) || v.CostLimitMicros != nil && (*v.CostLimitMicros < 0 || *v.CostLimitMicros > MaxSafeInteger || v.Currency != "CNY" && v.Currency != "USD") || v.CostLimitMicros == nil && v.Currency != "" {
		return fmt.Errorf("invalid model execution budget")
	}
	return nil
}
func ValidateSelection(v Selection) error {
	if !idPattern.MatchString(v.DeploymentID) || v.DeploymentRevision < 1 || v.DeploymentRevision > 2147483647 || !ValidDigest(v.DeploymentSHA256) {
		return fmt.Errorf("invalid model selection")
	}
	if err := ValidateParameters(v.Parameters); err != nil {
		return err
	}
	return ValidateBudget(v.Budget)
}
func NewBinding(d Deployment, p Parameters, budget *Budget, rate *RateSnapshot) (Binding, error) {
	b := Binding{Protocol: Protocol, Deployment: d, Parameters: p, ParametersSHA256: ParametersDigest(p), InputTokenReservation: d.ContextWindow * int64(p.MaxSteps), OutputTokenReservation: p.MaxOutputTokens, Budget: budget, Rate: rate}
	b.BindingSHA256 = BindingDigest(b)
	return b, ValidateBinding(b)
}
func ValidateBinding(b Binding) error {
	if err := ValidateDeployment(b.Deployment); err != nil {
		return err
	}
	if err := ValidateParameters(b.Parameters); err != nil {
		return err
	}
	if err := ValidateBudget(b.Budget); err != nil {
		return err
	}
	if b.Protocol != Protocol || b.ParametersSHA256 != ParametersDigest(b.Parameters) || b.BindingSHA256 != BindingDigest(b) || b.InputTokenReservation != b.Deployment.ContextWindow*int64(b.Parameters.MaxSteps) || b.OutputTokenReservation != b.Parameters.MaxOutputTokens || b.InputTokenReservation > MaxSafeInteger-b.OutputTokenReservation {
		return fmt.Errorf("invalid immutable model binding")
	}
	if r := b.Rate; r != nil {
		if !idPattern.MatchString(r.ID) || r.Version < 1 || r.Currency != "CNY" && r.Currency != "USD" || r.InputRateMicrosPerMillion < 0 || r.InputRateMicrosPerMillion > MaxSafeInteger || r.OutputRateMicrosPerMillion < 0 || r.OutputRateMicrosPerMillion > MaxSafeInteger {
			return fmt.Errorf("invalid model rate snapshot")
		}
		if _, err := ReservedCostMicros(b); err != nil {
			return err
		}
	}
	if b.Budget != nil {
		if b.Budget.TokenLimit != nil && b.InputTokenReservation+b.OutputTokenReservation > *b.Budget.TokenLimit {
			return fmt.Errorf("model token budget cannot cover reservation")
		}
		if b.Budget.CostLimitMicros != nil {
			if b.Rate == nil || b.Rate.Currency != b.Budget.Currency {
				return fmt.Errorf("model cost budget requires matching exact rate")
			}
			cost, err := ReservedCostMicros(b)
			if err != nil || cost > *b.Budget.CostLimitMicros {
				return fmt.Errorf("model cost budget cannot cover reservation")
			}
		}
	}
	return nil
}
func ReservedCostMicros(b Binding) (int64, error) {
	if b.Rate == nil {
		return 0, fmt.Errorf("model rate is unknown")
	}
	total := new(big.Int).Mul(big.NewInt(b.InputTokenReservation), big.NewInt(b.Rate.InputRateMicrosPerMillion))
	total.Add(total, new(big.Int).Mul(big.NewInt(b.OutputTokenReservation), big.NewInt(b.Rate.OutputRateMicrosPerMillion)))
	total.Add(total, big.NewInt(999999))
	total.Quo(total, big.NewInt(1000000))
	if !total.IsInt64() || total.Sign() < 0 || total.Cmp(big.NewInt(MaxSafeInteger)) > 0 {
		return 0, fmt.Errorf("model reservation cost overflows")
	}
	return total.Int64(), nil
}
func ValidateEvidence(v Evidence) error {
	if v.GatewayProtocol != GatewayProtocol || !ValidDigest(v.BindingSHA256) || !ValidDigest(v.DeploymentSHA256) || !ValidDigest(v.ParametersSHA256) || v.Model == "" || !boundedText(v.Model, 256) || v.Requests == nil || len(v.Requests) > MaxSteps {
		return fmt.Errorf("invalid model execution evidence")
	}
	for i, r := range v.Requests {
		if r.Step != i || !ValidDigest(r.RequestSHA256) || r.ResponseSHA256 != "" && !ValidDigest(r.ResponseSHA256) || !boundedText(r.ProviderModel, 256) || !boundedText(r.ProviderRequestID, 256) || r.OutputAllowance < 1 || r.OutputAllowance > MaxOutputTokens || r.UsageState != "complete" && r.UsageState != "partial" && r.UsageState != "unreported" {
			return fmt.Errorf("invalid model request evidence")
		}
		if r.State != "started" && r.State != "completed" && r.State != "rejected" && r.State != "unknown" || validateRequestUsage(r.UsageState, r.Usage) != nil {
			return fmt.Errorf("invalid model request state or usage")
		}
		if r.State == "started" && (v.GatewayClosed || r.ResponseSHA256 != "" || r.ProviderModel != "" || r.ProviderRequestID != "" || r.Usage != nil) || r.State == "completed" && (r.ResponseSHA256 == "" || r.ProviderModel == "") {
			return fmt.Errorf("invalid model request lifecycle evidence")
		}
	}
	return nil
}

func validateRequestUsage(state string, u *RequestUsage) error {
	if u == nil {
		if state != "unreported" {
			return fmt.Errorf("missing model request usage")
		}
		return nil
	}
	present := false
	for _, n := range []*int64{u.InputTokens, u.OutputTokens, u.CachedInputTokens, u.CacheWriteInputTokens, u.ReasoningOutputTokens} {
		if n == nil {
			continue
		}
		present = true
		if *n < 0 || *n > MaxSafeInteger {
			return fmt.Errorf("invalid model request counter")
		}
	}
	if !present || state == "unreported" || state == "complete" && (u.InputTokens == nil || u.OutputTokens == nil) {
		return fmt.Errorf("invalid model request usage coverage")
	}
	if u.InputTokens != nil {
		var cached, write int64
		if u.CachedInputTokens != nil {
			cached = *u.CachedInputTokens
		}
		if u.CacheWriteInputTokens != nil {
			write = *u.CacheWriteInputTokens
		}
		if cached+write > *u.InputTokens {
			return fmt.Errorf("conflicting model input counters")
		}
	}
	if u.OutputTokens != nil && u.ReasoningOutputTokens != nil && *u.ReasoningOutputTokens > *u.OutputTokens {
		return fmt.Errorf("conflicting model output counters")
	}
	return nil
}

// EvidenceMatchesBinding checks immutable identity separately from execution
// success. Billing can be known even when the source check or sealing fails.
func EvidenceMatchesBinding(b Binding, e Evidence) bool {
	return ValidateBinding(b) == nil && ValidateEvidence(e) == nil &&
		e.BindingSHA256 == b.BindingSHA256 && e.DeploymentSHA256 == b.Deployment.SHA256 &&
		e.ParametersSHA256 == b.ParametersSHA256 && e.Model == b.Deployment.Model &&
		len(e.Requests) <= b.Parameters.MaxSteps
}

// A request spends its entire output allowance before HTTP starts. Only an
// original complete response may return unused allowance to the next request.
func evidenceAllowancesValid(b Binding, e Evidence) bool {
	var consumed int64
	for _, r := range e.Requests {
		if r.OutputAllowance > b.OutputTokenReservation-consumed {
			return false
		}
		consumed += r.OutputAllowance
		if r.State == "completed" && r.UsageState == "complete" && r.Usage != nil && r.Usage.OutputTokens != nil {
			if *r.Usage.OutputTokens > r.OutputAllowance {
				return false
			}
			consumed -= r.OutputAllowance - *r.Usage.OutputTokens
		}
	}
	return true
}

// EvidenceResponsesComplete proves provider request coverage for code execution.
// Token coverage is separate: a valid model reply may omit billing counters.
func EvidenceResponsesComplete(b Binding, e Evidence) bool {
	if !EvidenceMatchesBinding(b, e) || !e.GatewayClosed || len(e.Requests) == 0 || !evidenceAllowancesValid(b, e) {
		return false
	}
	for _, r := range e.Requests {
		if r.State != "completed" || r.ResponseSHA256 == "" || r.ProviderModel != b.Deployment.Model {
			return false
		}
	}
	return true
}

// AggregateEvidenceUsage preserves every known host counter. Completeness also
// needs the gateway closed and every admitted request's original response. A
// genuinely closed gateway with no requests certifies zero without a model ACK.
func AggregateEvidenceUsage(b Binding, e Evidence) (*RequestUsage, bool) {
	if !EvidenceMatchesBinding(b, e) {
		return nil, false
	}
	complete := e.GatewayClosed && evidenceAllowancesValid(b, e)
	values := []*int64{nil, nil, nil, nil, nil}
	for _, r := range e.Requests {
		if r.State != "completed" || r.ResponseSHA256 == "" || r.ProviderModel != b.Deployment.Model || r.UsageState != "complete" || r.Usage == nil {
			complete = false
		}
		if r.Usage == nil {
			continue
		}
		counts := []*int64{r.Usage.InputTokens, r.Usage.OutputTokens, r.Usage.CachedInputTokens, r.Usage.CacheWriteInputTokens, r.Usage.ReasoningOutputTokens}
		for i, n := range counts {
			if n == nil {
				continue
			}
			if values[i] == nil {
				v := int64(0)
				values[i] = &v
			}
			if *values[i] > MaxSafeInteger-*n {
				return nil, false
			}
			*values[i] += *n
		}
		if r.Usage.InputTokens != nil && *r.Usage.InputTokens > b.Deployment.ContextWindow || r.Usage.OutputTokens != nil && *r.Usage.OutputTokens > r.OutputAllowance {
			complete = false
		}
	}
	if len(e.Requests) == 0 && complete {
		input, output := int64(0), int64(0)
		values[0], values[1] = &input, &output
	}
	u := &RequestUsage{InputTokens: values[0], OutputTokens: values[1], CachedInputTokens: values[2], CacheWriteInputTokens: values[3], ReasoningOutputTokens: values[4]}
	if values[0] == nil && values[1] == nil && values[2] == nil && values[3] == nil && values[4] == nil {
		return nil, false
	}
	if u.InputTokens != nil && *u.InputTokens > b.InputTokenReservation || u.OutputTokens != nil && *u.OutputTokens > b.OutputTokenReservation {
		complete = false
	}
	return u, complete
}

func boundedText(value string, limit int) bool {
	if len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func DecodeBinding(value any) (Binding, error) {
	var b Binding
	if err := decode(value, &b); err != nil {
		return b, err
	}
	return b, ValidateBinding(b)
}
func DecodeSelection(value any) (Selection, error) {
	var s Selection
	if err := decode(value, &s); err != nil {
		return s, err
	}
	return s, ValidateSelection(s)
}
func decode(value any, out any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("model object cannot be null")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid model JSON")
	}
	return nil
}
