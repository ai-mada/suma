// Package inputdelivery describes durable human-input delivery independently
// of a vendor's protocol. A complete write is never a vendor acceptance ACK.
package inputdelivery

import (
	"fmt"
	"regexp"
)

type ToolInputDeliveryObservation struct {
	Sequence        int64  `json:"sequence"`
	ParamsSHA256    string `json:"params_sha256"`
	AnswerSHA256    string `json:"answer_sha256,omitempty"`
	AnswerObserved  bool   `json:"answer_observed"`
	ReplyStarted    bool   `json:"reply_started"`
	ReplyWritten    bool   `json:"reply_written"`
	RequestResolved bool   `json:"request_resolved"`
	ExecutionClosed bool   `json:"execution_closed"`
	FailureCode     string `json:"failure_code,omitempty"`
}

// Observation is the package-local spelling; the concrete exported name keeps
// generated API schemas unambiguous across domain packages.
type Observation = ToolInputDeliveryObservation

var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var failure = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

func Validate(o Observation) error {
	if o.Sequence < 1 || o.Sequence > 9007199254740991 || !digest.MatchString(o.ParamsSHA256) || o.AnswerObserved != (o.AnswerSHA256 != "") || o.AnswerSHA256 != "" && !digest.MatchString(o.AnswerSHA256) || o.ReplyStarted && !o.AnswerObserved || o.ReplyWritten && !o.ReplyStarted || o.FailureCode != "" && !failure.MatchString(o.FailureCode) {
		return fmt.Errorf("invalid execution input delivery observation")
	}
	return nil
}
