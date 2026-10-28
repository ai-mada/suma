package dispatchrecommendationcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

var ErrInvalid = errors.New("invalid dispatch recommendation proposal")
var candidateID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var reasoningEffort = regexp.MustCompile(`^[A-Za-z0-9._-]{0,64}$`)

func text(value string, limit int) bool {
	if len(value) > limit || !utf8.ValidString(value) || strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func ValidateProposal(p DispatchRecommendationProposal) error {
	if p.SchemaVersion != 1 || !text(p.Summary, 2000) || !text(p.Rationale, 4000) || len(p.Recommendations) < 1 || len(p.Recommendations) > 3 || p.Questions == nil || len(p.Questions) > 5 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, choice := range p.Recommendations {
		if validateChoice(choice) != nil || seen[choice.CandidateID] {
			return ErrInvalid
		}
		seen[choice.CandidateID] = true
	}
	for _, question := range p.Questions {
		if !text(question, 1000) {
			return ErrInvalid
		}
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > MaxOutputBytes {
		return ErrInvalid
	}
	return nil
}

// A choice is also decoded independently when a person selects a proposal.
// Validate its syntax there as well; candidate membership remains a domain rule.
func validateChoice(c DispatchRecommendationChoice) error {
	if !candidateID.MatchString(c.CandidateID) || !text(c.Reason, 2000) || c.MaxSteps != nil && (*c.MaxSteps < 1 || *c.MaxSteps > 128) || c.MaxOutputTokens != nil && (*c.MaxOutputTokens < 1 || *c.MaxOutputTokens > 131072) || c.ReasoningEffort != nil && !reasoningEffort.MatchString(*c.ReasoningEffort) {
		return ErrInvalid
	}
	return nil
}

// Canonical field names are checked before encoding/json's case-insensitive
// struct decoder. Nullable choice scalars must still be explicitly present.
func object(raw []byte, required, nullable []string) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != len(required) {
		return ErrInvalid
	}
	allowedNull := map[string]bool{}
	for _, key := range nullable {
		allowedNull[key] = true
	}
	for _, key := range required {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !allowedNull[key] {
			return ErrInvalid
		}
	}
	return nil
}

func ParseProposal(raw string) (DispatchRecommendationProposal, error) {
	var result DispatchRecommendationProposal
	data := []byte(raw)
	if len(data) > MaxOutputBytes || !utf8.Valid(data) || !tc.ValidJSONUnicode(data) || uniqueJSON(data) != nil || object(data, []string{"schema_version", "summary", "rationale", "recommendations", "questions"}, nil) != nil {
		return result, ErrInvalid
	}
	type wire DispatchRecommendationProposal
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || ValidateProposal(DispatchRecommendationProposal(decoded)) != nil {
		return result, ErrInvalid
	}
	return DispatchRecommendationProposal(decoded), nil
}

func (p *DispatchRecommendationProposal) UnmarshalJSON(raw []byte) error {
	parsed, err := ParseProposal(string(raw))
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

func (c *DispatchRecommendationChoice) UnmarshalJSON(raw []byte) error {
	if !tc.ValidJSONUnicode(raw) || uniqueJSON(raw) != nil || object(raw, []string{"candidate_id", "max_steps", "max_output_tokens", "reasoning_effort", "reason"}, []string{"max_steps", "max_output_tokens", "reasoning_effort"}) != nil {
		return ErrInvalid
	}
	type wire DispatchRecommendationChoice
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || validateChoice(DispatchRecommendationChoice(decoded)) != nil {
		return ErrInvalid
	}
	*c = DispatchRecommendationChoice(decoded)
	return nil
}

// A bounded token walk prevents duplicate keys from being erased by decoding.
// It preserves integer lexical validation for the later typed decoder.
func uniqueJSON(raw []byte) error {
	if len(raw) > MaxOutputBytes {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	count := 0
	var value func(int) error
	value = func(depth int) error {
		count++
		if depth > 12 || count > 8192 {
			return ErrInvalid
		}
		token, err := decoder.Token()
		if err != nil {
			return ErrInvalid
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for decoder.More() {
					token, err := decoder.Token()
					key, ok := token.(string)
					if err != nil || !ok || seen[key] {
						return ErrInvalid
					}
					seen[key] = true
					if value(depth+1) != nil {
						return ErrInvalid
					}
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim('}') {
					return ErrInvalid
				}
			case '[':
				for decoder.More() {
					if value(depth+1) != nil {
						return ErrInvalid
					}
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim(']') {
					return ErrInvalid
				}
			default:
				return ErrInvalid
			}
		}
		return nil
	}
	if value(0) != nil {
		return ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}
