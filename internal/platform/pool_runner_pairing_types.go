package platform

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	pc "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"
)

// The machine and browser share one wire definition; neither creates a user
// identity or a project grant from a Pool pairing.
type PoolRunnerPairing = pc.PoolRunnerPairing
type PoolRunnerPairingPage = pc.PoolRunnerPairingPage
type PoolRunnerPairingCreateRequest = pc.PoolRunnerPairingCreateRequest
type PoolRunnerPairingCommandRequest = pc.PoolRunnerPairingCommandRequest
type PoolRunnerPairingProposalRequest = pc.PoolRunnerPairingProposalRequest
type PoolRunnerPairingStatusRequest = pc.PoolRunnerPairingStatusRequest

func poolPairingError(status int, reason string) error {
	messages := map[string]string{
		"invalid":     "资源池节点配对请求或分页参数无效",
		"changed":     "资源池节点配对依据已变化，请核对原请求",
		"expired":     "资源池节点配对已过期，请创建新请求",
		"rejected":    "资源池节点配对已被拒绝",
		"limit":       "此资源池已有八个等待中的节点配对",
		"unavailable": "资源池节点配对需要安全的控制面地址",
	}
	return &APIError{status, "pool_runner_pairing_" + reason, messages[reason], nil}
}
func poolPairingStamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }
func poolPairingText(s string, max int) bool {
	return s != "" && len(s) <= max && strings.TrimSpace(s) == s && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func poolPairingMachineInput(id, pool, key, token string) bool {
	return executionPoolID.MatchString(id) && executionPoolID.MatchString(pool) && validatePoolOperation(key) == nil && len(token) >= 32 && len(token) <= 512 && utf8.ValidString(token) && strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}
func validPoolPairing(p PoolRunnerPairing) bool {
	if !executionPoolID.MatchString(p.ID) || p.Purpose != "pool_runner" || !executionPoolID.MatchString(p.OwnerTenantID) || !executionPoolID.MatchString(p.PoolID) || !ingressVersion(p.PoolVersion) || !ingressVersion(p.Version) || !poolPairingText(p.Name, 200) || p.MaxSlots < 1 || p.MaxSlots > 128 || validatePoolTTL(p.TTLSeconds) != nil {
		return false
	}
	canonical, e := runnerPairingControlURL(p.ControlURL)
	if e != nil || canonical != p.ControlURL {
		return false
	}
	created, e := time.Parse(time.RFC3339Nano, p.CreatedAt)
	if e != nil || poolPairingStamp(created) != p.CreatedAt {
		return false
	}
	expires, e := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if e != nil || poolPairingStamp(expires) != p.ExpiresAt || !expires.Equal(created.Add(10*time.Minute)) {
		return false
	}
	proposed := p.DeviceName != "" || p.CredentialSHA256 != ""
	if proposed && (!poolPairingText(p.DeviceName, 100) || !desktopWorkspaceHex(p.CredentialSHA256, 64)) {
		return false
	}
	approved := p.RunnerID != "" || p.WorkerID != "" || p.CredentialVersion != 0 || p.CredentialExpiresAt != ""
	if approved {
		if !executionPoolID.MatchString(p.RunnerID) || p.WorkerID != "pool-"+p.RunnerID || p.CredentialVersion != 1 {
			return false
		}
		if _, e := time.Parse(time.RFC3339Nano, p.CredentialExpiresAt); e != nil {
			return false
		}
	}
	switch p.State {
	case "waiting":
		return !proposed && !approved
	case "proposed":
		return proposed && !approved
	case "approved":
		return proposed && approved
	case "rejected":
		return !approved
	default:
		return false
	}
}
func poolPairingTerminal(p PoolRunnerPairing) error {
	if p.State == "expired" {
		return poolPairingError(409, "expired")
	}
	if p.State == "rejected" {
		return poolPairingError(409, "rejected")
	}
	return nil
}
