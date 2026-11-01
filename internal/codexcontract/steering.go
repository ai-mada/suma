package codexcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const SteeringProtocol = "codex-turn-steer-v1"

// MaxSteeringTextBytes matches the existing saved discussion message limit.
const MaxSteeringTextBytes = 32000

// ValidSteeringText matches the immutable discussion producer: at most 8,000
// Unicode scalars and 32,000 UTF-8 bytes. JSON encoding, rather
// than rewriting human content, protects the native transport boundary.
func ValidSteeringText(text string) bool {
	return utf8.ValidString(text) && len(text) <= MaxSteeringTextBytes && utf8.RuneCountInString(text) <= 8000 && strings.TrimSpace(text) != "" && !strings.ContainsRune(text, 0)
}

// A source is advertised only after the original configuration and turn ACKs.
// It is not a thread-resume token or authority to replace the original plan.
type CodexSteeringSource struct {
	Protocol      string `json:"protocol"`
	ThreadID      string `json:"thread_id"`
	TurnID        string `json:"turn_id"`
	BindingSHA256 string `json:"binding_sha256"`
}

type CodexSteeringText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type CodexSteeringParams struct {
	ThreadID            string              `json:"threadId"`
	ExpectedTurnID      string              `json:"expectedTurnId"`
	Input               []CodexSteeringText `json:"input"`
	ClientUserMessageID string              `json:"clientUserMessageId"`
}

type CodexSteeringObservation struct {
	Sequence       int64             `json:"sequence"`
	RequestID      CodexRPCRequestID `json:"request_id"`
	ParamsSHA256   string            `json:"params_sha256"`
	Observed       bool              `json:"observed"`
	WriteStarted   bool              `json:"write_started"`
	WriteWritten   bool              `json:"write_written"`
	NativeAccepted bool              `json:"native_accepted"`
	NativeRejected bool              `json:"native_rejected"`
	TurnClosed     bool              `json:"turn_closed"`
	FailureCode    string            `json:"failure_code,omitempty"`
}

type CodexSteeringInstructionEvidence struct {
	InstructionID string                    `json:"instruction_id"`
	MessageSHA256 string                    `json:"message_sha256"`
	Observation   *CodexSteeringObservation `json:"observation,omitempty"`
}

// Bodies stay in the separately authorized discussion/control domain. An ACK
// establishes native acceptance, not model compliance, code quality or review.
type CodexSteeringEvidence struct {
	SessionID    string                             `json:"session_id"`
	Source       CodexSteeringSource                `json:"source"`
	Closed       bool                               `json:"closed"`
	Instructions []CodexSteeringInstructionEvidence `json:"instructions"`
}

func ValidateSteeringSource(s CodexSteeringSource) error {
	if s.Protocol != SteeringProtocol || !validID(s.ThreadID, false) || !validID(s.TurnID, false) || !hash.MatchString(s.BindingSHA256) {
		return fmt.Errorf("invalid original Codex steering source")
	}
	return nil
}

func BuildSteeringParams(s CodexSteeringSource, id, text string) (CodexSteeringParams, error) {
	p := CodexSteeringParams{ThreadID: s.ThreadID, ExpectedTurnID: s.TurnID, Input: []CodexSteeringText{{Type: "text", Text: text}}, ClientUserMessageID: id}
	if ValidateSteeringSource(s) != nil || !validID(id, false) || !ValidSteeringText(text) {
		return p, fmt.Errorf("invalid original Codex steering input")
	}
	return p, nil
}

func SteeringParamsDigest(s CodexSteeringSource, id, text string) (string, error) {
	p, err := BuildSteeringParams(s, id, text)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateSteeringObservation(o CodexSteeringObservation) error {
	if o.Sequence < 1 || o.Sequence > MaxSafeInteger || ValidateRPCRequestID(o.RequestID) != nil || !hash.MatchString(o.ParamsSHA256) || !o.Observed || o.WriteStarted && !o.Observed || o.WriteWritten && !o.WriteStarted || (o.NativeAccepted || o.NativeRejected) && !o.WriteStarted || o.NativeAccepted && o.NativeRejected || o.FailureCode != "" && !userInputFailureCode.MatchString(o.FailureCode) {
		return fmt.Errorf("invalid original Codex steering observation")
	}
	return nil
}

func ValidateSteeringEvidence(e CodexSteeringEvidence) error {
	if !validID(e.SessionID, false) || ValidateSteeringSource(e.Source) != nil || e.Instructions == nil || len(e.Instructions) > 32 {
		return fmt.Errorf("invalid Codex steering evidence")
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, v := range e.Instructions {
		if !validID(v.InstructionID, false) || seen[v.InstructionID] || !hash.MatchString(v.MessageSHA256) {
			return fmt.Errorf("invalid Codex steering instruction identity")
		}
		seen[v.InstructionID] = true
		if v.Observation != nil {
			o := v.Observation
			key := o.RequestID.Kind + ":" + o.RequestID.Value
			if ValidateSteeringObservation(*o) != nil || ids[key] {
				return fmt.Errorf("invalid Codex steering native identity")
			}
			ids[key] = true
		}
	}
	return nil
}
