// Package codexcontract defines public, bounded Codex app-server configuration
// evidence. Acknowledged settings are not per-request provider model identity;
// token thresholds are notices, never hard spending or execution allowances.
package codexcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"unicode/utf8"
)

const Protocol = "codex-app-server-v1"
const BudgetMode = "observe_only"
const IdentityEvidence = "configuration_ack_only"
const MaxSafeInteger int64 = 9007199254740991

// ModelOption is projected from the CLI's actual model/list response. It does
// not assert account entitlement, endpoint health, a price or a provider bill.
type CodexModelOption struct {
	Model                  string   `json:"model"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort"`
}
type CodexCatalog struct {
	Protocol            string        `json:"protocol"`
	ConfigurationSHA256 string        `json:"configuration_sha256"`
	Models              []ModelOption `json:"models"`
	CatalogSHA256       string        `json:"catalog_sha256"`
}
type CodexSelection struct {
	CatalogSHA256    string `json:"catalog_sha256"`
	Model            string `json:"model"`
	ReasoningEffort  string `json:"reasoning_effort"`
	TokenNoticeLimit *int64 `json:"token_notice_limit,omitempty"`
}
type CodexBinding struct {
	Protocol            string `json:"protocol"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	CatalogSHA256       string `json:"catalog_sha256"`
	Model               string `json:"model"`
	ReasoningEffort     string `json:"reasoning_effort"`
	TokenNoticeLimit    *int64 `json:"token_notice_limit,omitempty"`
	BudgetMode          string `json:"budget_mode"`
	IdentityEvidence    string `json:"identity_evidence"`
	BindingSHA256       string `json:"binding_sha256"`
}

// Evidence keeps requested and acknowledged configuration separate. Usage is
// carried by the existing ToolEvidence counters with partial/unknown coverage.
// Missing turn-start acknowledgements never authorize automatic replay/resume.
type CodexEvidence struct {
	BindingSHA256               string   `json:"binding_sha256"`
	ThreadID                    string   `json:"thread_id,omitempty"`
	TurnID                      string   `json:"turn_id,omitempty"`
	RequestedModel              string   `json:"requested_model"`
	RequestedReasoningEffort    string   `json:"requested_reasoning_effort"`
	AcknowledgedModel           string   `json:"acknowledged_model,omitempty"`
	AcknowledgedReasoningEffort string   `json:"acknowledged_reasoning_effort,omitempty"`
	AcknowledgedProvider        string   `json:"acknowledged_provider,omitempty"`
	ConfigurationAcknowledged   bool     `json:"configuration_acknowledged"`
	InterruptRequested          bool     `json:"interrupt_requested"`
	InterruptAcknowledged       bool     `json:"interrupt_acknowledged"`
	ReroutedModels              []string `json:"rerouted_models,omitempty"`
	TokenNoticeExceeded         bool     `json:"token_notice_exceeded"`
}

// Public type names are unique in the generated OpenAPI/TypeScript namespace.
type ModelOption = CodexModelOption
type Catalog = CodexCatalog
type Selection = CodexSelection
type Binding = CodexBinding
type Evidence = CodexEvidence

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$`)
var hash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validID(value string, optional bool) bool {
	return optional && value == "" || identifier.MatchString(value)
}
func notice(value *int64) bool { return value == nil || *value > 0 && *value <= MaxSafeInteger }
func digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func CatalogDigest(c Catalog) string { c.CatalogSHA256 = ""; return digest(c) }
func BindingDigest(b Binding) string { b.BindingSHA256 = ""; return digest(b) }
func CloneCatalog(c Catalog) Catalog {
	c.Models = append([]ModelOption{}, c.Models...)
	for i := range c.Models {
		c.Models[i].ReasoningEfforts = append([]string{}, c.Models[i].ReasoningEfforts...)
	}
	return c
}
func CloneBinding(b Binding) Binding {
	if b.TokenNoticeLimit != nil {
		value := *b.TokenNoticeLimit
		b.TokenNoticeLimit = &value
	}
	return b
}
func CloneEvidence(e *Evidence) *Evidence {
	if e == nil {
		return nil
	}
	copy := *e
	copy.ReroutedModels = append([]string(nil), e.ReroutedModels...)
	return &copy
}
func NewCatalog(configurationSHA string, models []ModelOption) (Catalog, error) {
	c := CloneCatalog(Catalog{Protocol: Protocol, ConfigurationSHA256: configurationSHA, Models: models})
	for i := range c.Models {
		sort.Strings(c.Models[i].ReasoningEfforts)
	}
	sort.Slice(c.Models, func(i, j int) bool { return c.Models[i].Model < c.Models[j].Model })
	c.CatalogSHA256 = CatalogDigest(c)
	return c, ValidateCatalog(c)
}
func ValidateCatalog(c Catalog) error {
	if c.Protocol != Protocol || !hash.MatchString(c.ConfigurationSHA256) || !hash.MatchString(c.CatalogSHA256) || len(c.Models) < 1 || len(c.Models) > 256 || c.CatalogSHA256 != CatalogDigest(c) {
		return fmt.Errorf("Codex model catalog is invalid")
	}
	previous := ""
	for _, model := range c.Models {
		if !validID(model.Model, false) || model.Model <= previous || len(model.ReasoningEfforts) > 32 || !validID(model.DefaultReasoningEffort, true) {
			return fmt.Errorf("Codex model options are invalid")
		}
		previous = model.Model
		effortBefore := ""
		defaultFound := len(model.ReasoningEfforts) == 0 && model.DefaultReasoningEffort == ""
		for _, effort := range model.ReasoningEfforts {
			if !validID(effort, false) || effort <= effortBefore {
				return fmt.Errorf("Codex reasoning options are invalid")
			}
			effortBefore = effort
			defaultFound = defaultFound || effort == model.DefaultReasoningEffort
		}
		if !defaultFound {
			return fmt.Errorf("Codex default reasoning effort is unavailable")
		}
	}
	return nil
}
func ValidateSelection(s Selection) error {
	if !hash.MatchString(s.CatalogSHA256) || !validID(s.Model, false) || !validID(s.ReasoningEffort, true) || !notice(s.TokenNoticeLimit) {
		return fmt.Errorf("Codex model selection is invalid")
	}
	return nil
}
func NewBinding(c Catalog, s Selection) (Binding, error) {
	if ValidateCatalog(c) != nil || ValidateSelection(s) != nil || c.CatalogSHA256 != s.CatalogSHA256 {
		return Binding{}, fmt.Errorf("Codex model selection is unavailable")
	}
	found := false
	for _, model := range c.Models {
		if model.Model != s.Model {
			continue
		}
		found = len(model.ReasoningEfforts) == 0 && s.ReasoningEffort == ""
		for _, effort := range model.ReasoningEfforts {
			found = found || effort == s.ReasoningEffort
		}
	}
	if !found {
		return Binding{}, fmt.Errorf("Codex model selection is unavailable")
	}
	b := CloneBinding(Binding{Protocol: Protocol, ConfigurationSHA256: c.ConfigurationSHA256, CatalogSHA256: c.CatalogSHA256, Model: s.Model, ReasoningEffort: s.ReasoningEffort, TokenNoticeLimit: s.TokenNoticeLimit, BudgetMode: BudgetMode, IdentityEvidence: IdentityEvidence})
	b.BindingSHA256 = BindingDigest(b)
	return b, ValidateBinding(b)
}
func ValidateBinding(b Binding) error {
	if b.Protocol != Protocol || b.BudgetMode != BudgetMode || b.IdentityEvidence != IdentityEvidence || !hash.MatchString(b.ConfigurationSHA256) || !hash.MatchString(b.CatalogSHA256) || !hash.MatchString(b.BindingSHA256) || !validID(b.Model, false) || !validID(b.ReasoningEffort, true) || !notice(b.TokenNoticeLimit) || b.BindingSHA256 != BindingDigest(b) {
		return fmt.Errorf("Codex execution binding is invalid")
	}
	return nil
}
func ValidateEvidence(e Evidence) error {
	if !hash.MatchString(e.BindingSHA256) || !validID(e.ThreadID, true) || !validID(e.TurnID, true) || !validID(e.RequestedModel, false) || !validID(e.RequestedReasoningEffort, true) || !validID(e.AcknowledgedModel, true) || !validID(e.AcknowledgedReasoningEffort, true) || !validID(e.AcknowledgedProvider, true) || len(e.ReroutedModels) > 32 || e.TurnID != "" && e.ThreadID == "" || e.InterruptAcknowledged && !e.InterruptRequested {
		return fmt.Errorf("Codex configuration evidence is invalid")
	}
	if e.ConfigurationAcknowledged && (e.ThreadID == "" || e.AcknowledgedModel != e.RequestedModel || e.AcknowledgedReasoningEffort != e.RequestedReasoningEffort || e.AcknowledgedProvider == "") {
		return fmt.Errorf("Codex configuration acknowledgement is invalid")
	}
	for _, model := range e.ReroutedModels {
		if !validID(model, false) {
			return fmt.Errorf("Codex model reroute evidence is invalid")
		}
	}
	return nil
}
func EvidenceMatchesBinding(b Binding, e Evidence) bool {
	return ValidateBinding(b) == nil && ValidateEvidence(e) == nil && e.BindingSHA256 == b.BindingSHA256 && e.RequestedModel == b.Model && e.RequestedReasoningEffort == b.ReasoningEffort && e.ConfigurationAcknowledged && e.TurnID != "" && (!e.TokenNoticeExceeded || b.TokenNoticeLimit != nil)
}

