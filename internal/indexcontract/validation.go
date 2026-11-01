package indexcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Protocol = "suma-knowledge-index-v1"
const MaxSequence = 2147483647
const MaxWatermark = int64(9007199254740991)
const MaxCandidates = 100

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func ID(value string) bool {
	return len(value) > 0 && len(value) <= 64 && utf8.ValidString(value) && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}
func Generation(value string) bool {
	return len(value) > 0 && len(value) <= 128 && utf8.ValidString(value) && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}
func Mode(value string) bool {
	return value == "lancedb_fts" || value == "lancedb_hybrid" || value == "oceanbase_fts" || value == "oceanbase_hybrid"
}
func sourceValid(value Source, action string) bool {
	if (value.Kind != "documents" && value.Kind != "project_knowledge") || !ID(value.ID) || value.Sequence < 1 || value.Sequence > MaxSequence {
		return false
	}
	if value.Kind == "project_knowledge" && action == "delete" && value.Revision == 0 && value.SHA256 == "" {
		return true
	}
	return value.Revision > 0 && value.Revision <= MaxSequence && digestPattern.MatchString(value.SHA256) && (value.Kind != "documents" || value.Sequence == value.Revision)
}
func ValidateRequest(value Request) error {
	if value.Protocol != Protocol || !ID(value.EventID) || !ID(value.TenantID) || !ID(value.ProjectID) || (value.Action != "upsert" && value.Action != "delete") || !sourceValid(value.Source, value.Action) {
		return fmt.Errorf("invalid typed index source")
	}
	if value.Action == "delete" {
		if value.Title != "" || value.Content != "" {
			return fmt.Errorf("index tombstone must not carry content")
		}
		return nil
	}
	if !utf8.ValidString(value.Title) || len(value.Title) > 4096 || !utf8.ValidString(value.Content) || len(value.Content) > 1<<20 || strings.ContainsRune(value.Title, 0) || strings.ContainsRune(value.Content, 0) {
		return fmt.Errorf("invalid bounded index text")
	}
	if value.Source.Kind == "documents" && value.Source.SHA256 != Digest([]byte(value.Content)) {
		return fmt.Errorf("document index content digest mismatch")
	}
	return nil
}
func ValidateReceipt(value Receipt) error {
	if value.Protocol != Protocol || !ID(value.EventID) || !ID(value.TenantID) || !ID(value.ProjectID) || (value.Action != "upsert" && value.Action != "delete") || !sourceValid(value.Source, value.Action) || !Generation(value.Generation) || value.IndexWatermark < 0 || value.IndexWatermark > MaxWatermark || !digestPattern.MatchString(value.RequestSHA256) {
		return fmt.Errorf("invalid typed index receipt")
	}
	if value.Disposition != "applied" && value.Disposition != "duplicate" && value.Disposition != "obsolete" {
		return fmt.Errorf("invalid index disposition")
	}
	return nil
}
func ReceiptMatches(value Receipt, request Request, raw []byte) bool {
	return ValidateReceipt(value) == nil && value.Protocol == request.Protocol && value.EventID == request.EventID && value.TenantID == request.TenantID && value.ProjectID == request.ProjectID && value.Source == request.Source && value.Action == request.Action && value.RequestSHA256 == Digest(raw)
}
func ValidateSearchRequest(value SearchRequest) error {
	if value.Protocol != Protocol || !ID(value.TenantID) || !ID(value.ProjectID) || (value.SourceKind != "documents" && value.SourceKind != "project_knowledge") || !utf8.ValidString(value.Query) || strings.TrimSpace(value.Query) == "" || len(value.Query) > 500 || strings.IndexFunc(value.Query, unicode.IsControl) >= 0 || value.Limit < 1 || value.Limit > MaxCandidates {
		return fmt.Errorf("invalid typed index query")
	}
	return nil
}
func ValidateLocator(value Locator) error {
	if value.Unit != "unicode_codepoints" || value.Start < 0 || value.End <= value.Start || value.End > 1<<20 {
		return fmt.Errorf("invalid index codepoint locator")
	}
	return nil
}
func ValidateSearchResult(value SearchResult) error {
	if value.Protocol != Protocol || value.Items == nil || len(value.Items) > MaxCandidates || !Mode(value.Mode) || !Generation(value.Generation) || value.IndexWatermark < 0 || value.IndexWatermark > MaxWatermark {
		return fmt.Errorf("invalid typed index search result")
	}
	for _, hit := range value.Items {
		if !sourceValid(hit.Source, "upsert") || ValidateLocator(hit.Locator) != nil {
			return fmt.Errorf("invalid typed index search hit")
		}
	}
	return nil
}
