package acpcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrPermissionInvalid = errors.New("invalid ACP tool permission")
var permIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,149}$`)
var permDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

const permMaxInteger int64 = 9007199254740991

func permText(s string, max int, multiline bool) bool {
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
func permToken(s string, max int) bool {
	return s != "" && strings.TrimSpace(s) == s && permText(s, max, false)
}
func permID(s string) bool  { return permIdentifier.MatchString(s) }
func permKey(s string) bool { return len(s) >= 8 && permToken(s, 128) }
func permTime(s string) (time.Time, bool) {
	t, e := time.Parse(time.RFC3339Nano, s)
	return t, e == nil && strings.HasSuffix(s, "Z")
}

func ValidatePermissionOption(p PermissionOption) error {
	if !permToken(p.ID, 128) || !permToken(p.Name, 512) {
		return ErrPermissionInvalid
	}
	switch p.Kind {
	case "allow_once", "reject_once", "allow_always", "reject_always":
		return nil
	}
	return ErrPermissionInvalid
}
func ValidatePermissionRequest(p PermissionRequest) error {
	if !permKey(p.IdempotencyKey) || !permToken(p.SessionID, 128) || !permToken(p.ToolCallID, 128) || strings.TrimSpace(p.Title) == "" || !permText(p.Title, 4096, true) || !permToken(p.Kind, 128) || !permText(p.Details, 32<<10, true) || p.Options == nil || len(p.Options) < 1 || len(p.Options) > 16 {
		return ErrPermissionInvalid
	}
	seen := map[string]bool{}
	for _, o := range p.Options {
		if ValidatePermissionOption(o) != nil || seen[o.ID] {
			return ErrPermissionInvalid
		}
		seen[o.ID] = true
	}
	encoded, err := json.Marshal(p)
	if err != nil || len(encoded) > 64<<10 {
		return ErrPermissionInvalid
	}
	return nil
}
func ValidatePermissionDecision(p PermissionDecision) error {
	_, ok := permTime(p.DecidedAt)
	if !permToken(p.OptionID, 128) || (p.Kind != "allow_once" && p.Kind != "reject_once") || !permID(p.ActorID) || !ok {
		return ErrPermissionInvalid
	}
	return nil
}
func ValidateDecisionRequest(p DecisionRequest) error {
	if !permID(p.ProjectID) || p.ExpectedVersion < 1 || p.ExpectedVersion >= permMaxInteger || !permKey(p.IdempotencyKey) || !permToken(p.OptionID, 128) {
		return ErrPermissionInvalid
	}
	return nil
}
func ValidatePermission(p Permission) error {
	start, a := permTime(p.CreatedAt)
	end, b := permTime(p.ExpiresAt)
	if !permID(p.ID) || !permID(p.ProjectID) || !permID(p.RunID) || !permID(p.NodeID) || p.Attempt < 1 || p.Fence < 1 || p.Fence > permMaxInteger || !permID(p.DispatchID) || !permID(p.WorkerID) || !permDigest.MatchString(p.ProfileSHA256) || p.Version < 1 || p.Version > permMaxInteger || ValidatePermissionRequest(p.Request) != nil || !a || !b || !end.After(start) || p.AllowedActions == nil {
		return ErrPermissionInvalid
	}
	if p.State != "pending" && p.State != "decided" && p.State != "expired" && p.State != "superseded" {
		return ErrPermissionInvalid
	}
	if p.State == "decided" && p.Decision == nil || p.State != "decided" && p.Decision != nil {
		return ErrPermissionInvalid
	}
	if p.Decision != nil {
		if ValidatePermissionDecision(*p.Decision) != nil {
			return ErrPermissionInvalid
		}
		found := false
		for _, o := range p.Request.Options {
			if o.ID == p.Decision.OptionID && o.Kind == p.Decision.Kind {
				found = true
			}
		}
		if !found || p.Decision.Kind == "allow_once" && !p.Request.DetailsComplete {
			return ErrPermissionInvalid
		}
	}
	last := -1
	for _, action := range p.AllowedActions {
		at := -1
		if action == "allow_once" {
			at = 0
		}
		if action == "reject_once" {
			at = 1
		}
		if at <= last || p.State != "pending" || action == "allow_once" && !p.Request.DetailsComplete {
			return ErrPermissionInvalid
		}
		last = at
	}
	return nil
}
func ValidatePermissionList(p PermissionList) error {
	if p.Items == nil || len(p.Items) > 32 {
		return ErrPermissionInvalid
	}
	seen := map[string]bool{}
	for _, item := range p.Items {
		if ValidatePermission(item) != nil || seen[item.ID] {
			return ErrPermissionInvalid
		}
		seen[item.ID] = true
	}
	return nil
}

// Check UTF-16 escapes before encoding/json can replace an unpaired surrogate.
func permUnicode(raw []byte) bool {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	hex := func(v []byte) uint16 {
		var n uint16
		for _, c := range v {
			n <<= 4
			switch {
			case c >= '0' && c <= '9':
				n += uint16(c - '0')
			case c >= 'a' && c <= 'f':
				n += uint16(c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				n += uint16(c - 'A' + 10)
			}
		}
		return n
	}
	in := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			in = !in
			continue
		}
		if !in || raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		n := hex(raw[i+1 : i+5])
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n < 0xd800 || n > 0xdbff {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low := hex(raw[i+3 : i+7])
		if low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
func permDecode(raw []byte, out any, required []string) error {
	return permDecodeBounded(raw, out, required, 128<<10)
}
func permDecodeBounded(raw []byte, out any, required []string, maximum int) error {
	if len(raw) > maximum || !permUnicode(raw) {
		return ErrPermissionInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	count := 0
	var walk func(int) error
	walk = func(depth int) error {
		count++
		if depth > 24 || count > 65536 {
			return ErrPermissionInvalid
		}
		v, e := d.Token()
		if e != nil {
			return ErrPermissionInvalid
		}
		switch v {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				s, ok := k.(string)
				if e != nil || !ok || seen[s] {
					return ErrPermissionInvalid
				}
				seen[s] = true
				if walk(depth+1) != nil {
					return ErrPermissionInvalid
				}
			}
			_, e = d.Token()
			return e
		case json.Delim('['):
			for d.More() {
				if walk(depth+1) != nil {
					return ErrPermissionInvalid
				}
			}
			_, e = d.Token()
			return e
		}
		return nil
	}
	if walk(0) != nil {
		return ErrPermissionInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrPermissionInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return ErrPermissionInvalid
	}
	for _, k := range required {
		v, ok := fields[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return ErrPermissionInvalid
		}
	}
	allowed := map[string]bool{}
	typ := reflect.TypeOf(out).Elem()
	for i := 0; i < typ.NumField(); i++ {
		allowed[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for k := range fields {
		if !allowed[k] {
			return ErrPermissionInvalid
		}
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrPermissionInvalid
	}
	return nil
}
func (p *ToolPermissionOption) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionOption
	var v wire
	if permDecode(raw, &v, []string{"id", "name", "kind"}) != nil || ValidatePermissionOption(PermissionOption(v)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermissionOption(v)
	return nil
}
func (p *ToolPermissionRequest) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionRequest
	var v wire
	if permDecodeBounded(raw, &v, []string{"idempotency_key", "session_id", "tool_call_id", "title", "kind", "details", "details_complete", "options"}, 64<<10) != nil || ValidatePermissionRequest(PermissionRequest(v)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermissionRequest(v)
	return nil
}
func (p *ToolPermissionDecision) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionDecision
	var v wire
	if permDecode(raw, &v, []string{"option_id", "kind", "actor_id", "decided_at"}) != nil || ValidatePermissionDecision(PermissionDecision(v)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermissionDecision(v)
	return nil
}
func (p *ToolPermissionDecisionRequest) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionDecisionRequest
	var v wire
	if permDecode(raw, &v, []string{"project_id", "expected_version", "idempotency_key", "option_id"}) != nil || ValidateDecisionRequest(DecisionRequest(v)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermissionDecisionRequest(v)
	return nil
}
func (p *ToolPermission) UnmarshalJSON(raw []byte) error {
	type wire ToolPermission
	var v wire
	if permDecode(raw, &v, []string{"id", "project_id", "run_id", "node_id", "attempt", "fence", "dispatch_id", "worker_id", "profile_sha256", "version", "state", "request", "created_at", "expires_at", "allowed_actions"}) != nil || ValidatePermission(Permission(v)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermission(v)
	return nil
}
func (p *ToolPermissionList) UnmarshalJSON(raw []byte) error {
	type wire ToolPermissionList
	var v wire
	if permDecodeBounded(raw, &v, []string{"items"}, 3<<20) != nil || ValidatePermissionList(PermissionList(v)) != nil {
		return ErrPermissionInvalid
	}
	*p = ToolPermissionList(v)
	return nil
}
