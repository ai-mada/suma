// Package executioncontract contains host-authored execution lifecycle facts.
// These facts describe an authenticated attempt and never grant execution.
package executioncontract

import (
	"bytes"
	"encoding/json"
	"fmt"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const StartupFailureProtocol = "suma.execution-startup-failure.v1"

type StartupFailureProof struct {
	Protocol         string           `json:"protocol"`
	Stage            string           `json:"stage"`
	Assignment       cc.AssignmentRef `json:"assignment"`
	Profile          tc.Profile       `json:"profile"`
	Purpose          string           `json:"purpose"`
	WorkspaceStarted bool             `json:"workspace_started"`
	ProcessStarted   bool             `json:"process_started"`
}

func (proof *StartupFailureProof) UnmarshalJSON(raw []byte) error {
	// False is an explicit host assertion. Missing/null flags must not become
	// a certificate merely because a Go bool has its zero value.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 7 {
		return fmt.Errorf("invalid host startup failure proof")
	}
	for _, name := range []string{"protocol", "stage", "assignment", "profile", "purpose", "workspace_started", "process_started"} {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("invalid host startup failure proof")
		}
	}
	type wire StartupFailureProof
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*proof = StartupFailureProof(decoded)
	return nil
}

func ValidateStartupFailureProof(proof StartupFailureProof, assignment cc.AssignmentRef, profile tc.Profile, purpose string) error {
	if proof.Protocol != StartupFailureProtocol || proof.Stage != "before_workspace" || !assignment.Valid() || proof.Assignment != assignment || tc.ValidateProfile(profile, true) != nil || !tc.EqualProfile(proof.Profile, profile) || purpose != "candidate_conflict_repair" || proof.Purpose != purpose || proof.WorkspaceStarted || proof.ProcessStarted {
		return fmt.Errorf("invalid host startup failure proof")
	}
	return nil
}