// Strict decoding rejects private additions, duplicate fields and deep inputs.
// The original integer lexeme must decode exactly into the typed int64 field.
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
func decode(value any, result any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 || !utf8.Valid(raw) || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("Codex object encoding is invalid")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err = scanJSON(scan, 0); err != nil {
		return err
	}
	if _, err = scan.Token(); err != io.EOF {
		return fmt.Errorf("Codex object has trailing data")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(result)
}
func scanJSON(d *json.Decoder, depth int) error {
	if depth > 8 {
		return fmt.Errorf("Codex JSON nesting limit exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter == '{' {
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("Codex JSON field is repeated")
			}
			seen[name] = true
			if err = scanJSON(d, depth+1); err != nil {
				return err
			}
		}
	} else if delimiter == '[' {
		for d.More() {
			if err = scanJSON(d, depth+1); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("Codex JSON delimiter is invalid")
	}
	_, err = d.Token()
	return err
}

// HasNodeOverrides is the shared admission/runner policy. A workflow supplies
// a reviewed binding and goal; it cannot replace the local provider, process,
// permission scope or claim native hard limits through unrelated template keys.
func HasNodeOverrides(config map[string]any) bool {
	for _, field := range []string{"model_execution", "model", "model_id", "effort", "reasoning_effort", "model_provider", "provider", "base_url", "endpoint", "api_key", "env", "env_from", "cwd", "argv", "max_steps", "max_output_tokens", "service_tier", "config", "configuration", "sandbox", "approval_policy", "windows", "windows_sandbox"} {
		if _, exists := config[field]; exists {
			return true
		}
	}
	return false
}
