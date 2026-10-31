// Package desktoprecoverycontract shares facts for already-committed result
// receipt recovery. A proof is not a Runner credential or execution permission.
package desktoprecoverycontract

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
)

const PlanProtocol = "suma-desktop-result-recovery-plan-v1"
const ProofProtocol = "suma-desktop-result-recovery-proof-v1"
const ResultProtocol = "suma-desktop-result-recovery-v1"
const MaxItems = 32
const InputLimit = 256 << 10

type Scope struct {
	ProjectID               string `json:"project_id"`
	UserID                  string `json:"user_id"`
	TenantID                string `json:"tenant_id"`
	WorkerID                string `json:"worker_id"`
	AdapterID               string `json:"adapter_id"`
	InstanceID              string `json:"instance_id"`
	ExpectedBindingSHA256   string `json:"expected_binding_sha256"`
	BaseCommit              string `json:"base_commit"`
	RegistrationOperationID string `json:"registration_operation_id"`
	CredentialSHA256        string `json:"credential_sha256"`
}

type Item struct {
	JournalKey   string `json:"journal_key"`
	RunID        string `json:"run_id"`
	NodeID       string `json:"node_id"`
	Attempt      int    `json:"attempt"`
	Fence        string `json:"fence"`
	OperationID  string `json:"operation_id"`
	BodySHA256   string `json:"body_sha256"`
	ResultSHA256 string `json:"result_sha256"`
}

type Plan struct {
	Protocol       string `json:"protocol"`
	ProjectID      string `json:"project_id"`
	WorkerID       string `json:"worker_id"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	Items          []Item `json:"items"`
}

type Receipt struct {
	JournalKey  string `json:"journal_key"`
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
}

type Proof struct {
	Protocol   string    `json:"protocol"`
	Scope      Scope     `json:"scope"`
	PlanSHA256 string    `json:"plan_sha256"`
	IssuedAt   string    `json:"issued_at"`
	ExpiresAt  string    `json:"expires_at"`
	Items      []Receipt `json:"items"`
	Signature  string    `json:"signature"`
}

type Result struct {
	Protocol  string   `json:"protocol"`
	Plan      *Plan    `json:"plan,omitempty"`
	Total     int      `json:"total"`
	Recovered int      `json:"recovered"`
	Remaining int      `json:"remaining"`
	Reasons   []string `json:"reasons"`
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var workerPattern = regexp.MustCompile(`^desktop-[a-f0-9]{32}$`)
var registrationPattern = regexp.MustCompile(`^desktop-runner-[a-f0-9]{32}$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}(?:[a-f0-9]{24})?$`)

func Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func SafeText(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}
func ValidateScope(scope Scope) error {
	adapters := map[string]bool{"codex-jsonl": true, "codebuddy-cli": true, "dsh-multica": true, "claude-code-cli": true, "qoder-acp": true, "codex-app-server": true, "opencode-sdk": true}
	if !idPattern.MatchString(scope.ProjectID) || !idPattern.MatchString(scope.UserID) || !idPattern.MatchString(scope.TenantID) || !workerPattern.MatchString(scope.WorkerID) || !adapters[scope.AdapterID] || !digestPattern.MatchString(scope.InstanceID) || !digestPattern.MatchString(scope.ExpectedBindingSHA256) || !commitPattern.MatchString(scope.BaseCommit) || !registrationPattern.MatchString(scope.RegistrationOperationID) || !digestPattern.MatchString(scope.CredentialSHA256) {
		return fmt.Errorf("invalid result-recovery scope")
	}
	return nil
}
func ValidatePlan(plan Plan) error {
	if plan.Protocol != PlanProtocol || !idPattern.MatchString(plan.ProjectID) || !workerPattern.MatchString(plan.WorkerID) || !digestPattern.MatchString(plan.SnapshotSHA256) || plan.Items == nil || len(plan.Items) > MaxItems {
		return fmt.Errorf("invalid result-recovery plan")
	}
	seen := map[string]bool{}
	for _, item := range plan.Items {
		fence, err := strconv.ParseInt(item.Fence, 10, 64)
		if err != nil || fence < 1 || strconv.FormatInt(fence, 10) != item.Fence || item.Attempt < 1 || item.Attempt > 1000000 || !SafeText(item.JournalKey, 1024) || seen[item.JournalKey] || !SafeText(item.RunID, 256) || !SafeText(item.NodeID, 256) || !SafeText(item.OperationID, 256) || !digestPattern.MatchString(item.BodySHA256) || !digestPattern.MatchString(item.ResultSHA256) {
			return fmt.Errorf("invalid result-recovery item")
		}
		seen[item.JournalKey] = true
	}
	return nil
}
func PlanSHA256(plan Plan) string { raw, _ := json.Marshal(plan); return Digest(raw) }

// ServerResultSHA deliberately reproduces the existing Server's dag.Result
// normalization, including field order. The Runner wire/envelope is different.
func ServerResultSHA(raw json.RawMessage) (string, error) {
	var result dag.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return Digest(encoded), nil
}
func proofSignature(proof Proof, key []byte) string {
	proof.Signature = ""
	raw, _ := json.Marshal(proof)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ProofProtocol + "\n"))
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func SignProof(proof Proof, key []byte) Proof {
	proof.Signature = proofSignature(proof, key)
	return proof
}

// CheckProof checks immutable fact authenticity. Read-only status may inspect a
// previously valid expired proof; allowExpired never authorizes an ACK write.
func CheckProof(proof Proof, plan Plan, key []byte, now time.Time, allowExpired bool) error {
	if len(key) != 32 || ValidatePlan(plan) != nil || ValidateScope(proof.Scope) != nil || proof.Protocol != ProofProtocol || proof.Scope.ProjectID != plan.ProjectID || proof.Scope.WorkerID != plan.WorkerID || proof.PlanSHA256 != PlanSHA256(plan) || len(proof.Items) != len(plan.Items) || !digestPattern.MatchString(proof.Signature) {
		return fmt.Errorf("invalid recovery receipt proof")
	}
	issued, e1 := time.Parse(time.RFC3339Nano, proof.IssuedAt)
	expires, e2 := time.Parse(time.RFC3339Nano, proof.ExpiresAt)
	if e1 != nil || e2 != nil || !expires.After(issued) || expires.Sub(issued) > time.Minute || issued.After(now.Add(5*time.Second)) || !allowExpired && !now.Before(expires) {
		return fmt.Errorf("expired or invalid recovery receipt proof")
	}
	for i, receipt := range proof.Items {
		if receipt.JournalKey != plan.Items[i].JournalKey || receipt.OperationID != plan.Items[i].OperationID || !map[string]bool{"matched": true, "missing": true, "mismatch": true, "source_changed": true}[receipt.Status] {
			return fmt.Errorf("recovery receipt items differ from plan")
		}
	}
	expected, _ := hex.DecodeString(proofSignature(proof, key))
	actual, _ := hex.DecodeString(proof.Signature)
	if !hmac.Equal(expected, actual) {
		return fmt.Errorf("recovery receipt proof signature mismatch")
	}
	return nil
}
