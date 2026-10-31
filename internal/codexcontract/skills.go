package codexcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const ExecutionSkillProtocol = "suma-codex-skills-v1"
const MaxExecutionSkills = 16
const MaxExecutionSkillBytes = 32000

// ExecutionSkillDocument binds a reviewed knowledge revision and the different
// digest of its native SKILL.md bytes. Neither digest grants tool permissions.
type ExecutionSkillDocument struct {
	ID             string `json:"id"`
	Revision       int    `json:"revision"`
	SHA256         string `json:"sha256"`
	Title          string `json:"title"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Content        string `json:"content"`
	ContentSHA256  string `json:"content_sha256"`
	DocumentSHA256 string `json:"document_sha256"`
}

// ExecutionSkillBundle is generated from the protected plan, not browser paths
// or workflow text. Its order is the original explicit selection order.
type ExecutionSkillBundle struct {
	Protocol     string                   `json:"protocol"`
	Documents    []ExecutionSkillDocument `json:"documents"`
	BundleSHA256 string                   `json:"bundle_sha256"`
}

// ExecutionSkillReceipt deliberately excludes private filesystem paths and
// instruction text. It can be shown with an attempt without leaking its home.
type ExecutionSkillReceipt struct {
	ID             string `json:"id"`
	Revision       int    `json:"revision"`
	SHA256         string `json:"sha256"`
	Name           string `json:"name"`
	DocumentSHA256 string `json:"document_sha256"`
}

// ExecutionSkillEvidence separates host bytes, CLI discovery and invocation
// acknowledgements. None proves the model read or successfully applied a skill.
type ExecutionSkillEvidence struct {
	BundleSHA256        string                  `json:"bundle_sha256"`
	Documents           []ExecutionSkillReceipt `json:"documents"`
	Materialized        bool                    `json:"materialized"`
	CatalogAcknowledged bool                    `json:"catalog_acknowledged"`
	InvocationRequested bool                    `json:"invocation_requested"`
	TurnAcknowledged    bool                    `json:"turn_acknowledged"`
	FilesVerified       bool                    `json:"files_verified"`
	FailureCode         string                  `json:"failure_code,omitempty"`
}

func executionSkillBytesSHA(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ExecutionSkillName never turns a user-controlled title or ID into a path.
func ExecutionSkillName(id string) string {
	return "suma-" + executionSkillBytesSHA([]byte(id))[:24]
}

func validSkillText(value string, limit int, required bool) bool {
	return utf8.ValidString(value) && len(value) <= limit && !strings.ContainsRune(value, 0) && (!required || strings.TrimSpace(value) != "")
}

func validateSkillDocumentIdentity(d ExecutionSkillDocument) error {
	if !validID(d.ID, false) || d.Revision <= 0 || int64(d.Revision) > MaxSafeInteger || !hash.MatchString(d.SHA256) || d.Name != ExecutionSkillName(d.ID) || !validSkillText(d.Title, 200, true) || !validSkillText(d.Description, 1000, true) || !validSkillText(d.Content, 16000, true) {
		return fmt.Errorf("invalid reviewed execution skill identity or content")
	}
	return nil
}

// RenderExecutionSkill uses JSON double-quoted YAML scalars for platform-owned
// metadata. Skill content is kept byte-for-byte; no scripts or hooks are added.
func RenderExecutionSkill(d ExecutionSkillDocument) ([]byte, error) {
	if err := validateSkillDocumentIdentity(d); err != nil {
		return nil, err
	}
	name, _ := json.Marshal(d.Name)
	description, _ := json.Marshal(d.Description)
	return []byte("---\nname: " + string(name) + "\ndescription: " + string(description) + "\n---\n\n" + d.Content + "\n"), nil
}

func ExecutionSkillBundleDigest(b ExecutionSkillBundle) string {
	b.BundleSHA256 = ""
	return digest(b)
}

// BuildExecutionSkillBundle derives metadata and byte digests only. Callers must
// resolve the supplied documents from current, authorized published revisions.
func BuildExecutionSkillBundle(documents []ExecutionSkillDocument) (*ExecutionSkillBundle, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	b := &ExecutionSkillBundle{Protocol: ExecutionSkillProtocol, Documents: append([]ExecutionSkillDocument{}, documents...)}
	for i := range b.Documents {
		d := &b.Documents[i]
		d.Name = ExecutionSkillName(d.ID)
		if strings.TrimSpace(d.Description) == "" {
			d.Description = d.Title
		}
		d.ContentSHA256 = executionSkillBytesSHA([]byte(d.Content))
		raw, err := RenderExecutionSkill(*d)
		if err != nil {
			return nil, err
		}
		d.DocumentSHA256 = executionSkillBytesSHA(raw)
	}
	b.BundleSHA256 = ExecutionSkillBundleDigest(*b)
	if err := ValidateExecutionSkillBundle(*b); err != nil {
		return nil, err
	}
	return b, nil
}

func ValidateExecutionSkillBundle(b ExecutionSkillBundle) error {
	if b.Protocol != ExecutionSkillProtocol || len(b.Documents) == 0 || len(b.Documents) > MaxExecutionSkills || !hash.MatchString(b.BundleSHA256) || b.BundleSHA256 != ExecutionSkillBundleDigest(b) {
		return fmt.Errorf("invalid execution skill bundle")
	}
	seen := map[string]bool{}
	total := 0
	for _, d := range b.Documents {
		if err := validateSkillDocumentIdentity(d); err != nil {
			return err
		}
		raw, err := RenderExecutionSkill(d)
		if err != nil || seen[d.ID] || seen[d.Name] || d.ContentSHA256 != executionSkillBytesSHA([]byte(d.Content)) || d.DocumentSHA256 != executionSkillBytesSHA(raw) {
			return fmt.Errorf("duplicate or changed execution skill document")
		}
		seen[d.ID], seen[d.Name] = true, true
		total += len(raw)
		if total > MaxExecutionSkillBytes {
			return fmt.Errorf("execution skill documents exceed input limit")
		}
	}
	return nil
}

// DecodeExecutionSkillBundle is strict at the untyped workflow boundary. An
// authenticated Assignment must independently carry the same protected bundle.
func DecodeExecutionSkillBundle(value any) (*ExecutionSkillBundle, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 512000 {
		return nil, fmt.Errorf("invalid execution skill encoding")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var bundle ExecutionSkillBundle
	if d.Decode(&bundle) != nil || d.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("invalid execution skill fields")
	}
	if err = ValidateExecutionSkillBundle(bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func ExecutionSkillReceipts(b ExecutionSkillBundle) []ExecutionSkillReceipt {
	out := make([]ExecutionSkillReceipt, len(b.Documents))
	for i, d := range b.Documents {
		out[i] = ExecutionSkillReceipt{ID: d.ID, Revision: d.Revision, SHA256: d.SHA256, Name: d.Name, DocumentSHA256: d.DocumentSHA256}
	}
	return out
}

func CloneExecutionSkillBundle(b *ExecutionSkillBundle) *ExecutionSkillBundle {
	if b == nil {
		return nil
	}
	copy := *b
	copy.Documents = append([]ExecutionSkillDocument{}, b.Documents...)
	return &copy
}

func CloneExecutionSkillEvidence(e *ExecutionSkillEvidence) *ExecutionSkillEvidence {
	if e == nil {
		return nil
	}
	copy := *e
	copy.Documents = append([]ExecutionSkillReceipt{}, e.Documents...)
	return &copy
}

func ValidateExecutionSkillEvidence(e ExecutionSkillEvidence) error {
	if !hash.MatchString(e.BundleSHA256) || len(e.Documents) == 0 || len(e.Documents) > MaxExecutionSkills || e.CatalogAcknowledged && !e.Materialized || e.InvocationRequested && !e.CatalogAcknowledged || e.TurnAcknowledged && !e.InvocationRequested || e.FilesVerified && !e.Materialized {
		return fmt.Errorf("invalid execution skill evidence sequence")
	}
	if e.FailureCode != "" && !validID(e.FailureCode, false) {
		return fmt.Errorf("invalid execution skill failure code")
	}
	seen := map[string]bool{}
	for _, d := range e.Documents {
		if !validID(d.ID, false) || d.Revision <= 0 || int64(d.Revision) > MaxSafeInteger || !hash.MatchString(d.SHA256) || d.Name != ExecutionSkillName(d.ID) || !hash.MatchString(d.DocumentSHA256) || seen[d.ID] || seen[d.Name] {
			return fmt.Errorf("invalid execution skill receipt")
		}
		seen[d.ID], seen[d.Name] = true, true
	}
	return nil
}

func ExecutionSkillEvidenceMatchesBundle(b ExecutionSkillBundle, e ExecutionSkillEvidence) bool {
	return ValidateExecutionSkillBundle(b) == nil && ValidateExecutionSkillEvidence(e) == nil && b.BundleSHA256 == e.BundleSHA256 && digest(ExecutionSkillReceipts(b)) == digest(e.Documents)
}
