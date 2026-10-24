package toolcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var commandID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidCommandID(s string) bool { return commandID.MatchString(s) }
func ValidDigest(s string) bool    { return digest.MatchString(s) }
func validText(s string, max int, multiline bool) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
func protocolPair(adapter, protocol string) bool {
	return map[string]string{"command": "text-v1", "suma": "suma-jsonl-v1", "codex": "codex-jsonl-v1", "opencode": "opencode-jsonl-v1"}[adapter] == protocol && protocol != ""
}
func ValidProtocol(protocol string) bool {
	return protocol == "text-v1" || protocol == "suma-jsonl-v1" || protocol == "codex-jsonl-v1" || protocol == "opencode-jsonl-v1"
}
func ValidateProfile(p Profile, complete bool) error {
	if !ValidCommandID(p.CommandID) || !protocolPair(p.Adapter, p.Protocol) {
		return fmt.Errorf("invalid command/adapter/protocol")
	}
	for _, v := range []string{p.ProfileSHA256, p.ExecutableSHA256} {
		if (complete || v != "") && !ValidDigest(v) {
			return fmt.Errorf("invalid profile/executable digest")
		}
	}
	if complete && strings.TrimSpace(p.ToolVersion) == "" || !validText(p.ToolVersion, 256, false) {
		return fmt.Errorf("invalid tool version")
	}
	return nil
}
func ValidateObservation(o Observation) error {
	if err := ValidateProfile(o.Profile, o.Available); err != nil {
		return err
	}
	if !validText(o.Reason, 1000, false) {
		return fmt.Errorf("invalid observation reason")
	}
	if !o.Available && strings.TrimSpace(o.Reason) == "" {
		return fmt.Errorf("unavailable observation requires a reason")
	}
	return nil
}
func validateUsage(u *Usage) error {
	if u == nil {
		return nil
	}
	present := false
	for _, n := range []*int64{u.InputTokens, u.CachedInputTokens, u.CacheWriteInputTokens, u.OutputTokens, u.ReasoningOutputTokens} {
		if n != nil {
			present = true
			if *n < 0 || *n > MaxSafeUsage {
				return fmt.Errorf("usage counter must be a nonnegative safe integer")
			}
		}
	}
	if !present {
		return fmt.Errorf("empty usage is not measured zero")
	}
	return nil
}

// UnmarshalJSON distinguishes missing counters from explicit null. Plain pointer
// decoding would incorrectly accept null as an absent source fact.
func (u *ToolUsage) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("usage must be a nonempty object")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	if err := scanValue(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("trailing usage content")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("reported usage counter cannot be null")
		}
	}
	type wire ToolUsage
	var decoded wire
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&decoded); err != nil {
		return err
	}
	if err := validateUsage((*Usage)(&decoded)); err != nil {
		return err
	}
	*u = ToolUsage(decoded)
	return nil
}

func validateCoverage(state string, u *Usage) error {
	if err := validateUsage(u); err != nil {
		return err
	}
	switch state {
	case UsageComplete:
		if u == nil || u.InputTokens == nil || u.OutputTokens == nil {
			return fmt.Errorf("complete usage requires measured input and output")
		}
	case UsagePartial:
		if u == nil {
			return fmt.Errorf("partial usage requires a reported subtotal")
		}
	case UsageUnreported:
		if u != nil {
			return fmt.Errorf("unreported usage cannot contain measured counters")
		}
	default:
		return fmt.Errorf("usage coverage is required")
	}
	return nil
}
func ValidateEvent(e Event) error {
	if err := validateEventDiagnostic(e); err != nil {
		return err
	}
	if e.SchemaVersion != 1 || !ValidCommandID(e.CommandID) || !ValidDigest(e.ProfileSHA256) || !ValidProtocol(e.Protocol) {
		return fmt.Errorf("invalid tool event identity/schema")
	}
	switch e.Kind {
	case "session_started", "turn_started", "activity_started", "activity_updated", "activity_completed", "message", "usage", "error", "turn_completed", "turn_failed", "waiting_input":
	default:
		return fmt.Errorf("invalid tool event kind")
	}
	if e.SessionID != "" && !safeID.MatchString(e.SessionID) || e.ItemID != "" && !safeID.MatchString(e.ItemID) || !validText(e.Message, 8192, true) || !validText(e.Status, 64, false) {
		return fmt.Errorf("invalid tool event text/identifier")
	}
	if e.UsageState != "" || e.Usage != nil || e.Kind == "usage" {
		if err := validateCoverage(e.UsageState, e.Usage); err != nil {
			return err
		}
	}
	b, err := json.Marshal(e)
	if err != nil || len(b) > 16384 {
		return fmt.Errorf("tool event exceeds 16 KiB")
	}
	return nil
}
func ValidateEvidence(e Evidence) error {
	if err := ValidateProfile(e.Profile, true); err != nil {
		return err
	}
	if e.SessionID != "" && !safeID.MatchString(e.SessionID) {
		return fmt.Errorf("invalid evidence session")
	}
	if e.Terminal != "completed" && e.Terminal != "failed" && e.Terminal != "unknown" {
		return fmt.Errorf("invalid evidence terminal")
	}
	if e.EventCount < 0 || e.EventCount > 1000000 {
		return fmt.Errorf("invalid event count")
	}
	return validateCoverage(e.UsageState, e.Usage)
}

// ProfileDigest binds the canonical administrator configuration and executable
// bytes. Private configuration values are consumed only by the digest operation.
func ProfileDigest(config any, executableSHA string) (string, error) {
	if !ValidDigest(executableSHA) {
		return "", fmt.Errorf("invalid executable digest")
	}
	b, err := json.Marshal(struct {
		Config           any    `json:"config"`
		ExecutableSHA256 string `json:"executable_sha256"`
	}{config, executableSHA})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// DecodeEvent rejects duplicate/unknown fields as well as excessive depth before
// constructing the typed event. Vendor JSON is parsed separately by the runner.
func DecodeEvent(raw []byte) (Event, error) {
	var e Event
	if len(raw) > 16384 || !utf8.Valid(raw) {
		return e, fmt.Errorf("invalid event encoding/size")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err := scanValue(scan, 0); err != nil {
		return e, err
	}
	if _, err := scan.Token(); err != io.EOF {
		return e, fmt.Errorf("trailing tool event content")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&e); err != nil {
		return e, err
	}
	return e, ValidateEvent(e)
}
func scanValue(d *json.Decoder, depth int) error {
	if depth > 8 {
		return fmt.Errorf("tool event depth exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '{' {
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return fmt.Errorf("duplicate/invalid field")
			}
			seen[s] = true
			if err := scanValue(d, depth+1); err != nil {
				return err
			}
		}
	} else if delim == '[' {
		for d.More() {
			if err := scanValue(d, depth+1); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}
